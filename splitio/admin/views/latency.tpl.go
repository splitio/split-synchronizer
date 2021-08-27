package views

// LatencySerieTPLVars template variables
type LatencySerieTPLVars struct {
	Label           string
	Data            string
	BackgroundColor string
	BorderColor     string
}

// LatencyForChart object definition
type LatencyForChart struct {
	Label           string  `json:"label"`
	Data            []int64 `json:"data"`
	BackgroundColor string  `json:"backgroundColor"`
	BorderColor     string  `json:"borderColor"`
	BorderWidth     int     `json:"borderWidth"`
}

// NewLatencyBucketsForChart builds a chart-redendering config object with latency data
func NewLatencyBucketsForChart(label string, data []int64, bgColor string, brColor string) LatencyForChart {
	return LatencyForChart{
		Label:           label,
		Data:            data,
		BackgroundColor: bgColor,
		BorderColor:     brColor,
		BorderWidth:     1,
	}
}

// LatencySerieTPL main menu string template
var LatencySerieTPL = `{
    label: '{{.Label}}',
    data: {{.Data}},
    backgroundColor:'{{.BackgroundColor}}',
    borderColor: '{{.BorderColor}}',
    borderWidth: 1
},`
