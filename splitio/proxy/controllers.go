package proxy

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-split-commons/dtos"
	"github.com/splitio/go-split-commons/service/api"
	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/go-split-commons/util"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/proxy/boltdb"
	"github.com/splitio/split-synchronizer/splitio/proxy/boltdb/collections"
	"github.com/splitio/split-synchronizer/splitio/proxy/controllers"
	"github.com/splitio/split-synchronizer/splitio/proxy/interfaces"
	"github.com/splitio/split-synchronizer/splitio/task"
)

const (
	metricTime     = "/metrics/time"
	metricLatency  = "/metrics/times"
	metricCounter  = "/metrics/counter"
	metricCounters = "/metrics/counters"
	metricGauge    = "/metrics/gauge"
	localAPIOK     = "request.ok"
	localAPIError  = "request.error"
)

var metricsRecorder = api.NewHTTPMetricsRecorder(conf.Data.APIKey, interfaces.GetAdvancedConfig(), interfaces.Logger)

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

	before := time.Now()
	splits, till, errf := fetchSplitsFromDB(since)
	if errf != nil {
		switch errf {
		case boltdb.ErrorBucketNotFound:
			log.Warning.Println("Maybe Splits are not yet synchronized")
		default:
			log.Error.Println(errf)
		}
		interfaces.ProxyTelemetryWrapper.StoreCounters(storage.SplitChangesCounter, "500", true)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errf.Error()})
		return
	}
	bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
	interfaces.ProxyTelemetryWrapper.LocalTelemtry.IncCounter("")
	interfaces.ProxyTelemetryWrapper.Telemetry.IncCounter("")
	interfaces.ProxyTelemetryWrapper.StoreLatencies(storage.SplitChangesLatency, bucket, true)
	interfaces.ProxyTelemetryWrapper.StoreCounters(storage.SplitChangesCounter, "ok", true)
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
	before := time.Now()
	added, removed, till, errf := fetchSegmentsFromDB(since, segmentName)
	if errf != nil {
		interfaces.ProxyTelemetryWrapper.StoreCounters(storage.SegmentChangesCounter, "500", true)
		c.JSON(http.StatusNotFound, gin.H{"error": errf.Error()})
		return
	}
	bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
	interfaces.ProxyTelemetryWrapper.StoreLatencies(storage.SegmentChangesLatency, bucket, true)
	interfaces.ProxyTelemetryWrapper.StoreCounters(storage.SegmentChangesCounter, "ok", true)
	c.JSON(http.StatusOK, gin.H{"name": segmentName, "added": added,
		"removed": removed, "since": since, "till": till})
}

//-----------------------------------------------------------------------------
// MY SEGMENTS
//-----------------------------------------------------------------------------
func mySegments(c *gin.Context) {
	before := time.Now()
	key := c.Param("key")
	var mysegments = make([]dtos.MySegmentDTO, 0)

	segmentCollection := collections.NewSegmentChangesCollection(boltdb.DBB)
	segments, errs := segmentCollection.FetchAll()
	if errs != nil {
		log.Warning.Println(errs)
		interfaces.ProxyTelemetryWrapper.StoreCounters(storage.MySegmentsCounter, "500", true)
	} else {
		for _, segment := range segments {
			for _, skey := range segment.Keys {
				if !skey.Removed && skey.Name == key {
					mysegments = append(mysegments, dtos.MySegmentDTO{Name: segment.Name})
					break
				}
			}
		}
	}

	bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
	interfaces.ProxyTelemetryWrapper.StoreLatencies(storage.MySegmentsLatency, bucket, true)
	interfaces.ProxyTelemetryWrapper.StoreCounters(storage.MySegmentsCounter, "ok", true)
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

	before := time.Now()
	controllers.AddImpressions(data, sdkVersion, machineIP, machineName)
	bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
	interfaces.ProxyTelemetryWrapper.StoreLatencies(storage.TestImpressionsLatency, bucket, true)
	interfaces.ProxyTelemetryWrapper.StoreCounters(storage.TestImpressionsCounter, "ok", true)
}

