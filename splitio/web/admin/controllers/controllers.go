package controllers

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/appcontext"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/recorder"
	"github.com/splitio/split-synchronizer/splitio/stats"
	"github.com/splitio/split-synchronizer/splitio/storage"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
	"github.com/splitio/split-synchronizer/splitio/task"
	"github.com/splitio/split-synchronizer/splitio/web"
	"github.com/splitio/split-synchronizer/splitio/web/dashboard"
)

// Uptime returns the service uptime
func Uptime(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"uptime": stats.UptimeFormatted()})
}

// Version returns the service version
func Version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"version": splitio.Version})
}

// Ping returns a 200 HTTP status code
func Ping(c *gin.Context) {
	c.String(http.StatusOK, "%s", "pong")
}

// ShowStats returns stats
func ShowStats(c *gin.Context) {
	counters := stats.Counters()
	latencies := stats.Latencies()
	c.JSON(http.StatusOK, gin.H{"counters": counters, "latencies": latencies})
}

// kill process helper
func kill(sig syscall.Signal) error {
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}
	return p.Signal(sig)
}

// StopProccess triggers a kill signal
func StopProccess(c *gin.Context) {
	stopType := c.Param("stopType")
	var toReturn string

	switch stopType {
	case "force":
		toReturn = stopType
		log.PostShutdownMessageToSlack(true)
		defer kill(syscall.SIGKILL)
	case "graceful":
		toReturn = stopType
		defer kill(syscall.SIGINT)
	default:
		c.String(http.StatusBadRequest, "Invalid sign type: %s", toReturn)
		return
	}

	c.String(http.StatusOK, "%s: %s", "Sign has been sent", toReturn)

}

// GetConfiguration Returns Sync Config
func GetConfiguration(c *gin.Context) {
	config := map[string]interface{}{
		"mode":      nil,
		"redisMode": nil,
		"redis":     nil,
		"proxy":     nil,
	}
	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		config["mode"] = "ProxyMode"
		config["proxy"] = conf.Data.Proxy
	} else {
		config["mode"] = "ProducerMode"
		if conf.Data.Redis.ClusterMode {
			config["redisMode"] = "Cluster"
		} else {
			if conf.Data.Redis.SentinelReplication {
				config["redisMode"] = "Sentinel"
			} else {
				config["redisMode"] = "Standard"
			}
		}
		config["redis"] = conf.Data.Redis
	}
	c.JSON(http.StatusOK, gin.H{
		"apiKey":              log.ObfuscateAPIKey(conf.Data.APIKey),
		"impressionListener":  conf.Data.ImpressionListener,
		"splitRefreshRate":    conf.Data.SplitsFetchRate,
		"segmentsRefreshRate": conf.Data.SegmentFetchRate,
		"impressionsPostRate": conf.Data.ImpressionsPostRate,
		"impressionsPerPost":  conf.Data.ImpressionsPerPost,
		"impressionsThreads":  conf.Data.ImpressionsThreads,
		"eventsPostRate":      conf.Data.EventsPostRate,
		"eventsPerPost":       conf.Data.EventsPerPost,
		"eventsThreads":       conf.Data.EventsThreads,
		"metricsPostRate":     conf.Data.MetricsPostRate,
		"httpTimeout":         conf.Data.HTTPTimeout,
		"mode":                config["mode"],
		"redisMode":           config["redisMode"],
		"log":                 conf.Data.Logger,
		"redis":               config["redis"],
		"proxy":               config["proxy"],
		"admin":               conf.Data.Producer.Admin,
	})
}

func parseStatus(ok bool, value string) map[string]interface{} {
	status := make(map[string]interface{})
	if ok {
		status["healthy"] = true
		status["message"] = value + " service working as expected"
		return status
	}
	status["healthy"] = false
	status["message"] = "Cannot reach " + value
	return status
}

