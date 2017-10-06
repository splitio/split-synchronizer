package proxy

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/proxy/controllers"
	"github.com/splitio/split-synchronizer/splitio/proxy/dashboard"
	"github.com/splitio/split-synchronizer/splitio/stats"
	"github.com/splitio/split-synchronizer/splitio/stats/counter"
	"github.com/splitio/split-synchronizer/splitio/stats/latency"
	"github.com/splitio/split-synchronizer/splitio/storage/boltdb"
	"github.com/splitio/split-synchronizer/splitio/storage/boltdb/collections"
	"github.com/splitio/split-synchronizer/splitio/task"
)

var controllerLatenciesBkt = latency.NewLatencyBucket()
var controllerLatencies = latency.NewLatency()
var controllerCounters = counter.NewCounter()
var controllerLocalCounters = counter.NewLocalCounter()

const latencyAddImpressionsInBuffer = "goproxyAddImpressionsInBuffer.time"
const latencyPostSDKLatencies = "goproxyPostSDKLatencies.time"
const latencyPostSDKCounters = "goproxyPostSDKCounters.time"
const latencyPostSDKLatency = "goproxyPostSDKTime.time"
const latencyPostSDKCount = "goproxyPostSDKCount.time"
const latencyPostSDKGauge = "goproxyPostSDKGague.time"

//-----------------------------------------------------------------------------
// SPLIT CHANGES
//-----------------------------------------------------------------------------
func fetchSplitsFromDB(since int) ([]json.RawMessage, int64, error) {

	till := int64(since)
	splits := make([]json.RawMessage, 0)

	splitCollection := collections.NewSplitChangesCollection(boltdb.DBB)
	items, err := splitCollection.FetchAll()
	if err != nil {
		return splits, till, err
	}

	for _, split := range items {
		if split.ChangeNumber > int64(since) {
			if split.ChangeNumber > till {
				till = split.ChangeNumber
			}
			splits = append(splits, []byte(split.JSON))
		}
	}

	return splits, till, nil
}

func splitChanges(c *gin.Context) {
	sinceParam := c.DefaultQuery("since", "-1")
	since, err := strconv.Atoi(sinceParam)
	if err != nil {
		since = -1
	}

	startTime := controllerLatencies.StartMeasuringLatency()
	splits, till, errf := fetchSplitsFromDB(since)
	if errf != nil {
		switch errf {
		case boltdb.ErrorBucketNotFound:
			log.Warning.Println("Maybe Splits are not yet synchronized")
		default:
			log.Error.Println(errf)
		}
		controllerCounters.Increment("splitChangeFetcher.status.500")
		controllerLocalCounters.Increment("request.error")
		c.JSON(http.StatusInternalServerError, gin.H{"error": errf.Error()})
		return
	}
	controllerLatencies.RegisterLatency("splitChangeFetcher.time", startTime)
	controllerCounters.Increment("splitChangeFetcher.status.200")
	controllerLocalCounters.Increment("request.ok")
	controllerLatenciesBkt.RegisterLatency("/api/splitChanges", startTime)
	c.JSON(http.StatusOK, gin.H{"splits": splits, "since": since, "till": till})
}

//-----------------------------------------------------------------------------
// SEGMENT CHANGES
//-----------------------------------------------------------------------------

func fetchSegmentsFromDB(since int, segmentName string) ([]string, []string, int64, error) {
	added := make([]string, 0)
	removed := make([]string, 0)
	till := int64(since)

	segmentCollection := collections.NewSegmentChangesCollection(boltdb.DBB)
	item, err := segmentCollection.Fetch(segmentName)
	if err != nil {
		switch err {
		case boltdb.ErrorBucketNotFound:
			log.Warning.Printf("Bucket not found for segment [%s]\n", segmentName)
		default:
			log.Error.Println(err)
		}
		return added, removed, till, err
	}

	if item == nil {
		return added, removed, till, err
	}

	for _, skey := range item.Keys {
		if skey.ChangeNumber > int64(since) {
			if skey.Removed {
				if since > 0 {
					removed = append(removed, skey.Name)
				}
			} else {
				added = append(added, skey.Name)
			}

			if since > 0 {
				if skey.ChangeNumber > till {
					till = skey.ChangeNumber
				}
			} else {
				if !skey.Removed && skey.ChangeNumber > till {
					till = skey.ChangeNumber
				}
			}
		}
	}

	return added, removed, till, nil
}

