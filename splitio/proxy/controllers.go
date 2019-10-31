package proxy

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/proxy/controllers"
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
const latencyAddEventsInBuffer = "goproxyAddEventsInBuffer.time"
const latencyPostSDKLatencies = "goproxyPostSDKLatencies.time"
const latencyPostSDKCounters = "goproxyPostSDKCounters.time"
const latencyPostSDKLatency = "goproxyPostSDKTime.time"
const latencyPostSDKCount = "goproxyPostSDKCount.time"
const latencyPostSDKGauge = "goproxyPostSDKGague.time"

func validateAPIKey(keys []string, apiKey string) bool {
	for _, key := range keys {
		if apiKey == key {
			return true
		}
	}

	return false
}

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
func submitImpressions(
	impressionListenerEnabled bool,
	sdkVersion string,
	machineIP string,
	machineName string,
	data []byte,
) {
	if impressionListenerEnabled {
		_ = task.QueueImpressionsForListener(&task.ImpressionBulk{
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
}

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

		submitImpressions(impressionListenerEnabled, sdkVersion, machineIP, machineName, data)
		c.JSON(http.StatusOK, nil)
	}
}

func postImpressionBeacon(keys []string, impressionListenerEnabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil {
			c.JSON(http.StatusBadRequest, nil)
			return
		}

		data, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			log.Error.Println(err)
			controllerLocalCounters.Increment("request.error")
			c.JSON(http.StatusInternalServerError, nil)
			return
		}

		type BeaconImpressions struct {
			Entries []api.ImpressionsDTO `json:"entries"`
			Sdk     string               `json:"sdk"`
			Token   string               `json:"token"`
		}
		var body BeaconImpressions
		if err := json.Unmarshal([]byte(data), &body); err != nil {
			log.Error.Println(err)
			c.JSON(http.StatusBadRequest, nil)
			return
		}

		if !validateAPIKey(keys, body.Token) {
			c.AbortWithStatus(401)
			return
		}

		impressions, err := json.Marshal(body.Entries)
		if err != nil {
			log.Error.Println(err)
			c.JSON(http.StatusInternalServerError, nil)
			return
		}

		submitImpressions(impressionListenerEnabled, body.Sdk, "NA", "NA", impressions)
		c.JSON(http.StatusNoContent, nil)
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
// EVENTS - RESULTS
//-----------------------------------------------------------------------------
func submitEvents(sdkVersion string, machineIP string, machineName string, data []byte) {
	startTime := controllerLatencies.StartMeasuringLatency()
	controllers.AddEvents(data, sdkVersion, machineIP, machineName)
	controllerLatencies.RegisterLatency(latencyAddEventsInBuffer, startTime)
	controllerLocalCounters.Increment("request.ok")
	controllerLatenciesBkt.RegisterLatency("/api/events/bulk", startTime)
}

func postEvents(c *gin.Context) {
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

	submitEvents(sdkVersion, machineIP, machineName, data)
	c.JSON(http.StatusOK, nil)
}

func postEventsBeacon(keys []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil {
			c.JSON(http.StatusBadRequest, nil)
			return
		}

		data, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			log.Error.Println(err)
			controllerLocalCounters.Increment("request.error")
			c.JSON(http.StatusInternalServerError, nil)
			return
		}

		type BeaconEvents struct {
			Entries []api.EventDTO `json:"entries"`
			Sdk     string         `json:"sdk"`
			Token   string         `json:"token"`
		}
		var body BeaconEvents
		if err := json.Unmarshal([]byte(data), &body); err != nil {
			log.Error.Println(err)
			c.JSON(http.StatusBadRequest, nil)
			return
		}

		if !validateAPIKey(keys, body.Token) {
			c.AbortWithStatus(401)
			return
		}

		events, err := json.Marshal(body.Entries)
		if err != nil {
			log.Error.Println(err)
			c.JSON(http.StatusInternalServerError, nil)
			return
		}

		submitEvents(body.Sdk, "NA", "NA", events)
		c.JSON(http.StatusNoContent, nil)
	}
}
