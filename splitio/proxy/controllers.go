package proxy

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio"
	"github.com/splitio/go-agent/splitio/api"
	"github.com/splitio/go-agent/splitio/proxy/controllers"
	"github.com/splitio/go-agent/splitio/stats"
	"github.com/splitio/go-agent/splitio/stats/counter"
	"github.com/splitio/go-agent/splitio/stats/dashboard"
	"github.com/splitio/go-agent/splitio/stats/latency"
	"github.com/splitio/go-agent/splitio/storage/boltdb"
	"github.com/splitio/go-agent/splitio/storage/boltdb/collections"
)

var controllerLatenciesBkt = latency.NewLatencyBucket()
var controllerLatencies = latency.NewLatency()
var controllerCounters = counter.NewCounter()

const latencyFetchSplitsFromDB = "goproxy.FetchSplitsFromBoltDB"
const latencyFetchSegmentFromDB = "goproxy.FetchSegmentFromBoltDB"
const latencyAddImpressionsInBuffer = "goproxy.AddImpressionsInBuffer"
const latencyPostSDKLatencies = "goproxy.PostSDKLatencies"
const latencyPostSDKCounters = "goproxy.PostSDKCounters"
const latencyPostSDKLatency = "goproxy.PostSDKTime"
const latencyPostSDKCount = "goproxy.PostSDKCount"
const latencyPostSDKGauge = "goproxy.PostSDKGague"

//-----------------------------------------------------------------------------
// SPLIT CHANGES
//-----------------------------------------------------------------------------
func fetchSplitsFromDB(since int) ([]json.RawMessage, int64, error) {

	till := int64(since)
	splits := make([]json.RawMessage, 0)

	splitCollection := collections.NewSplitChangesCollection(boltdb.DBB)
	items, err := splitCollection.FetchAll()
	if err != nil {
		log.Error.Println(err)
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
		log.Error.Println(errf)
		controllerCounters.Increment("splitChangeFetcher.status.500")
		controllerCounters.Increment("splitChangeFetcher.exception")
		controllerCounters.Increment("request.error")
		c.JSON(http.StatusInternalServerError, gin.H{"error": errf.Error()})
		return
	}
	controllerLatencies.RegisterLatency("splitChangeFetcher.time", startTime)
	controllerLatencies.RegisterLatency(latencyFetchSplitsFromDB, startTime)
	controllerCounters.Increment("splitChangeFetcher.status.200")
	controllerCounters.Increment("request.ok")
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
		controllerCounters.Increment("segmentChangeFetcher.exception")
		controllerCounters.Increment("request.error")
		c.JSON(http.StatusNotFound, gin.H{"error": errf.Error()})
		//c.JSON(http.StatusOK, gin.H{"name": segmentName, "added": added,
		//	"removed": removed, "since": since, "till": till})
		return
	}
	controllerLatencies.RegisterLatency("segmentChangeFetcher.time", startTime)
	controllerLatencies.RegisterLatency(latencyFetchSegmentFromDB, startTime)
	controllerCounters.Increment("segmentChangeFetcher.status.200")
	controllerCounters.Increment("request.ok")
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
		controllerCounters.Increment("request.error")
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
	controllerCounters.Increment("request.ok")
	controllerLatenciesBkt.RegisterLatency("/api/mySegments/*", startTime)
	c.JSON(http.StatusOK, gin.H{"mySegments": mysegments})
}

//-----------------------------------------------------------------
//                 I M P R E S S I O N S
//-----------------------------------------------------------------
func postBulkImpressions(c *gin.Context) {
	sdkVersion := c.Request.Header.Get("SplitSDKVersion")
	machineIP := c.Request.Header.Get("SplitSDKMachineIP")
	data, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		log.Error.Println(err)
		controllerCounters.Increment("request.error")
		c.JSON(http.StatusInternalServerError, nil)
	}
	startTime := controllerLatencies.StartMeasuringLatency()
	controllers.AddImpressions(data, sdkVersion, machineIP)
	controllerLatencies.RegisterLatency(latencyAddImpressionsInBuffer, startTime)
	controllerCounters.Increment("request.ok")
	controllerLatenciesBkt.RegisterLatency("/api/testImpressions/bulk", startTime)
	c.JSON(http.StatusOK, nil)
}

//-----------------------------------------------------------------------------
// METRICS
//-----------------------------------------------------------------------------

