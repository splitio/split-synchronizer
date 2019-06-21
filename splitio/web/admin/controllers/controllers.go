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
	"github.com/splitio/split-synchronizer/splitio/web/dashboard"
)

// Uptime returns the service uptime
func Uptime(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"uptime": stats.UptimeFormated()})
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
		"apiKey":                 log.ObfuscateAPIKey(conf.Data.APIKey),
		"impressionListener":     conf.Data.ImpressionListener,
		"splitRefreshRate":       conf.Data.SplitsFetchRate,
		"segmentsRefreshRate":    conf.Data.SegmentFetchRate,
		"impressionsRefreshRate": conf.Data.ImpressionsPostRate,
		"impressionsPerPost":     conf.Data.ImpressionsPerPost,
		"impressionsThreads":     conf.Data.ImpressionsThreads,
		"eventsPushRate":         conf.Data.EventsPushRate,
		"eventsConsumerReadSize": conf.Data.EventsConsumerReadSize,
		"eventsConsumerThreads":  conf.Data.EventsConsumerThreads,
		"metricsRefreshRate":     conf.Data.MetricsPostRate,
		"httpTimeout":            conf.Data.HTTPTimeout,
		"mode":                   config["mode"],
		"redisMode":              config["redisMode"],
		"log":                    conf.Data.Logger,
		"redis":                  config["redis"],
		"proxy":                  config["proxy"],
		"admin":                  conf.Data.Producer.Admin,
	})
}

// getSdkStatus checks the status of the SDK Server
func getSdkStatus() map[string]interface{} {
	_, err := api.SdkClient.Get("/version")
	sdkStatus := make(map[string]interface{})
	if err != nil {
		sdkStatus["healthy"] = false
		sdkStatus["message"] = "Cannot reach SDK service"
		log.Debug.Println("Events Server:", err)
	} else {
		sdkStatus["healthy"] = true
		sdkStatus["message"] = "SDK service working as expected"
	}
	return sdkStatus
}

// getEventsStatus checks the status of the Events Server
func getEventsStatus() map[string]interface{} {
	_, err := api.EventsClient.Get("/version")
	eventsStatus := make(map[string]interface{})
	if err != nil {
		eventsStatus["healthy"] = false
		eventsStatus["message"] = "Cannot reach Events service"
		log.Debug.Println("Events Server:", err)
	} else {
		eventsStatus["healthy"] = true
		eventsStatus["message"] = "Events service working as expected"
	}
	return eventsStatus
}

// HealthCheck returns the service status
func HealthCheck(c *gin.Context) {
	status := make(map[string]interface{})
	sdkStatus := getSdkStatus()
	eventsStatus := getEventsStatus()

	// Producer service
	status["healthy"] = true

	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		status["message"] = "Proxy service working as expected"

		if sdkStatus["healthy"].(bool) && eventsStatus["healthy"].(bool) {
			c.JSON(http.StatusOK, gin.H{"proxy": status, "sdk": sdkStatus, "events": eventsStatus})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"proxy": status, "sdk": sdkStatus, "events": eventsStatus})
		}
	} else {
		status["message"] = "Synchronizer service working as expected"

		// Storage service
		storageStatus := make(map[string]interface{})
		splitStorage, exists := c.Get("SplitStorage")
		if exists {
			st, ok := splitStorage.(storage.SplitStorage)
			if ok {
				_, err := st.ChangeNumber()
				if err != nil {
					storageStatus["healthy"] = false
					storageStatus["message"] = err.Error()
				} else {
					storageStatus["healthy"] = true
					storageStatus["message"] = "Storage service working as expected"
				}
			} else {
				storageStatus["healthy"] = false
				storageStatus["message"] = "Could not access to SplitStorage"
			}

		}

		if storageStatus["healthy"].(bool) && sdkStatus["healthy"].(bool) && eventsStatus["healthy"].(bool) {
			c.JSON(http.StatusOK, gin.H{"sync": status, "storage": storageStatus, "sdk": sdkStatus, "events": eventsStatus})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"sync": status, "storage": storageStatus, "sdk": sdkStatus, "events": eventsStatus})
		}
	}

}

// DashboardSegmentKeys returns a keys for a given segment
func DashboardSegmentKeys(c *gin.Context) {

	segmentName := c.Param("segment")

	// Storage service
	splitStorage, _ := c.Get("SplitStorage")
	segmentStorage, _ := c.Get("SegmentStorage")

	dash := createDashboard(splitStorage, segmentStorage)

	var toReturn = dash.HTMLSegmentKeys(segmentName)

	c.String(http.StatusOK, "%s", toReturn)
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
	splitStorage, _ := c.Get("SplitStorage")
	segmentStorage, _ := c.Get("SegmentStorage")

	dash := createDashboard(splitStorage, segmentStorage)

	//Write your 200 header status (or other status codes, but only WriteHeader once)
	c.Writer.WriteHeader(http.StatusOK)
	//Convert your cached html string to byte array
	c.Writer.Write([]byte(dash.HTML()))
	return
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
