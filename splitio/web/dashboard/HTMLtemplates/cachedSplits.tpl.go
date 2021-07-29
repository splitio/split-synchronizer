package HTMLtemplates

import (
	"fmt"
	"strconv"
	"time"

	"github.com/splitio/go-split-commons/v4/dtos"
)

// CachedSplitsTPLVars list of cached splits
type CachedSplitsTPLVars struct {
	Splits []*CachedSplitRowTPLVars
}

// NewCachedSplitsTPLVars creates a cached split representation
func NewCachedSplitsTPLVars(splits []dtos.SplitDTO) *CachedSplitsTPLVars {

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
func newCachedSplitRowTPLVars(split dtos.SplitDTO) *CachedSplitRowTPLVars {

	toReturn := &CachedSplitRowTPLVars{}

	toReturn.Name = split.Name

	// STATUS
	toReturn.Status = split.Status
	if split.Status != "ACTIVE" {
		toReturn.StatusColor = "danger"
	} else {
		toReturn.StatusColor = ""
	}

	// KILLED
	toReturn.Killed = strconv.FormatBool(split.Killed)
	if split.Killed {
		toReturn.KilledColor = "danger"
	} else {
		toReturn.KilledColor = ""
	}

	// TREATMENTS
	treatmets := make(map[string]bool)
	for _, c := range split.Conditions {
		for _, p := range c.Partitions {
			if p.Treatment == split.DefaultTreatment {
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
	lastModified := time.Unix(0, split.ChangeNumber*int64(time.Millisecond))
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