// HealthCheck returns the service status
func HealthCheck(c *gin.Context) {
	response := make(map[string]interface{})
	status := make(map[string]interface{})
	status["healthy"] = true
	healthy := make(map[string]interface{})

	uptime := stats.UptimeFormatted()
	response["uptime"] = uptime

	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		status["message"] = "Proxy service working as expected"
		eventsOK, sdkOK := task.CheckEventsSdkStatus()
		healthy["date"] = task.GetHealthySince()
		healthy["time"] = task.GetHealthySinceTimestamp()
		eventsStatus := parseStatus(eventsOK, "Events")
		sdkStatus := parseStatus(sdkOK, "SDK")

		response["proxy"] = status
		response["sdk"] = sdkStatus
		response["events"] = eventsStatus
		response["healthySince"] = healthy

		if sdkStatus["healthy"].(bool) && eventsStatus["healthy"].(bool) {
			c.JSON(http.StatusOK, response)
		} else {
			c.JSON(http.StatusInternalServerError, response)
		}
	} else {

		status["message"] = "Synchronizer service working as expected"
		eventsOK, sdkOK := task.CheckEventsSdkStatus()
		storageOk := false
		// Storage service
		splitStorage := getSplitStorage(c.Get("SplitStorage"))
		if splitStorage != nil {
			storageOk = task.GetStorageStatus(splitStorage)
		} else {
			log.Warning.Println("Storage Status could not be fetched")
		}
		healthy["date"] = task.GetHealthySince()
		healthy["time"] = task.GetHealthySinceTimestamp()
		eventsStatus := parseStatus(eventsOK, "Events")
		sdkStatus := parseStatus(sdkOK, "SDK")
		storageStatus := parseStatus(storageOk, "Storage")

		response["sync"] = status
		response["storage"] = storageStatus
		response["sdk"] = sdkStatus
		response["events"] = eventsStatus
		response["healthySince"] = healthy

		if storageStatus["healthy"].(bool) && sdkStatus["healthy"].(bool) && eventsStatus["healthy"].(bool) {
			c.JSON(http.StatusOK, response)
		} else {
			c.JSON(http.StatusInternalServerError, response)
		}
	}
}

func getSplitStorage(splitStorage interface{}, exists bool) storage.SplitStorage {
	if !exists {
		return nil
	}
	if splitStorage == nil {
		log.Warning.Println("SplitStorage could not be fetched")
		return nil
	}
	st, ok := splitStorage.(storage.SplitStorage)
	if !ok {
		log.Warning.Println("SplitStorage could not be fetched")
		return nil
	}
	return st
}

func getSegmentStorage(segmentStorage interface{}, exists bool) storage.SegmentStorage {
	if !exists {
		return nil
	}
	if segmentStorage == nil {
		log.Warning.Println("SegmentStorage could not be fetched")
		return nil
	}
	st, ok := segmentStorage.(storage.SegmentStorage)
	if !ok {
		log.Warning.Println("SegmentStorage could not be fetched")
		return nil
	}
	return st
}

// DashboardSegmentKeys returns a keys for a given segment
func DashboardSegmentKeys(c *gin.Context) {

	segmentName := c.Param("segment")

	// Storage service
	splitStorage := getSplitStorage(c.Get("SplitStorage"))
	segmentStorage := getSegmentStorage(c.Get("SegmentStorage"))

	if splitStorage != nil && segmentStorage != nil {
		dash := createDashboard(splitStorage, segmentStorage)
		var toReturn = dash.HTMLSegmentKeys(segmentName)
		c.String(http.StatusOK, "%s", toReturn)
		return
	}
	c.String(http.StatusInternalServerError, "%s", nil)
}

func createDashboard(splitStorage interface{}, segmentStorage interface{}) *dashboard.Dashboard {
	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		return dashboard.NewDashboard(conf.Data.Proxy.Title, true,
			splitStorage.(storage.SplitStorage),
			segmentStorage.(storage.SegmentStorage),
		)
	}
	return dashboard.NewDashboard(conf.Data.Producer.Admin.Title, false,
		splitStorage.(storage.SplitStorage),
		segmentStorage.(storage.SegmentStorage),
	)
}

