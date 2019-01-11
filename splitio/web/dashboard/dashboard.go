package dashboard

import (
	"bytes"
	"encoding/json"
	"strconv"
	"time"

	"text/template"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/stats"
	"github.com/splitio/split-synchronizer/splitio/storage"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
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

func (d *Dashboard) parse(name string, text string, data interface{}) string {
	buf := new(bytes.Buffer)
	tpl := template.Must(template.New(name).Parse(text))
	tpl.Execute(buf, data)
	return buf.String()
}

func (d *Dashboard) parseLatencySerieData(key string, label string, backgroundColor string, borderColor string) string {

	var toReturn string

	latencies := stats.Latencies()
	if ldata, ok := latencies[key]; ok {
		if serie, err := json.Marshal(ldata); err == nil {
			toReturn = d.parse(
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

func (d *Dashboard) parseSDKStats() string {
	var toReturn string

	toReturn += d.parseLatencySerieData(
		"/api/splitChanges",
		"/api/splitChanges",
		ToRGBAString(255, 159, 64, 0.2),
		ToRGBAString(255, 159, 64, 1))

	toReturn += d.parseLatencySerieData(
		"/api/segmentChanges/*",
		"/api/segmentChanges/*",
		ToRGBAString(54, 162, 235, 0.2),
		ToRGBAString(54, 162, 235, 1))

	toReturn += d.parseLatencySerieData(
		"/api/testImpressions/bulk",
		"/api/testImpressions/bulk",
		ToRGBAString(75, 192, 192, 0.2),
		ToRGBAString(75, 192, 192, 1))

	toReturn += d.parseLatencySerieData(
		"/api/events/bulk",
		"/api/events/bulk",
		ToRGBAString(255, 205, 86, 0.2),
		ToRGBAString(255, 205, 86, 1))

	toReturn += d.parseLatencySerieData(
		"/api/mySegments/*",
		"/api/mySegments/*",
		ToRGBAString(153, 102, 255, 0.2),
		ToRGBAString(153, 102, 255, 1))

	return toReturn
}

func (d *Dashboard) parseBackendStats() string {
	var toReturn string

	toReturn += d.parseLatencySerieData(
		"backend::/api/splitChanges",
		"/api/splitChanges",
		ToRGBAString(255, 159, 64, 0.2),
		ToRGBAString(255, 159, 64, 1))

	toReturn += d.parseLatencySerieData(
		"backend::/api/segmentChanges",
		"/api/segmentChanges/*",
		ToRGBAString(54, 162, 235, 0.2),
		ToRGBAString(54, 162, 235, 1))

	toReturn += d.parseLatencySerieData(
		"backend::/api/testImpressions/bulk",
		"/api/testImpressions/bulk",
		ToRGBAString(75, 192, 192, 0.2),
		ToRGBAString(75, 192, 192, 1))

	toReturn += d.parseLatencySerieData(
		"backend::/api/events/bulk",
		"/api/events/bulk",
		ToRGBAString(255, 205, 86, 0.2),
		ToRGBAString(255, 205, 86, 1))

	return toReturn
}

func (d *Dashboard) parseCachedSplits() string {
	cachedSplits, err := d.splitStorage.RawSplits()
	if err != nil {
		log.Error.Println("Error fetching cached splits")
		return ""
	}

	return d.parse(
		"CachedSplits",
		HTMLtemplates.CachedSplitsTPL,
		HTMLtemplates.NewCachedSplitsTPLVars(cachedSplits))
}

func (d *Dashboard) parseCachedSegments() string {

	cachedSegments, err := d.segmentStorage.RegisteredSegmentNames()
	if err != nil {
		log.Error.Println("Error fetching cached segment list")
		return ""
	}

	toRender := make([]*HTMLtemplates.CachedSegmentRowTPLVars, 0)
	for _, segment := range cachedSegments {

		activeKeys, err := d.segmentStorage.CountActiveKeys(segment)
		if err != nil {
			log.Warning.Printf("Error counting active keys for segment %s\n", segment)
		}

		removedKeys, err := d.segmentStorage.CountRemovedKeys(segment)
		if err != nil {
			log.Warning.Printf("Error counting removed keys for segment %s\n", segment)
		}

		// LAST MODIFIED
		changeNumber, err := d.segmentStorage.ChangeNumber(segment)
		if err != nil {
			log.Warning.Printf("Error fetching last update for segment %s\n", segment)
		}
		lastModified := time.Unix(0, changeNumber*int64(time.Millisecond))

		toRender = append(toRender,
			&HTMLtemplates.CachedSegmentRowTPLVars{
				ProxyMode:    d.proxy,
				Name:         segment,
				ActiveKeys:   strconv.Itoa(int(activeKeys)),
				LastModified: lastModified.UTC().Format(time.UnixDate),
				RemovedKeys:  strconv.Itoa(int(removedKeys)),
				TotalKeys:    strconv.Itoa(int(removedKeys) + int(activeKeys)),
			})
	}

	return d.parse(
		"CachedSegemtns",
		HTMLtemplates.CachedSegmentsTPL,
		HTMLtemplates.CachedSegmentsTPLVars{Segments: toRender})
}

func (d *Dashboard) parseEventsSize() string {
	eventsStorageAdapter := redis.NewEventStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	size := eventsStorageAdapter.Size(eventsStorageAdapter.GetQueueNamespace())

	eventsSize := strconv.FormatInt(size, 10)

	return eventsSize
}

func (d *Dashboard) parseImpressionSize() string {
	impressionsStorageAdapter := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	size := impressionsStorageAdapter.Size(impressionsStorageAdapter.GetQueueNamespace())

	impressionsSize := strconv.FormatInt(size, 10)

	return impressionsSize
}

//HTML returns parsed HTML code
func (d *Dashboard) HTML() string {
	counters := stats.Counters()

	splitNames, err := d.splitStorage.SplitsNames()
	if err != nil {
		log.Error.Println("Error reading splits, maybe storage has not been initialized yet")
	}

	segmentNames, err := d.segmentStorage.RegisteredSegmentNames()
	if err != nil {
		log.Error.Println("Error reading segments, maybe storage has not been initialized yet")
	}

	//---> SDKs stats
	latenciesGroupDataSDK := d.parseSDKStats()

	//---> Backend stats
	latenciesGroupDataBackend := d.parseBackendStats()

	// Cached data
	cachedSplits := d.parseCachedSplits()
	cachedSegments := d.parseCachedSegments()

	// Queue data
	impressionsQueueSize := ""
	eventsQueueSize := ""
	if !d.proxy {
		impressionsQueueSize = d.parseImpressionSize()
		eventsQueueSize = d.parseEventsSize()
	}

	//Parsing main menu
	d.mainMenuTpl = d.parse(
		"MainMenu",
		HTMLtemplates.MainMenuTPL,
		HTMLtemplates.MainMenuTPLVars{ProxyMode: d.proxy})

	//Rendering layout
	d.mainMenuTpl = d.parse(
		"Layout",
		HTMLtemplates.LayoutTPL,
		HTMLtemplates.LayoutTPLVars{
			DashboardTitle:              d.title,
			ProxyMode:                   d.proxy,
			MainMenu:                    d.mainMenuTpl,
			Uptime:                      stats.UptimeFormated(),
			LoggedErrors:                FormatNumber(log.ErrorDashboard.Counts()),
			Version:                     splitio.Version,
			LoggedMessages:              log.ErrorDashboard.Messages(),
			SplitsNumber:                strconv.Itoa(len(splitNames)),
			SegmentsNumber:              strconv.Itoa(len(segmentNames)),
			RequestError:                strconv.Itoa(int(counters["request.error"])),
			RequestErrorFormated:        FormatNumber(counters["request.error"]),
			RequestOk:                   strconv.Itoa(int(counters["request.ok"])),
			RequestOkFormated:           FormatNumber(counters["request.ok"]),
			SdksTotalRequests:           FormatNumber(counters["request.ok"] + counters["request.error"]),
			BackendTotalRequests:        FormatNumber(counters["backend::request.ok"] + counters["backend::request.error"]),
			BackendRequestOkFormated:    FormatNumber(counters["backend::request.ok"]),
			BackendRequestErrorFormated: FormatNumber(counters["backend::request.error"]),
			BackendRequestOk:            strconv.Itoa(int(counters["backend::request.ok"])),
			BackendRequestError:         strconv.Itoa(int(counters["backend::request.error"])),
			LatenciesGroupData:          latenciesGroupDataSDK,
			LatenciesGroupDataBackend:   latenciesGroupDataBackend,
			SplitRows:                   cachedSplits,
			SegmentRows:                 cachedSegments,
			ImpressionsQueueSize:        impressionsQueueSize,
			EventsQueueSize:             eventsQueueSize,
		})

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

	return d.parse(
		"SegmentKeys",
		HTMLtemplates.CachedSegmentKeysTPL,
		HTMLtemplates.CachedSegmentKeysTPLVars{ProxyMode: d.proxy, SegmentKeys: segmentKeys},
	)
}
