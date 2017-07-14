package dashboard

import (
	"strconv"
	"strings"
	"time"

	"github.com/splitio/go-agent/splitio/storage/boltdb/collections"
)

var segmentRow = `<tr>
  <td>{{name}}</td>
  <td>{{totalKeys}}</td>
  <td>{{removedKeys}}</td>
  <td>{{addedKeys}}</td>
  <td>{{lastModified}}</td>
</tr>`

// ParseSegment parse segment row
func ParseSegment(segment collections.SegmentChangesItem) string {
	var html = segmentRow

	html = strings.Replace(html, "{{name}}", segment.Name, 1)
	html = strings.Replace(html, "{{totalKeys}}", strconv.FormatInt(int64(len(segment.Keys)), 10), 1)

	changeNumber := int64(0)
	removedKeys := 0
	addedKeys := 0
	for _, k := range segment.Keys {
		if k.ChangeNumber > changeNumber {
			changeNumber = k.ChangeNumber
		}
		if k.Removed {
			removedKeys++
		} else {
			addedKeys++
		}
	}

	html = strings.Replace(html, "{{removedKeys}}", strconv.FormatInt(int64(removedKeys), 10), 1)
	html = strings.Replace(html, "{{addedKeys}}", strconv.FormatInt(int64(addedKeys), 10), 1)

	lastModified := time.Unix(0, int64(changeNumber)*int64(time.Millisecond))
	html = strings.Replace(html, "{{lastModified}}", lastModified.UTC().Format(time.UnixDate), 1)

	return html
}
