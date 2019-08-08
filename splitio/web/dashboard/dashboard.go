package dashboard

import (
	"strconv"
	"time"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/stats"
	"github.com/splitio/split-synchronizer/splitio/storage"
	"github.com/splitio/split-synchronizer/splitio/task"
	"github.com/splitio/split-synchronizer/splitio/web"
	"github.com/splitio/split-synchronizer/splitio/web/dashboard/HTMLtemplates"
)

// Dashboard represents html dashboard class
type Dashboard struct {
	title          string
	proxy          bool
	splitStorage   storage.SplitStorage
	segmentStorage storage.SegmentStorage
	layoutTpl      string
	mainMenuTpl    string
}

// NewDashboard returns an instance of Dashboard struct
func NewDashboard(title string, isProxy bool, splitStorage storage.SplitStorage, segmentStorage storage.SegmentStorage) *Dashboard {
	return &Dashboard{title: title, proxy: isProxy, splitStorage: splitStorage, segmentStorage: segmentStorage}
}

//HTML returns parsed HTML code
func (d *Dashboard) HTML() string {
	metrics := web.GetMetrics(d.splitStorage, d.segmentStorage)

	eventStatus := true
	sdkStatus := true
	storageStatus := true
	runningMode := "Running as Proxy Mode"
	if !d.proxy {
		runningMode = "Running as Synchronizer Mode"
		eventStatus, sdkStatus, storageStatus = task.CheckProducerStatus(d.splitStorage)
	} else {
		eventStatus, sdkStatus = task.CheckEventsSdkStatus()
	}

	//Parsing main menu
	d.mainMenuTpl = web.ParseTemplate(
		"MainMenu",
		HTMLtemplates.MainMenuTPL,
		HTMLtemplates.MainMenuTPLVars{ProxyMode: d.proxy})

	//Rendering layout
	d.mainMenuTpl = web.ParseTemplate(
		"Layout",
		HTMLtemplates.LayoutTPL,
		HTMLtemplates.LayoutTPLVars{
			DashboardTitle:               d.title,
			RunningMode:                  runningMode,
			ProxyMode:                    d.proxy,
			MainMenu:                     d.mainMenuTpl,
			Uptime:                       stats.UptimeFormatted(),
			LoggedErrors:                 metrics.LoggedErrors,
			Version:                      splitio.Version,
			LoggedMessages:               metrics.LoggedMessages,
			SplitsNumber:                 metrics.SplitsNumber,
			SegmentsNumber:               metrics.SegmentsNumber,
			RequestError:                 metrics.RequestError,
			RequestErrorFormatted:        metrics.RequestErrorFormatted,
			RequestOk:                    metrics.RequestOk,
			RequestOkFormatted:           metrics.RequestOkFormatted,
			SdksTotalRequests:            metrics.SdksTotalRequests,
			BackendTotalRequests:         metrics.BackendTotalRequests,
			BackendRequestOkFormatted:    metrics.BackendRequestOkFormatted,
			BackendRequestErrorFormatted: metrics.BackendRequestErrorFormatted,
			BackendRequestOk:             metrics.BackendRequestOk,
			BackendRequestError:          metrics.BackendRequestError,
			LatenciesGroupData:           metrics.LatenciesGroupData,
			LatenciesGroupDataBackend:    metrics.LatenciesGroupDataBackend,
			SplitRows:                    metrics.SplitRows,
			SegmentRows:                  metrics.SegmentRows,
			ImpressionsQueueSize:         metrics.ImpressionsQueueSize,
			EventsQueueSize:              metrics.EventsQueueSize,
			EventServerStatus:            eventStatus,
			SDKServerStatus:              sdkStatus,
			StorageStatus:                storageStatus,
			Sync:                         true,
			HealthySince:                 task.GetHealthySinceTimestamp(),
			RefreshTime:                  15000,
			EventsDelta:                  metrics.EventsDelta,
			ImpressionsDelta:             metrics.ImpressionsDelta,
		},
	)

	return d.mainMenuTpl
}

// HTMLSegmentKeys return a html representation of segment's keys list
func (d *Dashboard) HTMLSegmentKeys(segmentName string) string {

	keys, err := d.segmentStorage.Keys(segmentName)
	if err != nil {
		log.Error.Println("Error fetching keys for segment:", segmentName)
		return ""
	}

	segmentKeys := make([]HTMLtemplates.CachedSegmentKeysRowTPLVars, 0)

	for _, key := range keys {
		lastModified := time.Unix(0, key.LastModified*int64(time.Millisecond))
		var removedColor string
		if key.Removed {
			removedColor = "danger"
		} else {
			removedColor = ""
		}
		segmentKeys = append(segmentKeys, HTMLtemplates.CachedSegmentKeysRowTPLVars{
			Name:         key.Name,
			LastModified: lastModified.UTC().Format(time.UnixDate),
			Removed:      strconv.FormatBool(key.Removed),
			RemovedColor: removedColor,
		})
	}

	return web.ParseTemplate(
		"SegmentKeys",
		HTMLtemplates.CachedSegmentKeysTPL,
		HTMLtemplates.CachedSegmentKeysTPLVars{ProxyMode: d.proxy, SegmentKeys: segmentKeys},
	)
}