func postMetricsTimes(c *gin.Context) {
	startTime := controllerLatencies.StartMeasuringLatency()
	postEvent(c, api.PostMetricsLatency)
	controllerLatencies.RegisterLatency(latencyPostSDKLatencies, startTime)
	controllerCounters.Increment("request.ok")
	controllerLatenciesBkt.RegisterLatency("/api/metrics/times", startTime)
	c.JSON(http.StatusOK, "")
}

func postMetricsTime(c *gin.Context) {
	startTime := controllerLatencies.StartMeasuringLatency()
	postEvent(c, api.PostMetricsTime)
	controllerLatencies.RegisterLatency(latencyPostSDKLatency, startTime)
	controllerCounters.Increment("request.ok")
	controllerLatenciesBkt.RegisterLatency("/api/metrics/time", startTime)
	c.JSON(http.StatusOK, "")
}

func postMetricsCounters(c *gin.Context) {
	startTime := controllerLatencies.StartMeasuringLatency()
	postEvent(c, api.PostMetricsCounters)
	controllerLatencies.RegisterLatency(latencyPostSDKCounters, startTime)
	controllerCounters.Increment("request.ok")
	controllerLatenciesBkt.RegisterLatency("/api/metrics/counters", startTime)
	c.JSON(http.StatusOK, "")
}

func postMetricsCounter(c *gin.Context) {
	startTime := controllerLatencies.StartMeasuringLatency()
	postEvent(c, api.PostMetricsCount)
	controllerLatencies.RegisterLatency(latencyPostSDKCount, startTime)
	controllerCounters.Increment("request.ok")
	controllerLatenciesBkt.RegisterLatency("/api/metrics/counter", startTime)
	c.JSON(http.StatusOK, "")
}

func postMetricsGauge(c *gin.Context) {
	startTime := controllerLatencies.StartMeasuringLatency()
	postEvent(c, api.PostMetricsGauge)
	controllerLatencies.RegisterLatency(latencyPostSDKGauge, startTime)
	controllerCounters.Increment("request.ok")
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

	// TODO add channel to control number of posts
	go func() {
		log.Debug.Println(sdkVersion, machineIP, string(data))
		var e = fn(data, sdkVersion, machineIP)
		if e != nil {
			log.Error.Println(e)
		}
	}()
}

//-----------------------------------------------------------------------------
// ADMIN
//-----------------------------------------------------------------------------

func uptime(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"uptime": stats.UptimeFormated()})
}

func version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"version": splitio.Version})
}

func ping(c *gin.Context) {
	c.String(http.StatusOK, "%s", "pong")
}

func showStats(c *gin.Context) {
	counters := stats.Counters()
	latencies := stats.Latencies()
	c.JSON(http.StatusOK, gin.H{"counters": counters, "latencies": latencies})
}

func showDashboard(c *gin.Context) {
	counters := stats.Counters()
	latencies := stats.Latencies()

	htmlString := dashboard.HTML
	htmlString = strings.Replace(htmlString, "{{uptime}}", stats.UptimeFormated(), 1)
	htmlString = strings.Replace(htmlString, "{{proxy_errors}}", strconv.Itoa(int(log.ErrorDashboard.Counts())), 1)
	htmlString = strings.Replace(htmlString, "{{proxy_version}}", splitio.Version, 1)
	//fmt.Println("MESSAGESS!!!!---->", log.ErrorDashboard.Messages())

	htmlString = strings.Replace(htmlString, "{{lastErrorsRows}}", dashboard.ParseLastErrors(log.ErrorDashboard.Messages()), 1)

	//---> SDKs stats

	htmlString = strings.Replace(htmlString, "{{request_ok}}", strconv.Itoa(int(counters["request.ok"])), 2)
	htmlString = strings.Replace(htmlString, "{{request_error}}", strconv.Itoa(int(counters["request.error"])), 2)
	htmlString = strings.Replace(htmlString, "{{sdks_total_requests}}", strconv.Itoa(int(counters["request.ok"]+counters["request.error"])), 1)

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

	htmlString = strings.Replace(htmlString, "{{backend_request_ok}}", strconv.Itoa(int(counters["backend::request.ok"])), 2)
	htmlString = strings.Replace(htmlString, "{{backend_request_error}}", strconv.Itoa(int(counters["backend::request.error"])), 2)

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
		log.Warning.Println(err)
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
		log.Warning.Println(errs)
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
