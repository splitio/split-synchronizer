package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/splitio/go-split-commons/v4/storage"
	"github.com/splitio/go-split-commons/v4/telemetry"

	"github.com/splitio/split-synchronizer/v4/appcontext"
	"github.com/splitio/split-synchronizer/v4/log"
	"github.com/splitio/split-synchronizer/v4/splitio/common"
	syncTelemetry "github.com/splitio/split-synchronizer/v4/splitio/common/telemetry"
	"github.com/splitio/split-synchronizer/v4/splitio/task"
	"github.com/splitio/split-synchronizer/v4/splitio/web/dashboard/HTMLtemplates"
)

// Metrics struct
type Metrics struct {
	LoggedErrors                 string   `json:"loggedErrors"`
	LoggedMessages               []string `json:"loggedMessages"`
	SdksTotalRequests            string   `json:"sdksTotalRequests"`
	BackendTotalRequests         string   `json:"backendTotalRequests"`
	SplitsNumber                 string   `json:"splitsNumber"`
	SegmentsNumber               string   `json:"segmentsNumber"`
	RequestOkFormatted           string   `json:"requestOkFormatted"`
	RequestErrorFormatted        string   `json:"requestErrorFormatted"`
	BackendRequestOkFormatted    string   `json:"backendRequestOkFormatted"`
	BackendRequestErrorFormatted string   `json:"backendRequestErrorFormatted"`
	SplitRows                    string   `json:"splitRows"`
	SegmentRows                  string   `json:"segmentRows"`
	LatenciesGroupDataBackend    string   `json:"latenciesGroupDataBackend"`
	BackendRequestOk             string   `json:"backendRequestOk"`
	BackendRequestError          string   `json:"backendRequestError"`
	LatenciesGroupData           string   `json:"latenciesGroupData"`
	RequestOk                    string   `json:"requestOk"`
	RequestError                 string   `json:"requestError"`
	ImpressionsQueueSize         string   `json:"impressionsQueueSize"`
	EventsQueueSize              string   `json:"eventsQueueSize"`
	EventsLambda                 string   `json:"eventsLambda"`
	ImpressionsLambda            string   `json:"impressionsLambda"`
}

func formatNumber(n int64) string {
	//Hundred
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}

	//Thousand
	if n < 1000000 {
		k := float64(n) / float64(1000)
		return fmt.Sprintf("%.2f k", k)
	}

	//Million
	if n < 1000000000 {
		m := float64(n) / float64(1000000)
		return fmt.Sprintf("%.2f M", m)
	}

	//Billion
	if n < 1000000000000 {
		g := float64(n) / float64(1000000000)
		return fmt.Sprintf("%.2f G", g)
	}

	//Trillion
	if n < 1000000000000000 {
		t := float64(n) / float64(1000000000000)
		return fmt.Sprintf("%.2f T", t)
	}

	//Quadrillion
	q := float64(n) / float64(1000000000000000)
	return fmt.Sprintf("%.2f P", q)
}

func toRGBAString(r int, g int, b int, a float32) string {
	if a < 1 {
		return fmt.Sprintf("rgba(%d, %d, %d, %.1f)", r, g, b, a)
	}

	return fmt.Sprintf("rgba(%d, %d, %d, %d)", r, g, b, int(a))
}

// ParseTemplate parses template
func ParseTemplate(name string, text string, data interface{}) string {
	buf := new(bytes.Buffer)
	tpl := template.Must(template.New(name).Parse(text))
	tpl.Execute(buf, data)
	return buf.String()
}

