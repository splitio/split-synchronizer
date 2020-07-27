package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/appcontext"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/common"
	"github.com/splitio/split-synchronizer/splitio/stats"
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
	localTelemetryStorage := getTelemetryStorage(c.Get("LocalMetricStorage"))
	counters := localTelemetryStorage.PeekCounters()
	latencies := localTelemetryStorage.PeekLatencies()
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

	sdkClient := getSdkClient(c.Get("SdkClient"))
	eventsClient := getSdkClient(c.Get("EventsClient"))

	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		status["message"] = "Proxy service working as expected"
		eventsOK, sdkOK := task.CheckEventsSdkStatus(sdkClient, eventsClient)
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
		eventsOK, sdkOK := task.CheckEventsSdkStatus(sdkClient, eventsClient)
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

// DashboardSegmentKeys returns a keys for a given segment
func DashboardSegmentKeys(c *gin.Context) {
	segmentName := c.Param("segment")

	// Storage service
	storages := common.Storages{
		SplitStorage:          getSplitStorage(c.Get("SplitStorage")),
		SegmentStorage:        getSegmentStorage(c.Get("SegmentStorage")),
		EventStorage:          getEventStorage(c.Get("EventStorage")),
		ImpressionStorage:     getImpressionStorage(c.Get("ImpressionStorage")),
		LocalTelemetryStorage: getTelemetryStorage(c.Get("LocalMetricStorage")),
		TelemetryStorage:      getTelemetryStorage(c.Get("TelemetryStorage")),
	}
	// HttpClients
	httpClients := common.HTTPClients{
		SdkClient:    getSdkClient(c.Get("SdkClient")),
		EventsClient: getSdkClient(c.Get("EventsClient")),
	}

	if areValidStorages(storages) {
		dash := createDashboard(storages, httpClients)
		var toReturn = dash.HTMLSegmentKeys(segmentName)
		c.String(http.StatusOK, "%s", toReturn)
		return
	}
	log.Error.Println("DashboardSegmentKeys: Could not fetch storages")
	c.String(http.StatusInternalServerError, "%s", "Could not fetch storage")
}

func createDashboard(storages common.Storages, httpClients common.HTTPClients) *dashboard.Dashboard {
	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		return dashboard.NewDashboard(conf.Data.Proxy.Title, true, storages, httpClients)
	}
	return dashboard.NewDashboard(conf.Data.Producer.Admin.Title, false, storages, httpClients)
}

// Dashboard returns a dashboard
func Dashboard(c *gin.Context) {
	// Storage service
	storages := common.Storages{
		SplitStorage:          getSplitStorage(c.Get("SplitStorage")),
		SegmentStorage:        getSegmentStorage(c.Get("SegmentStorage")),
		EventStorage:          getEventStorage(c.Get("EventStorage")),
		ImpressionStorage:     getImpressionStorage(c.Get("ImpressionStorage")),
		LocalTelemetryStorage: getTelemetryStorage(c.Get("LocalMetricStorage")),
		TelemetryStorage:      getTelemetryStorage(c.Get("TelemetryStorage")),
	}
	// HttpClients
	httpClients := common.HTTPClients{
		SdkClient:    getSdkClient(c.Get("SdkClient")),
		EventsClient: getSdkClient(c.Get("EventsClient")),
	}

	if areValidStorages(storages) {
		dash := createDashboard(storages, httpClients)
		//Write your 200 header status (or other status codes, but only WriteHeader once)
		c.Writer.WriteHeader(http.StatusOK)
		//Convert your cached html string to byte array
		c.Writer.Write([]byte(dash.HTML()))
		return
	}
	log.Error.Println("Dashboard: Could not fetch storages")
	c.String(http.StatusInternalServerError, "%s", "Could not fetch storage")
}

// GetEventsQueueSize returns events queue size
func GetEventsQueueSize(c *gin.Context) {
	eventStorage := getEventStorage(c.Get("EventStorage"))
	queueSize := eventStorage.Count()
	c.JSON(http.StatusOK, gin.H{"queueSize": queueSize})
}

// GetImpressionsQueueSize returns impressions queue size
func GetImpressionsQueueSize(c *gin.Context) {
	impressionStorage := getImpressionStorage(c.Get("ImpressionStorage"))
	queueSize := impressionStorage.Count()
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

	eventStorage := getEventStorage(c.Get("EventStorage"))
	size, err := getIntegerParameterFromQuery(c, "size")
	if err != nil {
		c.String(http.StatusBadRequest, "%s", err.Error())
		return
	}
	err = eventStorage.Drop(size)
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

	impressionStorage := getImpressionStorage(c.Get("ImpressionStorage"))
	size, err := getIntegerParameterFromQuery(c, "size")
	if err != nil {
		c.String(http.StatusBadRequest, "%s", err.Error())
		return
	}
	err = impressionStorage.Drop(size)
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
	if size != nil && *size > splitio.MaxSizeToFlush {
		c.String(http.StatusBadRequest, "%s", "Max Size to Flush is "+strconv.FormatInt(splitio.MaxSizeToFlush, 10))
		return
	}
	recorders := getRecorders(c.Get("Recorders"))
	if recorders == nil {
		c.String(http.StatusInternalServerError, "%s", err.Error())
		return
	}
	var toFlush int64 = 0
	if size != nil {
		toFlush = *size
	}
	err = recorders.Event.FlushEvents(toFlush)
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
	if size != nil && *size > splitio.MaxSizeToFlush {
		c.String(http.StatusBadRequest, "%s", "Max Size to Flush is "+strconv.FormatInt(splitio.MaxSizeToFlush, 10))
		return
	}
	recorders := getRecorders(c.Get("Recorders"))
	if recorders == nil {
		c.String(http.StatusInternalServerError, "%s", err.Error())
		return
	}
	var toFlush int64 = 0
	if size != nil {
		toFlush = *size
	}
	fmt.Println("SIZE", toFlush, "MAX", splitio.MaxSizeToFlush)
	err = recorders.Impression.FlushImpressions(toFlush)
	if err != nil {
		c.String(http.StatusInternalServerError, "%s", err.Error())
		return
	}
	c.String(http.StatusOK, "%s", "Impressions flushed")
}

// GetMetrics returns stats for dashboard
func GetMetrics(c *gin.Context) {
	storages := common.Storages{
		SplitStorage:          getSplitStorage(c.Get("SplitStorage")),
		EventStorage:          getEventStorage(c.Get("EventStorage")),
		ImpressionStorage:     getImpressionStorage(c.Get("ImpressionStorage")),
		LocalTelemetryStorage: getTelemetryStorage(c.Get("LocalMetricStorage")),
		SegmentStorage:        getSegmentStorage(c.Get("SegmentStorage")),
		TelemetryStorage:      getTelemetryStorage(c.Get("TelemetryStorage")),
	}

	if areValidStorages(storages) {
		stats := web.GetMetrics(storages)
		c.JSON(http.StatusOK, stats)
		return
	}
	log.Error.Println("GetMetrics: Could not fetch storages")
	c.String(http.StatusInternalServerError, "%s", "Could not fetch storages")
}
