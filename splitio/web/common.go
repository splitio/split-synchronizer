package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"text/template"
	"time"

	"github.com/splitio/split-synchronizer/appcontext"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/stats"
	"github.com/splitio/split-synchronizer/splitio/storage"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
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
	EventsDelta                  string   `json:"eventsDelta"`
	ImpressionsDelta             string   `json:"impressionsDelta"`
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

func parseLatencySerieData(key string, label string, backgroundColor string, borderColor string) string {
	var toReturn string

	latencies := stats.Latencies()
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

func parseSDKStats() string {
	var toReturn string

	toReturn += parseLatencySerieData(
		"/api/splitChanges",
		"/api/splitChanges",
		toRGBAString(255, 159, 64, 0.2),
		toRGBAString(255, 159, 64, 1))

	toReturn += parseLatencySerieData(
		"/api/segmentChanges/*",
		"/api/segmentChanges/*",
		toRGBAString(54, 162, 235, 0.2),
		toRGBAString(54, 162, 235, 1))

	toReturn += parseLatencySerieData(
		"/api/testImpressions/bulk",
		"/api/testImpressions/bulk",
		toRGBAString(75, 192, 192, 0.2),
		toRGBAString(75, 192, 192, 1))

	toReturn += parseLatencySerieData(
		"/api/events/bulk",
		"/api/events/bulk",
		toRGBAString(255, 205, 86, 0.2),
		toRGBAString(255, 205, 86, 1))

	toReturn += parseLatencySerieData(
		"/api/mySegments/*",
		"/api/mySegments/*",
		toRGBAString(153, 102, 255, 0.2),
		toRGBAString(153, 102, 255, 1))

	return toReturn
}

func parseBackendStats() string {
	var toReturn string

	toReturn += parseLatencySerieData(
		"backend::/api/splitChanges",
		"/api/splitChanges",
		toRGBAString(255, 159, 64, 0.2),
		toRGBAString(255, 159, 64, 1))

	toReturn += parseLatencySerieData(
		"backend::/api/segmentChanges",
		"/api/segmentChanges/*",
		toRGBAString(54, 162, 235, 0.2),
		toRGBAString(54, 162, 235, 1))

	toReturn += parseLatencySerieData(
		"backend::/api/testImpressions/bulk",
		"/api/testImpressions/bulk",
		toRGBAString(75, 192, 192, 0.2),
		toRGBAString(75, 192, 192, 1))

	toReturn += parseLatencySerieData(
		"backend::/api/events/bulk",
		"/api/events/bulk",
		toRGBAString(255, 205, 86, 0.2),
		toRGBAString(255, 205, 86, 1))

	return toReturn
}

func parseCachedSplits(splitStorage storage.SplitStorage) string {
	cachedSplits, err := splitStorage.RawSplits()
	if err != nil {
		log.Error.Println("Error fetching cached splits")
		return ""
	}

	return ParseTemplate(
		"CachedSplits",
		HTMLtemplates.CachedSplitsTPL,
		HTMLtemplates.NewCachedSplitsTPLVars(cachedSplits))
}

func parseCachedSegments(segmentStorage storage.SegmentStorage) string {
	cachedSegments, err := segmentStorage.RegisteredSegmentNames()
	if err != nil {
		log.Error.Println("Error fetching cached segment list")
		return ""
	}

	toRender := make([]*HTMLtemplates.CachedSegmentRowTPLVars, 0)
	for _, segment := range cachedSegments {

		activeKeys, err := segmentStorage.CountActiveKeys(segment)
		if err != nil {
			log.Warning.Printf("Error counting active keys for segment %s\n", segment)
		}

		removedKeys, err := segmentStorage.CountRemovedKeys(segment)
		if err != nil {
			log.Warning.Printf("Error counting removed keys for segment %s\n", segment)
		}

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
				ActiveKeys:   strconv.Itoa(int(activeKeys)),
				LastModified: lastModified.UTC().Format(time.UnixDate),
				RemovedKeys:  strconv.Itoa(int(removedKeys)),
				TotalKeys:    strconv.Itoa(int(removedKeys) + int(activeKeys)),
			})
	}

	return ParseTemplate(
		"CachedSegments",
		HTMLtemplates.CachedSegmentsTPL,
		HTMLtemplates.CachedSegmentsTPLVars{Segments: toRender})
}

func parseEventsSize() string {
	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		return "0"
	}

	eventsStorageAdapter := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	size := eventsStorageAdapter.Size()

	eventsSize := strconv.FormatInt(size, 10)

	return eventsSize
}

func parseImpressionSize() string {
	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		return "0"
	}

	impressionsStorageAdapter := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	size := impressionsStorageAdapter.Size()

	impressionsSize := strconv.FormatInt(size, 10)

	return impressionsSize
}

func parseEventsDelta() string {
	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		return "0"
	}
	delta := task.GetEventsDelta()
	if delta > 10 {
		delta = 10
	}
	return strconv.FormatFloat(delta, 'f', 2, 64)
}

func parseImpressionsDelta() string {
	if appcontext.ExecutionMode() == appcontext.ProxyMode {
		return "0"
	}
	delta := task.GetImpressionsDelta()
	if delta > 10 {
		delta = 10
	}
	return strconv.FormatFloat(delta, 'f', 2, 64)
}

// GetMetrics data
func GetMetrics(splitStorage storage.SplitStorage, segmentStorage storage.SegmentStorage) Metrics {
	splitNames, err := splitStorage.SplitsNames()
	if err != nil {
		log.Error.Println("Error reading splits, maybe storage has not been initialized yet")
	}

	segmentNames, err := segmentStorage.RegisteredSegmentNames()
	if err != nil {
		log.Error.Println("Error reading segments, maybe storage has not been initialized yet")
	}

	// Counters
	counters := stats.Counters()

	return Metrics{
		SplitsNumber:                 strconv.Itoa(len(splitNames)),
		SegmentsNumber:               strconv.Itoa(len(segmentNames)),
		LoggedErrors:                 formatNumber(log.ErrorDashboard.Counts()),
		LoggedMessages:               log.ErrorDashboard.Messages(),
		RequestErrorFormatted:        formatNumber(counters["request.error"]),
		RequestOkFormatted:           formatNumber(counters["request.ok"]),
		SdksTotalRequests:            formatNumber(counters["request.ok"] + counters["request.error"]),
		BackendTotalRequests:         formatNumber(counters["backend::request.ok"] + counters["backend::request.error"]),
		BackendRequestOkFormatted:    formatNumber(counters["backend::request.ok"]),
		BackendRequestErrorFormatted: formatNumber(counters["backend::request.error"]),
		SplitRows:                    parseCachedSplits(splitStorage),
		SegmentRows:                  parseCachedSegments(segmentStorage),
		LatenciesGroupDataBackend:    "[" + parseBackendStats() + "]",
		BackendRequestOk:             strconv.Itoa(int(counters["backend::request.ok"])),
		BackendRequestError:          strconv.Itoa(int(counters["backend::request.error"])),
		LatenciesGroupData:           "[" + parseSDKStats() + "]",
		RequestOk:                    strconv.Itoa(int(counters["request.ok"])),
		RequestError:                 strconv.Itoa(int(counters["request.error"])),
		EventsQueueSize:              parseEventsSize(),
		ImpressionsQueueSize:         parseImpressionSize(),
		EventsDelta:                  parseEventsDelta(),
		ImpressionsDelta:             parseImpressionsDelta(),
	}
}
