package dashboard

import (
	"strconv"
	"time"

	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/common"
	"github.com/splitio/split-synchronizer/splitio/stats"
	"github.com/splitio/split-synchronizer/splitio/task"
	"github.com/splitio/split-synchronizer/splitio/web"
	"github.com/splitio/split-synchronizer/splitio/web/dashboard/HTMLtemplates"
)

// Dashboard represents html dashboard class
type Dashboard struct {
	title       string
	proxy       bool
	storages    common.Storages
	httpClients common.HTTPClients
	layoutTpl   string
	mainMenuTpl string
}

// NewDashboard returns an instance of Dashboard struct
func NewDashboard(
	title string,
	isProxy bool,
	storages common.Storages,
	httpClients common.HTTPClients,
) *Dashboard {
	return &Dashboard{
		title:       title,
		proxy:       isProxy,
		storages:    storages,
		httpClients: httpClients,
	}
}

//HTML returns parsed HTML code
func (d *Dashboard) HTML() string {
	metrics := web.GetMetrics(d.storages)

	eventStatus := true
	sdkStatus := true
	storageStatus := true
	runningMode := "Running as Proxy Mode"
	if !d.proxy {
		runningMode = "Running as Synchronizer Mode"
		eventStatus, sdkStatus, storageStatus = task.CheckProducerStatus(d.storages.SplitStorage, d.httpClients.SdkClient, d.httpClients.EventsClient)
	} else {
		eventStatus, sdkStatus = task.CheckEventsSdkStatus(d.httpClients.SdkClient, d.httpClients.EventsClient)
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
			EventsLambda:                 metrics.EventsLambda,
			ImpressionsLambda:            metrics.ImpressionsLambda,
		},
	)

	return d.mainMenuTpl
}

// HTMLSegmentKeys return a html representation of segment's keys list
func (d *Dashboard) HTMLSegmentKeys(segmentName string) string {
	keys := d.storages.SegmentStorage.Keys(segmentName)
	segmentKeys := make([]HTMLtemplates.CachedSegmentKeysRowTPLVars, 0)

	if keys != nil {
		for _, key := range keys.List() {
			name, _ := key.(string)
			cn, _ := d.storages.SegmentStorage.ChangeNumber(name)
			lastModified := time.Unix(0, cn)
			removedColor := ""
			segmentKeys = append(segmentKeys, HTMLtemplates.CachedSegmentKeysRowTPLVars{
				Name:         name,
				LastModified: lastModified.UTC().Format(time.UnixDate),
				Removed:      strconv.FormatBool(false),
				RemovedColor: removedColor,
			})
		}
	}

	return web.ParseTemplate(
		"SegmentKeys",
		HTMLtemplates.CachedSegmentKeysTPL,
		HTMLtemplates.CachedSegmentKeysTPLVars{ProxyMode: d.proxy, SegmentKeys: segmentKeys},
	)
}
