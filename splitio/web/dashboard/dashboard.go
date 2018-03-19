package dashboard

import (
	"bytes"
	"encoding/json"
	"strconv"
	"time"

	"text/template"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio"
	"github.com/splitio/split-synchronizer/splitio/proxy/dashboard"
	"github.com/splitio/split-synchronizer/splitio/stats"
	"github.com/splitio/split-synchronizer/splitio/storage"
	"github.com/splitio/split-synchronizer/splitio/web/dashboard/HTMLtemplates"
)

// Dashboard represents html dashboard class
type Dashboard struct {
	proxy          bool
	splitStorage   storage.SplitStorage
	segmentStorage storage.SegmentStorage
	layoutTpl      string
	mainMenuTpl    string
}

// NewDashboard returns an instance of Dashboard struct
func NewDashboard(isProxy bool, splitStorage storage.SplitStorage, segmentStorage storage.SegmentStorage) *Dashboard {
	return &Dashboard{proxy: isProxy, splitStorage: splitStorage, segmentStorage: segmentStorage}
}

func (d *Dashboard) parse(name string, text string, data interface{}) string {
	buf := new(bytes.Buffer)
	tpl := template.Must(template.New(name).Parse(text))
	tpl.Execute(buf, data)
	return buf.String()
}

func (d *Dashboard) parseBackendStats() string {
	var toReturn string

	latencies := stats.Latencies()
	if ldata, ok := latencies["backend::/api/splitChanges"]; ok {
		if serie, err := json.Marshal(ldata); err == nil {
			toReturn += d.parse(
				"backend::/api/splitChanges",
				HTMLtemplates.LatencySerieTPL,
				HTMLtemplates.LatencySerieTPLVars{
					Label:           "/api/splitChanges",
					BackgroundColor: "rgba(255, 159, 64, 0.2)",
					BorderColor:     "rgba(255, 159, 64, 1)",
					Data:            string(serie),
				})
		}
	}

	if ldata, ok := latencies["backend::/api/segmentChanges"]; ok {
		if serie, err := json.Marshal(ldata); err == nil {
			toReturn += d.parse(
				"backend::/api/segmentChanges",
				HTMLtemplates.LatencySerieTPL,
				HTMLtemplates.LatencySerieTPLVars{
					Label:           "/api/segmentChanges",
					BackgroundColor: "rgba(54, 162, 235, 0.2)",
					BorderColor:     "rgba(54, 162, 235, 1)",
					Data:            string(serie),
				})
		}
	}

	if ldata, ok := latencies["backend::/api/testImpressions/bulk"]; ok {
		if serie, err := json.Marshal(ldata); err == nil {
			toReturn += d.parse(
				"backend::/api/testImpressions/bulk",
				HTMLtemplates.LatencySerieTPL,
				HTMLtemplates.LatencySerieTPLVars{
					Label:           "backend::/api/testImpressions/bulk",
					BackgroundColor: "rgba(75, 192, 192, 0.2)",
					BorderColor:     "rgba(75, 192, 192, 1)",
					Data:            string(serie),
				})
		}
	}

	if ldata, ok := latencies["backend::/api/events/bulk"]; ok {
		if serie, err := json.Marshal(ldata); err == nil {
			toReturn += d.parse(
				"backend::/api/events/bulk",
				HTMLtemplates.LatencySerieTPL,
				HTMLtemplates.LatencySerieTPLVars{
					Label:           "backend::/api/events/bulk",
					BackgroundColor: "rgba(255, 205, 86, 0.2)",
					BorderColor:     "rgba(255, 205, 86, 1)",
					Data:            string(serie),
				})
		}
	}

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
			log.Error.Printf("Error counting active keys for segment %s\n", segment)
		}
		// LAST MODIFIED
		changeNumber, err := d.segmentStorage.ChangeNumber(segment)
		if err != nil {
			log.Error.Printf("Error fetching last update for segment %s\n", segment)
		}
		lastModified := time.Unix(0, changeNumber*int64(time.Millisecond))

		toRender = append(toRender,
			&HTMLtemplates.CachedSegmentRowTPLVars{
				Name:         segment,
				ActiveKeys:   strconv.Itoa(int(activeKeys)),
				LastModified: lastModified.UTC().Format(time.UnixDate),
			})
	}

	return d.parse(
		"CachedSegemtns",
		HTMLtemplates.CachedSegmentsTPL,
		HTMLtemplates.CachedSegmentsTPLVars{Segments: toRender})
}

//HTML returns parsed HTML code
func (d *Dashboard) HTML() string {
	counters := stats.Counters()

	splitNames, err := d.splitStorage.SplitsNames()
	if err != nil {
		log.Error.Println(err)
	}

	segmentNames, err := d.segmentStorage.RegisteredSegmentNames()
	if err != nil {
		log.Error.Println(err)
	}

	//---> Backend stats
	latenciesGroupDataBackend := d.parseBackendStats()

	// Cached data
	cachedSplits := d.parseCachedSplits()
	cachedSegments := d.parseCachedSegments()

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
			ProxyMode:                   d.proxy,
			MainMenu:                    d.mainMenuTpl,
			Uptime:                      stats.UptimeFormated(),
			LoggedErrors:                strconv.Itoa(int(log.ErrorDashboard.Counts())),
			Version:                     splitio.Version,
			LoggedMessages:              log.ErrorDashboard.Messages(),
			SplitsNumber:                strconv.Itoa(len(splitNames)),
			SegmentsNumber:              strconv.Itoa(len(segmentNames)),
			BackendTotalRequests:        dashboard.FormatNumber(counters["backend::request.ok"] + counters["backend::request.error"]),
			BackendRequestOkFormated:    dashboard.FormatNumber(counters["backend::request.ok"]),
			BackendRequestErrorFormated: dashboard.FormatNumber(counters["backend::request.error"]),
			BackendRequestOk:            strconv.Itoa(int(counters["backend::request.ok"])),
			BackendRequestError:         strconv.Itoa(int(counters["backend::request.error"])),
			LatenciesGroupDataBackend:   latenciesGroupDataBackend,
			SplitRows:                   cachedSplits,
			SegmentRows:                 cachedSegments,
		})

	return d.mainMenuTpl
}

// HTMLSegmentKeys return a html representation of segment's keys list
func (d *Dashboard) HTMLSegmentKeys(segmentName string) string {

	keys, err := d.segmentStorage.ActiveKeys(segmentName)
	if err != nil {
		log.Error.Println("Error fetching keys for segment:", segmentName)
		return ""
	}

	return d.parse(
		"SegmentKeys",
		HTMLtemplates.CachedSegmentKeysTPL,
		HTMLtemplates.CachedSegmentKeysTPLVars{SegmentKeys: keys},
	)
}