func postImpressionBulk(impressionListenerEnabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		sdkVersion := c.Request.Header.Get("SplitSDKVersion")
		machineIP := c.Request.Header.Get("SplitSDKMachineIP")
		machineName := c.Request.Header.Get("SplitSDKMachineName")
		data, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			log.Error.Println(err)
			interfaces.ProxyTelemetryWrapper.StoreCounters(storage.TestImpressionsCounter, "500", true)
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
			interfaces.ProxyTelemetryWrapper.StoreCounters(storage.TestImpressionsCounter, "500", true)
			c.JSON(http.StatusInternalServerError, nil)
			return
		}

		type BeaconImpressions struct {
			Entries []dtos.ImpressionsDTO `json:"entries"`
			Sdk     string                `json:"sdk"`
			Token   string                `json:"token"`
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
	postEvent(c, metricLatency)
	// startTime := controllerLatencies.StartMeasuringLatency()
	// controllerLatencies.RegisterLatency(latencyPostSDKLatencies, startTime)
	// controllerLocalCounters.Increment("request.ok")
	// controllerLatenciesBkt.RegisterLatency("/api/metrics/times", startTime)
	c.JSON(http.StatusOK, "")
}

func postMetricsTime(c *gin.Context) {
	postEvent(c, metricTime)
	// startTime := controllerLatencies.StartMeasuringLatency()
	// controllerLatencies.RegisterLatency(latencyPostSDKLatency, startTime)
	// controllerLocalCounters.Increment("request.ok")
	// controllerLatenciesBkt.RegisterLatency("/api/metrics/time", startTime)
	c.JSON(http.StatusOK, "")
}

func postMetricsCounters(c *gin.Context) {
	postEvent(c, metricCounters)
	// controllerLatencies.RegisterLatency(latencyPostSDKCounters, startTime)
	// controllerLocalCounters.Increment("request.ok")
	// controllerLatenciesBkt.RegisterLatency("/api/metrics/counters", startTime)
	c.JSON(http.StatusOK, "")
}

func postMetricsCounter(c *gin.Context) {
	postEvent(c, metricCounter)
	// controllerLatencies.RegisterLatency(latencyPostSDKCount, startTime)
	// controllerLocalCounters.Increment("request.ok")
	// controllerLatenciesBkt.RegisterLatency("/api/metrics/counter", startTime)
	c.JSON(http.StatusOK, "")
}

func postMetricsGauge(c *gin.Context) {
	postEvent(c, metricGauge)
	// controllerLatencies.RegisterLatency(latencyPostSDKGauge, startTime)
	// controllerLocalCounters.Increment("request.ok")
	// controllerLatenciesBkt.RegisterLatency("/api/metrics/gauge", startTime)
	c.JSON(http.StatusOK, "")
}

func postEvent(c *gin.Context, url string) {
	metadata := dtos.Metadata{
		SDKVersion: c.Request.Header.Get("SplitSDKVersion"),
		MachineIP:  c.Request.Header.Get("SplitSDKMachineIP"),
	}
	data, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		log.Error.Println(err)
	}

	go func() {
		log.Debug.Println(metadata.SDKVersion, metadata.MachineIP, string(data))
		var e = metricsRecorder.RecordRaw(url, data, metadata)
		if e != nil {
			log.Error.Println(e)
			// ProxyTelemetryWrapper.StoreCounters(storage., "500")
		}
	}()
}

//-----------------------------------------------------------------------------
// EVENTS - RESULTS
//-----------------------------------------------------------------------------
func submitEvents(sdkVersion string, machineIP string, machineName string, data []byte) {
	before := time.Now()
	controllers.AddEvents(data, sdkVersion, machineIP, machineName)
	bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
	interfaces.ProxyTelemetryWrapper.StoreLatencies(storage.PostEventsLatency, bucket, true)
	interfaces.ProxyTelemetryWrapper.StoreCounters(storage.PostEventsCounter, "ok", true)
}

func postEvents(c *gin.Context) {
	sdkVersion := c.Request.Header.Get("SplitSDKVersion")
	machineIP := c.Request.Header.Get("SplitSDKMachineIP")
	machineName := c.Request.Header.Get("SplitSDKMachineName")
	data, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		log.Error.Println(err)
		interfaces.ProxyTelemetryWrapper.StoreCounters(storage.PostEventsCounter, "500", true)
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
			interfaces.ProxyTelemetryWrapper.StoreCounters(storage.PostEventsCounter, "500", true)
			c.JSON(http.StatusInternalServerError, nil)
			return
		}

		type BeaconEvents struct {
			Entries []dtos.EventDTO `json:"entries"`
			Sdk     string          `json:"sdk"`
			Token   string          `json:"token"`
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

func auth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"pushEnabled": false, "token": ""})
}
