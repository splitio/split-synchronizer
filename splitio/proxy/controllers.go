package proxy

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-split-commons/dtos"
	"github.com/splitio/go-split-commons/util"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/common"
	"github.com/splitio/split-synchronizer/splitio/proxy/boltdb"
	"github.com/splitio/split-synchronizer/splitio/proxy/boltdb/collections"
	"github.com/splitio/split-synchronizer/splitio/proxy/controllers"
	"github.com/splitio/split-synchronizer/splitio/proxy/interfaces"
	"github.com/splitio/split-synchronizer/splitio/task"
)

const (
	split          = "sdk.splitChanges"
	segment        = "sdk.segmentChanges"
	mySegment      = "sdk.mySegments"
	impressions    = "sdk.impressions"
	events         = "sdk.events"
	metricTime     = "sdk.metrics.time"
	metricLatency  = "sdk.metrics.times"
	metricCounter  = "sdk.metrics.counter"
	metricCounters = "sdk.metrics.counters"
	metricGauge    = "sdk.metrics.gauge"
	localAPIOK     = "sdk.request.ok"
	localAPIError  = "sdk.request.error"
)

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
			log.Instance.Warning("Maybe Splits are not yet synchronized")
		default:
			log.Instance.Error(errf)
		}
		interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncCounter(localAPIError)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errf.Error()})
		return
	}
	bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncLatency(split, bucket)
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncCounter(localAPIOK)
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
			log.Instance.Warning("Bucket not found for segment [%s]\n", segmentName)
		default:
			log.Instance.Error(err)
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
		interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncCounter(localAPIError)
		c.JSON(http.StatusNotFound, gin.H{"error": errf.Error()})
		return
	}
	bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncLatency(segment, bucket)
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncCounter(localAPIOK)
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
		log.Instance.Warning(errs)
		interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncCounter(localAPIError)
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
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncLatency(mySegment, bucket)
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncCounter(localAPIOK)
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
	impressionsMode string,
	data []byte,
) {
	if impressionListenerEnabled {
		var rawPayload []dtos.ImpressionsDTO
		err := json.Unmarshal(data, &rawPayload)
		if err == nil && rawPayload != nil && len(rawPayload) > 0 {
			impressionsListenerDTO := make([]common.ImpressionsListener, 0, len(rawPayload))
			for _, impressionsDTO := range rawPayload {
				impressionListenerDTO := make([]common.ImpressionListener, 0, len(impressionsDTO.KeyImpressions))
				for _, impression := range impressionsDTO.KeyImpressions {
					impressionListenerDTO = append(impressionListenerDTO, common.ImpressionListener{
						BucketingKey: impression.BucketingKey,
						ChangeNumber: impression.ChangeNumber,
						KeyName:      impression.KeyName,
						Label:        impression.Label,
						Pt:           impression.Pt,
						Time:         impression.Time,
						Treatment:    impression.Treatment,
					})
				}
				impressionsListenerDTO = append(impressionsListenerDTO, common.ImpressionsListener{
					TestName:       impressionsDTO.TestName,
					KeyImpressions: impressionListenerDTO,
				})
			}

			serializedImpression, err := json.Marshal(impressionsListenerDTO)
			if err == nil {
				_ = task.QueueImpressionsForListener(&task.ImpressionBulk{
					Data:        json.RawMessage(serializedImpression),
					SdkVersion:  sdkVersion,
					MachineIP:   machineIP,
					MachineName: machineName,
				})
			}
		}
	}

	before := time.Now()
	controllers.AddImpressions(data, sdkVersion, machineIP, machineName, impressionsMode)
	bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncLatency(impressions, bucket)
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncCounter(localAPIOK)
}

func postImpressionBulk(impressionListenerEnabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		sdkVersion := c.Request.Header.Get("SplitSDKVersion")
		machineIP := c.Request.Header.Get("SplitSDKMachineIP")
		machineName := c.Request.Header.Get("SplitSDKMachineName")
		impressionsMode := c.Request.Header.Get("SplitSDKImpressionsMode")
		data, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			log.Instance.Error(err)
			interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncCounter(localAPIError)
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

		submitImpressions(impressionListenerEnabled, sdkVersion, machineIP, machineName, impressionsMode, data)
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
			log.Instance.Error(err)
			interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncCounter(localAPIError)
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
			log.Instance.Error(err)
			c.JSON(http.StatusBadRequest, nil)
			return
		}

		if !validateAPIKey(keys, body.Token) {
			c.AbortWithStatus(401)
			return
		}

		impressions, err := json.Marshal(body.Entries)
		if err != nil {
			log.Instance.Error(err)
			c.JSON(http.StatusInternalServerError, nil)
			return
		}

		submitImpressions(impressionListenerEnabled, body.Sdk, "NA", "NA", "", impressions)
		c.JSON(http.StatusNoContent, nil)
	}
}

