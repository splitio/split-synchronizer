package controllers

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-toolkit/v4/logging"
	"github.com/splitio/split-synchronizer/v4/appcontext"
	"github.com/splitio/split-synchronizer/v4/conf"
	"github.com/splitio/split-synchronizer/v4/log"
	"github.com/splitio/split-synchronizer/v4/splitio"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
	"github.com/splitio/split-synchronizer/v4/splitio/stats"
	"github.com/splitio/split-synchronizer/v4/splitio/task"
	"github.com/splitio/split-synchronizer/v4/splitio/util"
	"github.com/splitio/split-synchronizer/v4/splitio/web"
	"github.com/splitio/split-synchronizer/v4/splitio/web/dashboard"
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
	localTelemetryStorage := util.GetTelemetryStorage(c.Get(common.LocalMetricStorage))
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
		"apiKey":              logging.ObfuscateAPIKey(conf.Data.APIKey),
		"impressionListener":  conf.Data.ImpressionListener,
		"splitRefreshRate":    conf.Data.SplitsFetchRate,
		"segmentsRefreshRate": conf.Data.SegmentFetchRate,
		"impressionsPostRate": conf.Data.ImpressionsPostRate,
		"impressionsPerPost":  conf.Data.ImpressionsPerPost,
		"impressionsThreads":  conf.Data.ImpressionsThreads,
		"impressionsMode":     conf.Data.ImpressionsMode,
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

	httpClients := util.GetHTTPClients(c.Get(common.HTTPClientsGin))

	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		status["message"] = "Proxy service working as expected"
		eventsOK, sdkOK, authOK := task.CheckSplitServers(*httpClients)
		healthy["date"] = task.GetHealthySince()
		healthy["time"] = task.GetHealthySinceTimestamp()
		eventsStatus := parseStatus(eventsOK, "Events")
		sdkStatus := parseStatus(sdkOK, "SDK")
		authStatus := parseStatus(authOK, "Auth")

		response["proxy"] = status
		response["sdk"] = sdkStatus
		response["events"] = eventsStatus
		response["auth"] = authStatus
		response["healthySince"] = healthy

		if sdkStatus["healthy"].(bool) && eventsStatus["healthy"].(bool) && authStatus["healthy"].(bool) {
			c.JSON(http.StatusOK, response)
		} else {
			c.JSON(http.StatusInternalServerError, response)
		}
	} else {

		status["message"] = "Synchronizer service working as expected"
		eventsOK, sdkOK, authOK := task.CheckSplitServers(*httpClients)
		storageOk := false
		// Storage service
		splitStorage := util.GetSplitStorage(c.Get(common.SplitStorage))
		if splitStorage != nil {
			storageOk = task.GetStorageStatus(splitStorage)
		} else {
			log.Instance.Warning("Storage Status could not be fetched")
		}
		healthy["date"] = task.GetHealthySince()
		healthy["time"] = task.GetHealthySinceTimestamp()
		eventsStatus := parseStatus(eventsOK, "Events")
		sdkStatus := parseStatus(sdkOK, "SDK")
		storageStatus := parseStatus(storageOk, "Storage")
		authStatus := parseStatus(authOK, "Auth")

		response["sync"] = status
		response["storage"] = storageStatus
		response["sdk"] = sdkStatus
		response["events"] = eventsStatus
		response["auth"] = authStatus
		response["healthySince"] = healthy

		if storageStatus["healthy"].(bool) && sdkStatus["healthy"].(bool) && eventsStatus["healthy"].(bool) && authStatus["healthy"].(bool) {
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
		SplitStorage:          util.GetSplitStorage(c.Get(common.SplitStorage)),
		SegmentStorage:        util.GetSegmentStorage(c.Get(common.SegmentStorage)),
		EventStorage:          util.GetEventStorage(c.Get(common.EventStorage)),
		ImpressionStorage:     util.GetImpressionStorage(c.Get(common.ImpressionStorage)),
		LocalTelemetryStorage: util.GetTelemetryStorage(c.Get(common.LocalMetricStorage)),
	}
	// HttpClients
	httpClients := util.GetHTTPClients(c.Get(common.HTTPClientsGin))

	if util.AreValidStorages(storages) && util.AreValidAPIClient(httpClients) {
		dash := createDashboard(storages, *httpClients)
		var toReturn = dash.HTMLSegmentKeys(segmentName)
		c.String(http.StatusOK, "%s", toReturn)
		return
	}
	log.Instance.Error("DashboardSegmentKeys: Could not fetch storages")
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
		SplitStorage:          util.GetSplitStorage(c.Get(common.SplitStorage)),
		SegmentStorage:        util.GetSegmentStorage(c.Get(common.SegmentStorage)),
		EventStorage:          util.GetEventStorage(c.Get(common.EventStorage)),
		ImpressionStorage:     util.GetImpressionStorage(c.Get(common.ImpressionStorage)),
		LocalTelemetryStorage: util.GetTelemetryStorage(c.Get(common.LocalMetricStorage)),
	}
	// HttpClients
	httpClients := util.GetHTTPClients(c.Get(common.HTTPClientsGin))

	if util.AreValidStorages(storages) && util.AreValidAPIClient(httpClients) {
		dash := createDashboard(storages, *httpClients)
		//Write your 200 header status (or other status codes, but only WriteHeader once)
		c.Writer.WriteHeader(http.StatusOK)
		//Convert your cached html string to byte array
		c.Writer.Write([]byte(dash.HTML()))
		return
	}
	log.Instance.Error("Dashboard: Could not fetch storages")
	c.String(http.StatusInternalServerError, "%s", "Could not fetch storage")
}

// GetEventsQueueSize returns events queue size
func GetEventsQueueSize(c *gin.Context) {
	eventStorage := util.GetEventStorage(c.Get(common.EventStorage))
	queueSize := eventStorage.Count()
	c.JSON(http.StatusOK, gin.H{"queueSize": queueSize})
}

// GetImpressionsQueueSize returns impressions queue size
func GetImpressionsQueueSize(c *gin.Context) {
	impressionStorage := util.GetImpressionStorage(c.Get(common.ImpressionStorage))
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

	eventStorage := util.GetEventStorage(c.Get(common.EventStorage))
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

	impressionStorage := util.GetImpressionStorage(c.Get(common.ImpressionStorage))
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
	recorders := util.GetRecorders(c.Get(common.RecordersGin))
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
	recorders := util.GetRecorders(c.Get(common.RecordersGin))
	if recorders == nil {
		c.String(http.StatusInternalServerError, "%s", err.Error())
		return
	}
	var toFlush int64 = 0
	if size != nil {
		toFlush = *size
	}
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
		SplitStorage:          util.GetSplitStorage(c.Get(common.SplitStorage)),
		EventStorage:          util.GetEventStorage(c.Get(common.EventStorage)),
		ImpressionStorage:     util.GetImpressionStorage(c.Get(common.ImpressionStorage)),
		LocalTelemetryStorage: util.GetTelemetryStorage(c.Get(common.LocalMetricStorage)),
		SegmentStorage:        util.GetSegmentStorage(c.Get(common.SegmentStorage)),
	}

	if util.AreValidStorages(storages) {
		stats := web.GetMetrics(storages)
		c.JSON(http.StatusOK, stats)
		return
	}
	log.Instance.Error("GetMetrics: Could not fetch storages")
	c.String(http.StatusInternalServerError, "%s", "Could not fetch storages")
}
