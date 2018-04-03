package HTMLtemplates

// LatencySerieTPLVars template variables
type LatencySerieTPLVars struct {
	Label           string
	Data            string
	BackgroundColor string
	BorderColor     string
}

// LatencySerieTPL main menu string template
var LatencySerieTPL = `{
    label: '{{.Label}}',
    data: {{.Data}},
    backgroundColor:'{{.BackgroundColor}}',
    borderColor: '{{.BorderColor}}',
    borderWidth: 1
},`
