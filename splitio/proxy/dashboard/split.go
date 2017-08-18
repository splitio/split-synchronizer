package dashboard

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
)

var splitRow = `<tr class="splitItem">
  <td><span class="splitItemName">{{name}}</span></td>
  <td class="{{statusColor}}">{{status}}</td>
  <td class="{{killedColor}}">{{killed}}</td>
  <td>{{treatments}}</td>
  <td>{{lastModified}}</td>
</tr>`

// ParseSplit returns parsed HTML for split table
func ParseSplit(splitJSON string) string {
	var html = splitRow

	var splitDto api.SplitDTO
	err := json.Unmarshal([]byte(splitJSON), &splitDto)
	if err != nil {
		log.Error.Println("Error parsing split JSON for Dashboard", err)
		return ""
	}

	html = strings.Replace(html, "{{name}}", splitDto.Name, 1)

	//STATUS
	html = strings.Replace(html, "{{status}}", splitDto.Status, 1)
	if splitDto.Status != "ACTIVE" {
		html = strings.Replace(html, "{{statusColor}}", "danger", 1)
	} else {
		html = strings.Replace(html, "{{statusColor}}", "", 1)
	}

	//KILLED
	html = strings.Replace(html, "{{killed}}", strconv.FormatBool(splitDto.Killed), 1)
	if splitDto.Killed {
		html = strings.Replace(html, "{{killedColor}}", "danger", 1)
	} else {
		html = strings.Replace(html, "{{killedColor}}", "", 1)
	}

	//TREATMENTS
	treatmets := make(map[string]bool)
	for _, c := range splitDto.Conditions {
		for _, p := range c.Partitions {
			if p.Treatment == splitDto.DefaultTreatment {
				treatmets[p.Treatment] = true
			} else {
				treatmets[p.Treatment] = false
			}
		}
	}
	treatmetsHTML := ""
	for t, d := range treatmets {
		if d {
			treatmetsHTML += fmt.Sprintf(", <strong>%s</strong>", t)
		} else {
			treatmetsHTML += fmt.Sprintf(", %s", t)
		}
	}
	html = strings.Replace(html, "{{treatments}}", treatmetsHTML[1:], 1)

	// LAST MODIFIED
	lastModified := time.Unix(0, splitDto.ChangeNumber*int64(time.Millisecond))
	html = strings.Replace(html, "{{lastModified}}", lastModified.UTC().Format(time.UnixDate), 1)

	return html
}
