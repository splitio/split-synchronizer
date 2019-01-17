package producer

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/recorder"
	"github.com/splitio/split-synchronizer/splitio/storage"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
	"github.com/splitio/split-synchronizer/splitio/task"
	"github.com/splitio/split-synchronizer/splitio/web/admin/controllers"
	"github.com/splitio/split-synchronizer/splitio/web/dashboard"
)

// HealthCheck returns the service status
func HealthCheck(c *gin.Context) {
	producerStatus := make(map[string]interface{})
	storageStatus := make(map[string]interface{})

	// Producer service
	producerStatus["healthy"] = true
	producerStatus["message"] = "Synchronizer service working as expected"

	// Storage service
	splitStorage, exists := c.Get("SplitStorage")
	if exists {
		_, err := splitStorage.(storage.SplitStorage).ChangeNumber()
		if err != nil {
			storageStatus["healthy"] = false
			storageStatus["message"] = err.Error()
		} else {
			storageStatus["healthy"] = true
			storageStatus["message"] = "Storage service working as expected"
		}
	}

	sdkStatus := controllers.GetSdkStatus()
	eventsStatus := controllers.GetEventsStatus()

	if storageStatus["healthy"].(bool) && sdkStatus["healthy"].(bool) && eventsStatus["healthy"].(bool) {
		c.JSON(http.StatusOK, gin.H{"sync": producerStatus, "storage": storageStatus, "sdk": sdkStatus, "events": eventsStatus})
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"sync": producerStatus, "storage": storageStatus, "sdk": sdkStatus, "events": eventsStatus})
	}
}

// Dashboard returns a dashboard
func Dashboard(c *gin.Context) {

	// Storage service
	splitStorage, _ := c.Get("SplitStorage")
	segmentStorage, _ := c.Get("SegmentStorage")

	dash := dashboard.NewDashboard(conf.Data.Producer.Admin.Title, false,
		splitStorage.(storage.SplitStorage),
		segmentStorage.(storage.SegmentStorage),
	)

	//Write your 200 header status (or other status codes, but only WriteHeader once)
	c.Writer.WriteHeader(http.StatusOK)
	//Convert your cached html string to byte array
	c.Writer.Write([]byte(dash.HTML()))
	return
}

// DashboardSegmentKeys returns a keys for a given segment
func DashboardSegmentKeys(c *gin.Context) {

	segmentName := c.Param("segment")

	// Storage service
	splitStorage, _ := c.Get("SplitStorage")
	segmentStorage, _ := c.Get("SegmentStorage")

	dash := dashboard.NewDashboard(conf.Data.Producer.Admin.Title, false,
		splitStorage.(storage.SplitStorage),
		segmentStorage.(storage.SegmentStorage),
	)

	var toReturn = dash.HTMLSegmentKeys(segmentName)

	c.String(http.StatusOK, "%s", toReturn)
}

// GetEventsQueueSize returns events queue size
func GetEventsQueueSize(c *gin.Context) {
	if !conf.Data.Redis.DisableLegacyImpressions {
		log.Warning.Println("DisableLegacyImpressions is false: The size of events will only consider the events from the queue.")
	}

	eventsStorageAdapter := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	queueSize := eventsStorageAdapter.Size()
	c.JSON(http.StatusOK, gin.H{"queueSize": queueSize})
}

// GetImpressionsQueueSize returns impressions queue size
func GetImpressionsQueueSize(c *gin.Context) {
	if !conf.Data.Redis.DisableLegacyImpressions {
		log.Warning.Println("DisableLegacyImpressions is false: The size of impressions will only consider the impressions from the queue.")
	}

	impressionsStorageAdapter := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	queueSize := impressionsStorageAdapter.Size()
	c.JSON(http.StatusOK, gin.H{"queueSize": queueSize})
}

func getIntegerParameterFromQuery(c *gin.Context, key string) (*int64, error) {
	value := c.Query(key)
	if value != "" {
		field, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, errors.New("Wrong type passed as parameter")
		}
		return &field, nil
	}
	return nil, nil
}

// DropEvents drops Events from queue
func DropEvents(c *gin.Context) {
	eventsStorageAdapter := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	bulkSize, err := getIntegerParameterFromQuery(c, "size")
	if err != nil {
		c.String(http.StatusBadRequest, "%s", err.Error())
		return
	}
	if bulkSize != nil && *bulkSize < 1 {
		c.String(http.StatusBadRequest, "%s", "Size cannot be less than 1")
		return
	}
	err = eventsStorageAdapter.Drop(bulkSize)
	if err == nil {
		c.String(http.StatusOK, "%s", "Events dropped")
		return
	}
	c.String(http.StatusInternalServerError, "%s", err.Error())
}

// DropImpressions drops Impressions from queue
func DropImpressions(c *gin.Context) {
	impressionsStorageAdapter := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	bulkSize, err := getIntegerParameterFromQuery(c, "size")
	if err != nil {
		c.String(http.StatusBadRequest, "%s", err.Error())
		return
	}
	if bulkSize != nil && *bulkSize < 1 {
		c.String(http.StatusBadRequest, "%s", "Size cannot be less than 1")
		return
	}
	err = impressionsStorageAdapter.Drop(bulkSize)
	if err == nil {
		c.String(http.StatusOK, "%s", "Impressions dropped")
		return
	}
	c.String(http.StatusInternalServerError, "%s", err.Error())
}

// FlushEvents eviction of Events
func FlushEvents(c *gin.Context) {
	size, err := getIntegerParameterFromQuery(c, "size")
	if err != nil {
		c.String(http.StatusBadRequest, "%s", err.Error())
		return
	}
	if size != nil && *size < 1 {
		c.String(http.StatusBadRequest, "%s", "Size cannot be less than 1")
		return
	}
	eventsStorageAdapter := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	eventsRecorder := recorder.EventsHTTPRecorder{}
	task.EventsFlush(eventsRecorder, eventsStorageAdapter, size)
	c.String(http.StatusOK, "%s", "Events flushed")
}

// FlushImpressions eviction of Impressions
func FlushImpressions(c *gin.Context) {
	size, err := getIntegerParameterFromQuery(c, "size")
	if err != nil {
		c.String(http.StatusBadRequest, "%s", err.Error())
		return
	}
	if size != nil && *size < 1 {
		c.String(http.StatusBadRequest, "%s", "Size cannot be less than 1")
		return
	}
	impressionsStorageAdapter := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	impressionRecorder := recorder.ImpressionsHTTPRecorder{}
	task.ImpressionsFlush(impressionRecorder, impressionsStorageAdapter, size, conf.Data.Redis.DisableLegacyImpressions, true)
	c.String(http.StatusOK, "%s", "Impressions flushed")
}
