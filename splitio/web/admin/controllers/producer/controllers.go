package producer

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/splitio/storage"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
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

	if storageStatus["healthy"].(bool) {
		c.JSON(http.StatusOK, gin.H{"sync": producerStatus, "storage": storageStatus})
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"sync": producerStatus, "storage": storageStatus})
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

// GetEventsQueueSize returns events queue size
func GetEventsQueueSize(c *gin.Context) {
	eventsStorageAdapter := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	queueSize := eventsStorageAdapter.Size(eventsStorageAdapter.GetQueueNamespace())
	c.JSON(http.StatusOK, gin.H{"queueSize": queueSize})
}

// GetImpressionsQueueSize returns impressions queue size
func GetImpressionsQueueSize(c *gin.Context) {
	impressionsStorageAdapter := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	queueSize := impressionsStorageAdapter.Size(impressionsStorageAdapter.GetQueueNamespace())
	c.JSON(http.StatusOK, gin.H{"queueSize": queueSize})
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
	err = eventsStorageAdapter.Drop(eventsStorageAdapter.GetQueueNamespace(), bulkSize)
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
	err = impressionsStorageAdapter.Drop(impressionsStorageAdapter.GetQueueNamespace(), bulkSize)
	if err == nil {
		c.String(http.StatusOK, "%s", "Impressions dropped")
		return
	}
	c.String(http.StatusInternalServerError, "%s", err.Error())

}