func postImpressionsCount() gin.HandlerFunc {
	return func(c *gin.Context) {
		sdkVersion := c.Request.Header.Get("SplitSDKVersion")
		machineIP := c.Request.Header.Get("SplitSDKMachineIP")
		machineName := c.Request.Header.Get("SplitSDKMachineName")
		data, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			log.Instance.Error(err)
			c.JSON(http.StatusInternalServerError, nil)
			return
		}

		err = controllers.PostImpressionsCount(sdkVersion, machineIP, machineName, data)
		if err != nil {
			if httpError, ok := err.(*dtos.HTTPError); ok {
				c.JSON(httpError.Code, nil)
			} else {
				c.JSON(http.StatusInternalServerError, nil)
			}
			return
		}
		c.JSON(http.StatusOK, nil)
	}
}

func postImpressionsCountBeacon(keys []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body == nil {
			c.JSON(http.StatusBadRequest, nil)
			return
		}

		data, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			log.Instance.Error(err)
			c.JSON(http.StatusInternalServerError, nil)
			return
		}

		type BeaconImpressionsCount struct {
			Entries dtos.ImpressionsCountDTO `json:"entries"`
			Sdk     string                   `json:"sdk"`
			Token   string                   `json:"token"`
		}
		var body BeaconImpressionsCount
		if err := json.Unmarshal([]byte(data), &body); err != nil {
			log.Instance.Error(err)
			c.JSON(http.StatusBadRequest, nil)
			return
		}

		if !validateAPIKey(keys, body.Token) {
			c.AbortWithStatus(401)
			return
		}

		impressionsCount, err := json.Marshal(body.Entries)
		if err != nil {
			log.Instance.Error(err)
			c.JSON(http.StatusInternalServerError, nil)
			return
		}

		if len(body.Entries.PerFeature) == 0 {
			c.JSON(http.StatusNoContent, nil)
			return
		}

		controllers.PostImpressionsCount(body.Sdk, "NA", "NA", impressionsCount)
		if err != nil {
			if httpError, ok := err.(*dtos.HTTPError); ok {
				c.JSON(httpError.Code, nil)
			} else {
				c.JSON(http.StatusInternalServerError, nil)
			}
		}
		c.JSON(http.StatusNoContent, nil)
	}
}

//-----------------------------------------------------------------------------
// METRICS
//-----------------------------------------------------------------------------

func postMetricsTimes(c *gin.Context) {
	before := time.Now()
	postEvent(c, "/metrics/times")
	bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncLatency(metricLatency, bucket)
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncCounter(localAPIOK)
	c.JSON(http.StatusOK, "")
}

func postMetricsTime(c *gin.Context) {
	before := time.Now()
	postEvent(c, "/metrics/time")
	bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncLatency(metricTime, bucket)
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncCounter(localAPIOK)
	c.JSON(http.StatusOK, "")
}

func postMetricsCounters(c *gin.Context) {
	before := time.Now()
	postEvent(c, "/metrics/counters")
	bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncLatency(metricCounters, bucket)
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncCounter(localAPIOK)
	c.JSON(http.StatusOK, "")
}

func postMetricsCounter(c *gin.Context) {
	before := time.Now()
	postEvent(c, "/metrics/counter")
	bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncLatency(metricCounter, bucket)
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncCounter(localAPIOK)
	c.JSON(http.StatusOK, "")
}

func postMetricsGauge(c *gin.Context) {
	before := time.Now()
	postEvent(c, "/metrics/gauge")
	bucket := util.Bucket(time.Now().Sub(before).Nanoseconds())
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncLatency(metricGauge, bucket)
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncCounter(localAPIOK)
	c.JSON(http.StatusOK, "")
}

func postEvent(c *gin.Context, url string) {
	metadata := dtos.Metadata{
		SDKVersion: c.Request.Header.Get("SplitSDKVersion"),
		MachineIP:  c.Request.Header.Get("SplitSDKMachineIP"),
	}
	data, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		log.Instance.Error(err)
	}

	go func() {
		log.Instance.Debug(metadata.SDKVersion, metadata.MachineIP, string(data))
		var e = interfaces.MetricsRecorder.RecordRaw(url, data, metadata, nil)
		if e != nil {
			log.Instance.Error(e)
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
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncLatency(events, bucket)
	interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncCounter(localAPIOK)
}

func postEvents(c *gin.Context) {
	sdkVersion := c.Request.Header.Get("SplitSDKVersion")
	machineIP := c.Request.Header.Get("SplitSDKMachineIP")
	machineName := c.Request.Header.Get("SplitSDKMachineName")
	data, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		log.Instance.Error(err)
		interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncCounter(localAPIError)
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
			log.Instance.Error(err)
			interfaces.ProxyTelemetryWrapper.LocalTelemetry.IncCounter(localAPIError)
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
			log.Instance.Error(err)
			c.JSON(http.StatusBadRequest, nil)
			return
		}

		if !validateAPIKey(keys, body.Token) {
			c.AbortWithStatus(401)
			return
		}

		events, err := json.Marshal(body.Entries)
		if err != nil {
			log.Instance.Error(err)
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