func segmentChanges(c *gin.Context) {
	sinceParam := c.DefaultQuery("since", "-1")
	since, err := strconv.Atoi(sinceParam)
	if err != nil {
		since = -1
	}

	segmentName := c.Param("name")
	startTime := controllerLatencies.StartMeasuringLatency()
	added, removed, till, errf := fetchSegmentsFromDB(since, segmentName)
	if errf != nil {
		controllerCounters.Increment("segmentChangeFetcher.status.500")
		controllerLocalCounters.Increment("request.error")
		c.JSON(http.StatusNotFound, gin.H{"error": errf.Error()})
		return
	}
	controllerLatencies.RegisterLatency("segmentChangeFetcher.time", startTime)
	controllerCounters.Increment("segmentChangeFetcher.status.200")
	controllerLocalCounters.Increment("request.ok")
	controllerLatenciesBkt.RegisterLatency("/api/segmentChanges/*", startTime)
	c.JSON(http.StatusOK, gin.H{"name": segmentName, "added": added,
		"removed": removed, "since": since, "till": till})
}

//-----------------------------------------------------------------------------
// MY SEGMENTS
//-----------------------------------------------------------------------------
func mySegments(c *gin.Context) {
	startTime := controllerLatenciesBkt.StartMeasuringLatency()
	key := c.Param("key")
	var mysegments = make([]api.MySegmentDTO, 0)

	segmentCollection := collections.NewSegmentChangesCollection(boltdb.DBB)
	segments, errs := segmentCollection.FetchAll()
	if errs != nil {
		log.Warning.Println(errs)
		controllerCounters.Increment("mySegments.status.500")
		controllerLocalCounters.Increment("request.error")
	} else {
		for _, segment := range segments {
			for _, skey := range segment.Keys {
				if !skey.Removed && skey.Name == key {
					mysegments = append(mysegments, api.MySegmentDTO{Name: segment.Name})
					break
				}
			}
		}
	}

	controllerCounters.Increment("mySegments.status.200")
	controllerLocalCounters.Increment("request.ok")
	controllerLatenciesBkt.RegisterLatency("/api/mySegments/*", startTime)
	c.JSON(http.StatusOK, gin.H{"mySegments": mysegments})
}

//-----------------------------------------------------------------
//                 I M P R E S S I O N S
//-----------------------------------------------------------------
func postImpressionBulk(impressionListenerEnabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		sdkVersion := c.Request.Header.Get("SplitSDKVersion")
		machineIP := c.Request.Header.Get("SplitSDKMachineIP")
		machineName := c.Request.Header.Get("SplitSDKMachineName")
		data, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			log.Error.Println(err)
			controllerLocalCounters.Increment("request.error")
			c.JSON(http.StatusInternalServerError, nil)
			return
		}
		if impressionListenerEnabled {
			err = task.QueueImpressionsForListener(&task.ImpressionBulk{
				Data:        json.RawMessage(data),
				SdkVersion:  sdkVersion,
				MachineIP:   machineIP,
				MachineName: machineName,
			})
		}

		startTime := controllerLatencies.StartMeasuringLatency()
		controllers.AddImpressions(data, sdkVersion, machineIP, machineName)
		controllerLatencies.RegisterLatency(latencyAddImpressionsInBuffer, startTime)
		controllerLocalCounters.Increment("request.ok")
		controllerLatenciesBkt.RegisterLatency("/api/testImpressions/bulk", startTime)
		c.JSON(http.StatusOK, nil)
	}
}

//-----------------------------------------------------------------------------
// METRICS
//-----------------------------------------------------------------------------

func postMetricsTimes(c *gin.Context) {
	startTime := controllerLatencies.StartMeasuringLatency()
	postEvent(c, api.PostMetricsLatency)
	controllerLatencies.RegisterLatency(latencyPostSDKLatencies, startTime)
	controllerLocalCounters.Increment("request.ok")
	controllerLatenciesBkt.RegisterLatency("/api/metrics/times", startTime)
	c.JSON(http.StatusOK, "")
}

func postMetricsTime(c *gin.Context) {
	startTime := controllerLatencies.StartMeasuringLatency()
	postEvent(c, api.PostMetricsTime)
	controllerLatencies.RegisterLatency(latencyPostSDKLatency, startTime)
	controllerLocalCounters.Increment("request.ok")
	controllerLatenciesBkt.RegisterLatency("/api/metrics/time", startTime)
	c.JSON(http.StatusOK, "")
}