func parseSDKStats(localTelemetry storage.TelemetryRuntimeConsumer) string {
	var toReturn string

	asPeeker, ok := localTelemetry.(syncTelemetry.ProxyTelemetryPeeker)
	if !ok {
		return toReturn
	}

	splitLatencies := asPeeker.PeekEndpointLatency(syncTelemetry.SplitChangesEndpoint)
	serialized, _ := json.Marshal(splitLatencies)
	toReturn += ParseTemplate("sdk.SplitChanges", HTMLtemplates.LatencySerieTPL,
		HTMLtemplates.LatencySerieTPLVars{
			Label:           "/api/splitChanges",
			BackgroundColor: toRGBAString(255, 159, 64, 0.2),
			BorderColor:     toRGBAString(255, 159, 64, 1),
			Data:            string(serialized),
		})

	segmentsLatencies := asPeeker.PeekEndpointLatency(syncTelemetry.SegmentChangesEndpoint)
	serialized, _ = json.Marshal(segmentsLatencies)
	toReturn += ParseTemplate("sdk.segmentChanges", HTMLtemplates.LatencySerieTPL,
		HTMLtemplates.LatencySerieTPLVars{
			Label:           "/api/segmentChanges",
			BackgroundColor: toRGBAString(54, 162, 235, 0.2),
			BorderColor:     toRGBAString(54, 162, 235, 1),
			Data:            string(serialized),
		})

	impressionLatencies := asPeeker.PeekEndpointLatency(syncTelemetry.ImpressionsBulkEndpoint)
	serialized, _ = json.Marshal(impressionLatencies)
	toReturn += ParseTemplate("sdk.impressions", HTMLtemplates.LatencySerieTPL,
		HTMLtemplates.LatencySerieTPLVars{
			Label:           "/api/testImpressions/bulk",
			BackgroundColor: toRGBAString(75, 192, 192, 0.2),
			BorderColor:     toRGBAString(75, 192, 192, 1),
			Data:            string(serialized),
		})

	eventLatencies := asPeeker.PeekEndpointLatency(syncTelemetry.EventsBulkEndpoint)
	serialized, _ = json.Marshal(eventLatencies)
	toReturn += ParseTemplate("sdk.events", HTMLtemplates.LatencySerieTPL,
		HTMLtemplates.LatencySerieTPLVars{
			Label:           "/api/events/bulk",
			BackgroundColor: toRGBAString(255, 205, 86, 0.2),
			BorderColor:     toRGBAString(255, 205, 86, 1),
			Data:            string(serialized),
		})

	mySegmentsLatencies := asPeeker.PeekEndpointLatency(syncTelemetry.MySegmentsEndpoint)
	serialized, _ = json.Marshal(mySegmentsLatencies)
	toReturn += ParseTemplate("sdk.mySegments", HTMLtemplates.LatencySerieTPL,
		HTMLtemplates.LatencySerieTPLVars{
			Label:           "/api/events/bulk",
			BackgroundColor: toRGBAString(153, 102, 255, 0.2),
			BorderColor:     toRGBAString(153, 102, 255, 1),
			Data:            string(serialized),
		})

	// TODO(mredolatti): we should start tracking beacon endpoints as well

	return toReturn
}

func parseBackendStats(localTelemetry storage.TelemetryRuntimeConsumer) string {
	var toReturn string

	asPeeker, ok := localTelemetry.(storage.TelemetryPeeker)
	if !ok {
		return toReturn
	}

	splitLatencies := asPeeker.PeekHttpLatencies(telemetry.SplitSync)
	serialized, _ := json.Marshal(splitLatencies)
	toReturn += ParseTemplate("backend::/api/splitChanges", HTMLtemplates.LatencySerieTPL,
		HTMLtemplates.LatencySerieTPLVars{
			Label:           "/api/splitChanges",
			BackgroundColor: toRGBAString(255, 159, 64, 0.2),
			BorderColor:     toRGBAString(255, 159, 64, 1),
			Data:            string(serialized),
		})

	segmentsLatencies := asPeeker.PeekHttpLatencies(telemetry.SegmentSync)
	serialized, _ = json.Marshal(segmentsLatencies)
	toReturn += ParseTemplate("backend::/api/segmentChanges", HTMLtemplates.LatencySerieTPL,
		HTMLtemplates.LatencySerieTPLVars{
			Label:           "/api/segmentChanges",
			BackgroundColor: toRGBAString(54, 162, 235, 0.2),
			BorderColor:     toRGBAString(54, 162, 235, 1),
			Data:            string(serialized),
		})

	impressionLatencies := asPeeker.PeekHttpLatencies(telemetry.ImpressionSync)
	serialized, _ = json.Marshal(impressionLatencies)
	toReturn += ParseTemplate("backend::/api/testImpressions/bulk", HTMLtemplates.LatencySerieTPL,
		HTMLtemplates.LatencySerieTPLVars{
			Label:           "/api/testImpressions/bulk",
			BackgroundColor: toRGBAString(75, 192, 192, 0.2),
			BorderColor:     toRGBAString(75, 192, 192, 1),
			Data:            string(serialized),
		})

	eventLatencies := asPeeker.PeekHttpLatencies(telemetry.EventSync)
	serialized, _ = json.Marshal(eventLatencies)
	toReturn += ParseTemplate("backend::/api/events/bulk", HTMLtemplates.LatencySerieTPL,
		HTMLtemplates.LatencySerieTPLVars{
			Label:           "/api/events/bulk",
			BackgroundColor: toRGBAString(255, 205, 86, 0.2),
			BorderColor:     toRGBAString(255, 205, 86, 1),
			Data:            string(serialized),
		})

	return toReturn
}

func parseCachedSplits(splitStorage storage.SplitStorage) string {
	cachedSplits := splitStorage.All()

	return ParseTemplate(
		"CachedSplits",
		HTMLtemplates.CachedSplitsTPL,
		HTMLtemplates.NewCachedSplitsTPLVars(cachedSplits))
}

