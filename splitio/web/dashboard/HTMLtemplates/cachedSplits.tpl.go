package HTMLtemplates

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
)

// CachedSplitsTPLVars list of cached splits
type CachedSplitsTPLVars struct {
	Splits []*CachedSplitRowTPLVars
}

// NewCachedSplitsTPLVars creates a cached split representation
func NewCachedSplitsTPLVars(splits []string) *CachedSplitsTPLVars {

	toReturn := &CachedSplitsTPLVars{Splits: make([]*CachedSplitRowTPLVars, 0)}

	for _, split := range splits {
		toReturn.Splits = append(toReturn.Splits, newCachedSplitRowTPLVars(split))
	}

	return toReturn
}

// CachedSplitRowTPLVars template variables
type CachedSplitRowTPLVars struct {
	Name         string
	Status       string
	StatusColor  string
	Killed       string
	KilledColor  string
	Treatments   string
	LastModified string
}

// NewCachedSplitRowTPLVars return an instance of CachedSplitRowTPLVars
func newCachedSplitRowTPLVars(splitJSON string) *CachedSplitRowTPLVars {

	toReturn := &CachedSplitRowTPLVars{}

	var splitDto api.SplitDTO
	err := json.Unmarshal([]byte(splitJSON), &splitDto)
	if err != nil {
		log.Error.Println("Error parsing split JSON for Dashboard", err)
		return nil
	}

	toReturn.Name = splitDto.Name

	//STATUS
	toReturn.Status = splitDto.Status
	if splitDto.Status != "ACTIVE" {
		toReturn.StatusColor = "danger"
	} else {
		toReturn.StatusColor = ""
	}

	//KILLED
	toReturn.Killed = strconv.FormatBool(splitDto.Killed)
	if splitDto.Killed {
		toReturn.KilledColor = "danger"
	} else {
		toReturn.KilledColor = ""
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
	toReturn.Treatments = treatmetsHTML[1:]

	// LAST MODIFIED
	lastModified := time.Unix(0, splitDto.ChangeNumber*int64(time.Millisecond))
	toReturn.LastModified = lastModified.UTC().Format(time.UnixDate)

	return toReturn
}

// CachedSplitsTPL main menu string template
var CachedSplitsTPL = `
{{range .Splits}}
<tr class="splitItem">
  <td><span class="splitItemName">{{.Name}}</span></td>
  <td class="{{.StatusColor}}">{{.Status}}</td>
  <td class="{{.KilledColor}}">{{.Killed}}</td>
  <td>{{.Treatments}}</td>
  <td>{{.LastModified}}</td>
</tr>
{{end}}
`
