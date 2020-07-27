package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/splitio/go-split-commons/storage"
	"github.com/splitio/split-synchronizer/appcontext"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/common"
	"github.com/splitio/split-synchronizer/splitio/task"
	"github.com/splitio/split-synchronizer/splitio/web/dashboard/HTMLtemplates"
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

func parseLatencySerieData(key string, label string, backgroundColor string, borderColor string, localTelemetry storage.MetricsStorage) string {
	var toReturn string

	latencies := localTelemetry.PeekLatencies()
	if ldata, ok := latencies[key]; ok {
		if serie, err := json.Marshal(ldata); err == nil {
			toReturn = ParseTemplate(
				key,
				HTMLtemplates.LatencySerieTPL,
				HTMLtemplates.LatencySerieTPLVars{
					Label:           label,
					BackgroundColor: backgroundColor,
					BorderColor:     borderColor,
					Data:            string(serie),
				})
		}
	}

	return toReturn
}

func parseSDKStats(localTelemetry storage.MetricsStorage) string {
	var toReturn string

	toReturn += parseLatencySerieData(
		"/api/splitChanges",
		"/api/splitChanges",
		toRGBAString(255, 159, 64, 0.2),
		toRGBAString(255, 159, 64, 1),
		localTelemetry,
	)

	toReturn += parseLatencySerieData(
		"/api/segmentChanges/*",
		"/api/segmentChanges/*",
		toRGBAString(54, 162, 235, 0.2),
		toRGBAString(54, 162, 235, 1),
		localTelemetry,
	)

	toReturn += parseLatencySerieData(
		"/api/testImpressions/bulk",
		"/api/testImpressions/bulk",
		toRGBAString(75, 192, 192, 0.2),
		toRGBAString(75, 192, 192, 1),
		localTelemetry,
	)

	toReturn += parseLatencySerieData(
		"/api/events/bulk",
		"/api/events/bulk",
		toRGBAString(255, 205, 86, 0.2),
		toRGBAString(255, 205, 86, 1),
		localTelemetry,
	)

	toReturn += parseLatencySerieData(
		"/api/mySegments/*",
		"/api/mySegments/*",
		toRGBAString(153, 102, 255, 0.2),
		toRGBAString(153, 102, 255, 1),
		localTelemetry,
	)

	return toReturn
}

func parseBackendStats(localTelemetry storage.MetricsStorage) string {
	var toReturn string

	toReturn += parseLatencySerieData(
		"backend::/api/splitChanges",
		"/api/splitChanges",
		toRGBAString(255, 159, 64, 0.2),
		toRGBAString(255, 159, 64, 1),
		localTelemetry,
	)

	toReturn += parseLatencySerieData(
		"backend::/api/segmentChanges",
		"/api/segmentChanges/*",
		toRGBAString(54, 162, 235, 0.2),
		toRGBAString(54, 162, 235, 1),
		localTelemetry,
	)

	toReturn += parseLatencySerieData(
		"backend::/api/testImpressions/bulk",
		"/api/testImpressions/bulk",
		toRGBAString(75, 192, 192, 0.2),
		toRGBAString(75, 192, 192, 1),
		localTelemetry,
	)

	toReturn += parseLatencySerieData(
		"backend::/api/events/bulk",
		"/api/events/bulk",
		toRGBAString(255, 205, 86, 0.2),
		toRGBAString(255, 205, 86, 1),
		localTelemetry,
	)

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

		/*
			removedKeys, err := segmentStorage.CountRemovedKeys(segment)
			if err != nil {
				log.Warning.Printf("Error counting removed keys for segment %s\n", segment)
			}
		*/

		// LAST MODIFIED
		changeNumber, err := segmentStorage.ChangeNumber(segment)
		if err != nil {
			log.Warning.Printf("Error fetching last update for segment %s\n", segment)
		}
		lastModified := time.Unix(0, changeNumber*int64(time.Millisecond))

		toRender = append(toRender,
			&HTMLtemplates.CachedSegmentRowTPLVars{
				ProxyMode:    appcontext.ExecutionMode() == appcontext.ProxyMode,
				Name:         segment,
				ActiveKeys:   strconv.Itoa(size),
				LastModified: lastModified.UTC().Format(time.UnixDate),
				RemovedKeys:  strconv.Itoa(int(0)),
				TotalKeys:    strconv.Itoa(int(0) + size),
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
	counters := storages.LocalTelemetryStorage.PeekCounters()
	errorCount := int64(0)
	for key, counter := range counters {
		if key != "backend::request.ok" {
			errorCount += counter
		}
	}

	// SdkCounters
	sdkCounters := int64(0)
	sdkErrorCount := int64(0)
	for key, counter := range storages.TelemetryStorage.PeekCounters() {
		if !strings.Contains(key, "ok") {
			sdkErrorCount += counter
		}
		sdkCounters += counter
	}

	return Metrics{
		SplitsNumber:                 strconv.Itoa(len(splitNames)),
		SegmentsNumber:               strconv.Itoa(segmentNames.Size()),
		LoggedErrors:                 formatNumber(log.ErrorDashboard.Counts()),
		LoggedMessages:               log.ErrorDashboard.Messages(),
		RequestErrorFormatted:        formatNumber(sdkErrorCount),
		RequestOkFormatted:           formatNumber(sdkCounters),
		SdksTotalRequests:            formatNumber(sdkCounters + sdkErrorCount),
		BackendTotalRequests:         formatNumber(counters["backend::request.ok"] + errorCount),
		BackendRequestOkFormatted:    formatNumber(counters["backend::request.ok"]),
		BackendRequestErrorFormatted: formatNumber(errorCount),
		SplitRows:                    parseCachedSplits(storages.SplitStorage),
		SegmentRows:                  parseCachedSegments(storages.SplitStorage, storages.SegmentStorage),
		LatenciesGroupDataBackend:    "[" + parseBackendStats(storages.LocalTelemetryStorage) + "]",
		BackendRequestOk:             strconv.Itoa(int(counters["backend::request.ok"])),
		BackendRequestError:          strconv.Itoa(int(errorCount)),
		LatenciesGroupData:           "[" + parseSDKStats(storages.TelemetryStorage) + "]",
		RequestOk:                    strconv.Itoa(int(sdkCounters)),
		RequestError:                 strconv.Itoa(int(sdkErrorCount)),
		EventsQueueSize:              parseEventsSize(storages.EventStorage),
		ImpressionsQueueSize:         parseImpressionSize(storages.ImpressionStorage),
		EventsLambda:                 parseEventsLambda(),
		ImpressionsLambda:            parseImpressionsLambda(),
	}
}