func postMetricsCounters(c *gin.Context) {
	startTime := controllerLatencies.StartMeasuringLatency()
	postEvent(c, api.PostMetricsCounters)
	controllerLatencies.RegisterLatency(latencyPostSDKCounters, startTime)
	controllerLocalCounters.Increment("request.ok")
	controllerLatenciesBkt.RegisterLatency("/api/metrics/counters", startTime)
	c.JSON(http.StatusOK, "")
}

func postMetricsCounter(c *gin.Context) {
	startTime := controllerLatencies.StartMeasuringLatency()
	postEvent(c, api.PostMetricsCount)
	controllerLatencies.RegisterLatency(latencyPostSDKCount, startTime)
	controllerLocalCounters.Increment("request.ok")
	controllerLatenciesBkt.RegisterLatency("/api/metrics/counter", startTime)
	c.JSON(http.StatusOK, "")
}

func postMetricsGauge(c *gin.Context) {
	startTime := controllerLatencies.StartMeasuringLatency()
	postEvent(c, api.PostMetricsGauge)
	controllerLatencies.RegisterLatency(latencyPostSDKGauge, startTime)
	controllerLocalCounters.Increment("request.ok")
	controllerLatenciesBkt.RegisterLatency("/api/metrics/gauge", startTime)
	c.JSON(http.StatusOK, "")
}

func postEvent(c *gin.Context, fn func([]byte, string, string) error) {
	sdkVersion := c.Request.Header.Get("SplitSDKVersion")
	machineIP := c.Request.Header.Get("SplitSDKMachineIP")
	data, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		log.Error.Println(err)
	}

	go func() {
		log.Debug.Println(sdkVersion, machineIP, string(data))
		var e = fn(data, sdkVersion, machineIP)
		if e != nil {
			log.Error.Println(e)
			controllerLocalCounters.Increment("request.error")
		}
	}()
}

//-----------------------------------------------------------------------------
// ADMIN
//-----------------------------------------------------------------------------

func healthCheck(c *gin.Context) {

	proxyStatus := make(map[string]interface{})
	//cdnStatus := make(map[string]interface{})

	// Producer service
	proxyStatus["healthy"] = true
	proxyStatus["message"] = "Proxy service working as expected"

	c.JSON(http.StatusOK, gin.H{"proxy": proxyStatus})

}

func showStats(c *gin.Context) {
	counters := stats.Counters()
	latencies := stats.Latencies()
	c.JSON(http.StatusOK, gin.H{"counters": counters, "latencies": latencies})
}

func showDashboardSegmentKeys(c *gin.Context) {
	segmentName := c.Param("segment")
	var toReturn = ""
	segmentCollection := collections.NewSegmentChangesCollection(boltdb.DBB)
	segment, errs := segmentCollection.Fetch(segmentName)
	if errs != nil {
		log.Warning.Println(errs)
	} else {
		for _, key := range segment.Keys {
			toReturn += dashboard.ParseSegmentKey(key)
		}
	}
	c.String(http.StatusOK, "%s", toReturn)
}