// Dashboard returns a dashboard
func Dashboard(c *gin.Context) {
	// Storage service
	splitStorage := getSplitStorage(c.Get("SplitStorage"))
	segmentStorage := getSegmentStorage(c.Get("SegmentStorage"))

	if splitStorage != nil && segmentStorage != nil {
		dash := createDashboard(splitStorage, segmentStorage)
		//Write your 200 header status (or other status codes, but only WriteHeader once)
		c.Writer.WriteHeader(http.StatusOK)
		//Convert your cached html string to byte array
		c.Writer.Write([]byte(dash.HTML()))
		return
	}
	c.String(http.StatusInternalServerError, "%s", nil)
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
		if field < 1 {
			return nil, errors.New("Size cannot be less than 1")
		}
		return &field, nil
	}
	return nil, nil
}

// DropEvents drops Events from queue
func DropEvents(c *gin.Context) {
	if task.RequestOperation(task.EventsOperation) {
		defer task.FinishOperation(task.EventsOperation)
	} else {
		c.String(http.StatusInternalServerError, "%s", "Cannot execute drop. Another operation is performing operations on Events")
		return
	}

	eventsStorageAdapter := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	size, err := getIntegerParameterFromQuery(c, "size")
	if err != nil {
		c.String(http.StatusBadRequest, "%s", err.Error())
		return
	}
	err = eventsStorageAdapter.Drop(size)
	if err == nil {
		c.String(http.StatusOK, "%s", "Events dropped")
		return
	}
	c.String(http.StatusInternalServerError, "%s", err.Error())
}

// DropImpressions drops Impressions from queue
func DropImpressions(c *gin.Context) {
	if task.RequestOperation(task.ImpressionsOperation) {
		defer task.FinishOperation(task.ImpressionsOperation)
	} else {
		c.String(http.StatusInternalServerError, "%s", "Cannot execute drop. Another operation is performing operations on Impressions")
		return
	}

	impressionsStorageAdapter := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	size, err := getIntegerParameterFromQuery(c, "size")
	if err != nil {
		c.String(http.StatusBadRequest, "%s", err.Error())
		return
	}
	err = impressionsStorageAdapter.Drop(size)
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
	if size != nil && *size > api.MaxSizeToFlush {
		c.String(http.StatusBadRequest, "%s", "Max Size to Flush is "+strconv.FormatInt(api.MaxSizeToFlush, 10))
		return
	}
	eventsStorageAdapter := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	eventsRecorder := recorder.EventsHTTPRecorder{}
	err = task.EventsFlush(eventsRecorder, eventsStorageAdapter, size)
	if err != nil {
		c.String(http.StatusInternalServerError, "%s", err.Error())
		return
	}
	c.String(http.StatusOK, "%s", "Events flushed")
}

// FlushImpressions eviction of Impressions
func FlushImpressions(c *gin.Context) {
	size, err := getIntegerParameterFromQuery(c, "size")
	if err != nil {
		c.String(http.StatusBadRequest, "%s", err.Error())
		return
	}
	if size != nil && *size > api.MaxSizeToFlush {
		c.String(http.StatusBadRequest, "%s", "Max Size to Flush is "+strconv.FormatInt(api.MaxSizeToFlush, 10))
		return
	}
	impressionsStorageAdapter := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	impressionRecorder := recorder.ImpressionsHTTPRecorder{}
	err = task.ImpressionsFlush(impressionRecorder, impressionsStorageAdapter, size, conf.Data.Redis.DisableLegacyImpressions, true)
	if err != nil {
		c.String(http.StatusInternalServerError, "%s", err.Error())
		return
	}
	c.String(http.StatusOK, "%s", "Impressions flushed")
}

// GetMetrics returns stats for dashboard
func GetMetrics(c *gin.Context) {
	// Storage service
	splitStorage, _ := c.Get("SplitStorage")
	segmentStorage, _ := c.Get("SegmentStorage")

	stats := web.GetMetrics(splitStorage.(storage.SplitStorage), segmentStorage.(storage.SegmentStorage))

	c.JSON(http.StatusOK, stats)
}