func parseCachedSegments(splitStorage storage.SplitStorage, segmentStorage storage.SegmentStorage) string {
	cachedSegments := splitStorage.SegmentNames()

	toRender := make([]*HTMLtemplates.CachedSegmentRowTPLVars, 0)
	for _, s := range cachedSegments.List() {

		segment, _ := s.(string)
		activeKeys := segmentStorage.Keys(segment)
		size := 0
		if activeKeys != nil {
			size = activeKeys.Size()
		}

		removedKeys := 0
		if appcontext.ExecutionMode() == appcontext.ProxyMode {
			removedKeys = int(segmentStorage.CountRemovedKeys(segment))
		}

		// LAST MODIFIED
		changeNumber, err := segmentStorage.ChangeNumber(segment)
		if err != nil {
			log.Instance.Warning(fmt.Sprintf("Error fetching last update for segment %s\n", segment))
		}
		lastModified := time.Unix(0, changeNumber*int64(time.Millisecond))

		toRender = append(toRender,
			&HTMLtemplates.CachedSegmentRowTPLVars{
				ProxyMode:    appcontext.ExecutionMode() == appcontext.ProxyMode,
				Name:         segment,
				ActiveKeys:   strconv.Itoa(size),
				LastModified: lastModified.UTC().Format(time.UnixDate),
				RemovedKeys:  strconv.Itoa(removedKeys),
				TotalKeys:    strconv.Itoa(removedKeys + size),
			})
	}

	return ParseTemplate(
		"CachedSegments",
		HTMLtemplates.CachedSegmentsTPL,
		HTMLtemplates.CachedSegmentsTPLVars{Segments: toRender})
}

func parseEventsSize(eventStorage storage.EventsStorage) string {
	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		return "0"
	}

	size := eventStorage.Count()
	eventsSize := strconv.FormatInt(size, 10)

	return eventsSize
}

func parseImpressionSize(impressionStorage storage.ImpressionStorage) string {
	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		return "0"
	}

	size := impressionStorage.Count()
	impressionsSize := strconv.FormatInt(size, 10)

	return impressionsSize
}

func parseEventsLambda() string {
	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		return "0"
	}
	lambda := task.GetEventsLambda()
	if lambda > 10 {
		lambda = 10
	}
	return strconv.FormatFloat(lambda, 'f', 2, 64)
}

func parseImpressionsLambda() string {
	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		return "0"
	}
	lambda := task.GetImpressionsLambda()
	if lambda > 10 {
		lambda = 10
	}
	return strconv.FormatFloat(lambda, 'f', 2, 64)
}

// GetMetrics data
func GetMetrics(storages common.Storages) Metrics {
	splitNames := storages.SplitStorage.SplitNames()
	segmentNames := storages.SplitStorage.SegmentNames()

	// Counters
	//counters := storages.LocalTelemetryStorage.PeekCounters()
	// TODO(mredolatti): Refactor this
	counters := make(map[string]int64)
	backendErrorCount := int64(0)
	for key, counter := range counters {
		if strings.Contains(key, "backend::") && key != "backend::request.ok" {
			backendErrorCount += counter
		}
	}

	return Metrics{
		SplitsNumber:                 strconv.Itoa(len(splitNames)),
		SegmentsNumber:               strconv.Itoa(segmentNames.Size()),
		LoggedErrors:                 formatNumber(log.ErrorDashboard.Counts()),
		LoggedMessages:               log.ErrorDashboard.Messages(),
		RequestErrorFormatted:        formatNumber(counters["sdk.request.error"]),
		RequestOkFormatted:           formatNumber(counters["sdk.request.ok"]),
		SdksTotalRequests:            formatNumber(counters["sdk.request.ok"] + counters["sdk.request.error"]),
		BackendTotalRequests:         formatNumber(counters["backend::request.ok"] + backendErrorCount),
		BackendRequestOkFormatted:    formatNumber(counters["backend::request.ok"]),
		BackendRequestErrorFormatted: formatNumber(backendErrorCount),
		SplitRows:                    parseCachedSplits(storages.SplitStorage),
		SegmentRows:                  parseCachedSegments(storages.SplitStorage, storages.SegmentStorage),
		LatenciesGroupDataBackend:    "[" + parseBackendStats(storages.LocalTelemetryStorage) + "]",
		BackendRequestOk:             strconv.Itoa(int(counters["backend::request.ok"])),
		BackendRequestError:          strconv.Itoa(int(backendErrorCount)),
		LatenciesGroupData:           "[" + parseSDKStats(storages.LocalTelemetryStorage) + "]",
		RequestOk:                    strconv.Itoa(int(counters["sdk.request.ok"])),
		RequestError:                 strconv.Itoa(int(counters["sdk.request.error"])),
		EventsQueueSize:              parseEventsSize(storages.EventStorage),
		ImpressionsQueueSize:         parseImpressionSize(storages.ImpressionStorage),
		EventsLambda:                 parseEventsLambda(),
		ImpressionsLambda:            parseImpressionsLambda(),
	}
}