func showDashboard(c *gin.Context) {
	counters := stats.Counters()
	latencies := stats.Latencies()

	htmlString := dashboard.HTML
	htmlString = strings.Replace(htmlString, "{{uptime}}", stats.UptimeFormated(), 1)
	htmlString = strings.Replace(htmlString, "{{proxy_errors}}", strconv.Itoa(int(log.ErrorDashboard.Counts())), 1)
	htmlString = strings.Replace(htmlString, "{{proxy_version}}", splitio.Version, 1)
	htmlString = strings.Replace(htmlString, "{{lastErrorsRows}}", dashboard.ParseLastErrors(log.ErrorDashboard.Messages()), 1)

	//---> SDKs stats

	htmlString = strings.Replace(htmlString, "{{request_ok}}", strconv.Itoa(int(counters["request.ok"])), 1)
	htmlString = strings.Replace(htmlString, "{{request_ok_formated}}", dashboard.FormatNumber(counters["request.ok"]), 1)
	htmlString = strings.Replace(htmlString, "{{request_error}}", strconv.Itoa(int(counters["request.error"])), 1)
	htmlString = strings.Replace(htmlString, "{{request_error_formated}}", dashboard.FormatNumber(counters["request.error"]), 1)
	htmlString = strings.Replace(htmlString, "{{sdks_total_requests}}", dashboard.FormatNumber(counters["request.ok"]+counters["request.error"]), 1)

	//latenciesGroupData
	var latenciesGroupData string
	if ldata, ok := latencies["/api/splitChanges"]; ok {
		latenciesGroupData += dashboard.ParseLatencyBktDataSerie("/api/splitChanges",
			ldata,
			"rgba(255, 159, 64, 0.2)",
			"rgba(255, 159, 64, 1)")
	}

	if ldata, ok := latencies["/api/segmentChanges/*"]; ok {
		latenciesGroupData += dashboard.ParseLatencyBktDataSerie("/api/segmentChanges/*",
			ldata,
			"rgba(54, 162, 235, 0.2)",
			"rgba(54, 162, 235, 1)")
	}

	if ldata, ok := latencies["/api/testImpressions/bulk"]; ok {
		latenciesGroupData += dashboard.ParseLatencyBktDataSerie("/api/testImpressions/bulk",
			ldata,
			"rgba(75, 192, 192, 0.2)",
			"rgba(75, 192, 192, 1)")
	}

	if ldata, ok := latencies["/api/mySegments/*"]; ok {
		latenciesGroupData += dashboard.ParseLatencyBktDataSerie("/api/mySegments/*",
			ldata,
			"rgba(153, 102, 255, 0.2)",
			"rgba(153, 102, 255, 1)")
	}

	htmlString = strings.Replace(htmlString, "{{latenciesGroupData}}", latenciesGroupData, 1)

	//---> Backend stats

	htmlString = strings.Replace(htmlString, "{{backend_request_ok}}", strconv.Itoa(int(counters["backend::request.ok"])), 1)
	htmlString = strings.Replace(htmlString, "{{backend_request_ok_formated}}", dashboard.FormatNumber(counters["backend::request.ok"]), 1)
	htmlString = strings.Replace(htmlString, "{{backend_request_error}}", strconv.Itoa(int(counters["backend::request.error"])), 1)
	htmlString = strings.Replace(htmlString, "{{backend_request_error_formated}}", dashboard.FormatNumber(counters["backend::request.error"]), 1)

	var latenciesGroupDataBackend string
	if ldata, ok := latencies["backend::/api/splitChanges"]; ok {
		latenciesGroupDataBackend += dashboard.ParseLatencyBktDataSerie("/api/splitChanges",
			ldata,
			"rgba(255, 159, 64, 0.2)",
			"rgba(255, 159, 64, 1)")
	}

	if ldata, ok := latencies["backend::/api/segmentChanges"]; ok {
		latenciesGroupDataBackend += dashboard.ParseLatencyBktDataSerie("/api/segmentChanges/*",
			ldata,
			"rgba(54, 162, 235, 0.2)",
			"rgba(54, 162, 235, 1)")
	}

	if ldata, ok := latencies["backend::/api/testImpressions/bulk"]; ok {
		latenciesGroupDataBackend += dashboard.ParseLatencyBktDataSerie("/api/testImpressions/bulk",
			ldata,
			"rgba(75, 192, 192, 0.2)",
			"rgba(75, 192, 192, 1)")
	}

	htmlString = strings.Replace(htmlString, "{{latenciesGroupDataBackend}}", latenciesGroupDataBackend, 1)

	splitRows := ""
	splitCollection := collections.NewSplitChangesCollection(boltdb.DBB)
	splits, err := splitCollection.FetchAll()
	if err != nil {
		//	log.Warning.Println(err)
		htmlString = strings.Replace(htmlString, "{{splits_number}}", "0", 1)
	} else {
		htmlString = strings.Replace(htmlString, "{{splits_number}}", strconv.Itoa(len(splits)), 1)
		for _, split := range splits {
			splitRows += dashboard.ParseSplit(split.JSON)
		}
	}
	htmlString = strings.Replace(htmlString, "{{splitRows}}", splitRows, 1)

	segmentsRows := ""
	segmentCollection := collections.NewSegmentChangesCollection(boltdb.DBB)
	segments, errs := segmentCollection.FetchAll()
	if errs != nil {
		log.Warning.Println(errs, "- Maybe segments are not yet synchronized.")
		htmlString = strings.Replace(htmlString, "{{segments_number}}", "0", 1)
	} else {
		htmlString = strings.Replace(htmlString, "{{segments_number}}", strconv.Itoa(len(segments)), 1)
		for _, segment := range segments {
			segmentsRows += dashboard.ParseSegment(segment)
		}
	}
	htmlString = strings.Replace(htmlString, "{{segmentRows}}", segmentsRows, 1)

	//Write your 200 header status (or other status codes, but only WriteHeader once)
	c.Writer.WriteHeader(http.StatusOK)
	//Convert your cached html string to byte array
	c.Writer.Write([]byte(htmlString))
	return
}
