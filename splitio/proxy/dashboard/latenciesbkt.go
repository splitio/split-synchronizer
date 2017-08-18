package dashboard

import (
	"encoding/json"
	"strings"

	"github.com/splitio/split-synchronizer/log"
)

var latenciesDataObject = `{
    label: '{{label}}',
    data: {{data}},
    backgroundColor:'{{backgroundColor}}',
    borderColor: '{{borderColor}}',
    borderWidth: 1
},`

// ParseLatencyBktDataSerie returns a javascript data serie
func ParseLatencyBktDataSerie(
	label string,
	latencies []int64,
	backgroundColor string,
	borderColor string) string {

	latenciesJSON, err := json.Marshal(latencies)
	if err != nil {
		log.Warning.Println("error:", err)
	}

	html := strings.Replace(latenciesDataObject, "{{label}}", label, 1)
	html = strings.Replace(html, "{{data}}", string(latenciesJSON), 1)
	html = strings.Replace(html, "{{backgroundColor}}", backgroundColor, 1)
	html = strings.Replace(html, "{{borderColor}}", borderColor, 1)

	return html
}
