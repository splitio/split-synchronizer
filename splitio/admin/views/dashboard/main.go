package dashboard

import (
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
)

var funcs = map[string]interface{}{
	"serializeIncomingData": func(v interface{}) template.JS {
		b, _ := json.Marshal(v)
		return template.JS(b)
	},
}

const main = `
<!doctype html>
<html lang="en">

<head>
  <meta charset="utf-8">
  <meta http-equiv="x-ua-compatible" content="ie=edge">
  <meta name="viewport" content="width=device-width, initial-scale=1">

  <title>Split Sync - Dashboard | {{.DashboardTitle}}</title>
  
  {{template "ChartJS" .}}
  {{template "BootstrapMainStyle" .}}
  {{template "BootstrapThemeStyle" .}}
  {{template "BootstrapScript" .}}
  {{template "CustomStyle" .}}
</head>

<body>
  <div class="container-fluid">
    <div class="row">
      <div class="col-md-12" style="background-color: #182A3C;">
        <div class="logosvg pull-left">
          <p class="navbar-brand pull-right split-nav-title" href="#">| {{.RunningMode}}</p>
          <p class="navbar-brand pull-right split-nav-title" href="#">{{.DashboardTitle}}</p>
	  {{template "SplitLogo" .}}
        </div>
        <div class="pull-right" style="text-align: center; padding-right: 10px;">
	  <p style="padding-top: 8px; color: white; margin: 0px; font-weight: bold; text-align: right"><span>{{.Version}}</span></p>
          <p class="navbar-text navbar-right" style="padding-top: 0px;margin-bottom: 15px;margin-top: 0px;color:white;min-width: 175px;height: 10px;">
            <a href="#" onclick="javascript:sendSignal('graceful'); return false;" class="navbar-link">
	      <span class="label label-success">Graceful stop</span>
	    </a>&nbsp;&nbsp;
            <a href="#" onclick="javascript:sendSignal('force'); return false;" class="navbar-link">
	      <span class="label label-danger">Force stop</span>
	    </a>
          </p>
        </div>
        <div class="pull-right">
          {{template "Menu" .}}
        </div>
      </div>
    </div>

    <div class="tab-content">
      {{template "Cards" .}}
      {{template "UpstreamStats" .}}
      {{if .ProxyMode}}{{template "SdkStats" .}}{{end}}
      {{if not .ProxyMode}}{{template "QueueManager" .}}{{end}}
      {{template "DataInspector" .}}
    </div>
  </div>
   {{template "MainScript" .}}
</body>
</html>
`

type GlobalStats struct {
	BackendTotalRequests   int64            `json:"backendTotalRequests"`
	RequestsOk             int64            `json:"requestsOk"`
	RequestsErrored        int64            `json:"requestsErrored"`
	BackendRequestsOk      int64            `json:"backendRequestsOk"`
	BackendRequestsErrored int64            `json:"backendRequestsErrored"`
	SdksTotalRequests      int64            `json:"sdksTotalRequests"`
	LoggedErrors           int64            `json:"loggedErrors"`
	LoggedMessages         []string         `json:"loggedMessages"`
	Splits                 []SplitSummary   `json:"splits"`
	Segments               []SegmentSummary `json:"segments"`
	Latencies              []ChartJSData    `json:"latencies"`
	BackendLatencies       []ChartJSData    `json:"backendLatencies"`
	ImpressionsQueueSize   int64            `json:"impressionsQueueSize"`
	ImpressionsLambda      float64          `json:"impressionsLambda"`
	EventsQueueSize        int64            `json:"eventsQueueSize"`
	EventsLambda           float64          `json:"eventsLambda"`
	Uptime                 int64            `json:"uptime"`
}

type Health struct {
	SDKServerStatus   bool  `json:"sdkServerStatus"`
	EventServerStatus bool  `json:"eventsServerStatus"`
	AuthServerStatus  bool  `json:"authServerStatus"`
	StorageStatus     bool  `json:"storageStatus"`
	HealthySince      int64 `json:"healthySince"`
}

type DashboardInitializationVars struct {
	DashboardTitle string
	RunningMode    string
	Version        string
	ProxyMode      bool
	RefreshTime    int64
	Stats          GlobalStats `json:"stats"`
	Health         Health      `json:"health"`
}

// SplitSummary encapsulates a minimalistic view of split properties to be presented in the dashboard
type SplitSummary struct {
	Name             string   `json:"name"`
	Active           bool     `json:"active"`
	Killed           bool     `json:"killed"`
	DefaultTreatment string   `json:"defaultTreatment"`
	Treatments       []string `json:"treatments"`
	LastModified     string   `json:"cn"`
}

// SegmentSummary encapsulates a minimalistic view of segment properties to be presented in the dashboard
type SegmentSummary struct {
	Name         string `json:"name"`
	TotalKeys    int    `json:"totalKeys"`
	RemovedKeys  int    `json:"removedKeys"`
	ActiveKeys   int    `json:"activeKeys"`
	LastModified string `json:"cn"`
}

// SegmentKeySummary encapsulates basic information associated to the key in proxy mode
// (fields other than name are empty when running as producer
type SegmentKeySummary struct {
	Name         string `json:"name"`
	Removed      bool   `json:"removed"`
	ChangeNumber int64  `json:"cn"`
}

// RGBA bundles input to CSS's rgba function
type RGBA struct {
	Red   int32
	Green int32
	Blue  int32
	Alpha float32
}

// MakeRGBA constructs an RGBA object from it's components
func MakeRGBA(r int32, g int32, b int32, a float32) RGBA {
	return RGBA{Red: r, Green: g, Blue: b, Alpha: a}
}

// MarshalJSON encodes the RGBA struct into a CSS rgba call with the correct parameters
func (r RGBA) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"rgba(%d, %d, %d, %.2f)\"", r.Red, r.Green, r.Blue, r.Alpha)), nil
}

// ChartJSData bundles input to ChartJS rendering tool
type ChartJSData struct {
	Label           string        `json:"label"`
	Data            []interface{} `json:"data"`
	BackgroundColor RGBA          `json:"backgroundColor"`
	BorderColor     RGBA          `json:"borderColor"`
	BorderWidth     int           `json:"borderWidth"`
}

// AssembleDashboardTemplate concatenates the pieces in correct order to build the whole dashboard layout template
func AssembleDashboardTemplate() (*template.Template, error) {
	return template.New("DashboardLayout").Funcs(funcs).Parse(strings.Join([]string{
		// Embedded template definitions (MUST appear before the main layout)
		cards,
		charjs,
		bootstrapStyle,
		bootstrapTheme,
		customStyle,
		bootstrapScript,
		logo,
		sdkStats,
		upstreamStats,
		queueManager,
		dataInspector,
		menu,
		mainScript,
		// Main layout
		main,
	}, ""))
}
