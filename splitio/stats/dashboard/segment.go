package dashboard

import (
	"strconv"
	"strings"
	"time"

	"github.com/splitio/go-agent/splitio/storage/boltdb/collections"
)

var segmentRow = `<tr>
	<td>
		<a id="showKeys-{{name}}" href="#" onclick="javascript:getKeys('{{name}}');return false;" class="showKeysLnk btn-xs"><span class="glyphicon glyphicon-menu-right" aria-hidden="true"></span></a>
	</td>
  <td>{{name}}</td>
  <td>{{totalKeys}}</td>
  <td>{{removedKeys}}</td>
  <td>{{addedKeys}}</td>
  <td>{{lastModified}}</td>
</tr>
<tr id="segmentKeysDetailedList-{{name}}" class="segmentKeysDetailedList filterDisplayNone">
	<td colspan="6">
	<table class="table table-condensed table-hover">
		<thead>
			<tr>
				<th>Key</th>
				<th>Removed</th>
				<th>Last Modified</th>
			</tr>
			</thead>
			<tbody id="segmentKeysDetailedList-tbody-{{name}}" class="segmentKeysDetailedList-tbody">

			</tbody>
		</table>
	</td>
</tr>`

var segmentKeyRow = `<tr class="{{removedColor}}">
  <td>{{name}}</td>
  <td>{{removed}}</td>
  <td>{{lastModified}}</td>
</tr>`

// ParseSegment parse segment row
func ParseSegment(segment collections.SegmentChangesItem) string {
	var html = segmentRow

	html = strings.Replace(html, "{{name}}", segment.Name, 5)
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

	/*var segmentKeysRow = ""
	for _, skey := range segment.Keys {
		segmentKeysRow += ParseSegmentKey(skey)
	}

	html = strings.Replace(html, "{{segment_keys}}", segmentKeysRow, 1)*/
	return html
}

func ParseSegmentKey(key collections.SegmentKey) string {
	var html = segmentKeyRow

	html = strings.Replace(html, "{{name}}", key.Name, 1)
	html = strings.Replace(html, "{{removed}}", strconv.FormatBool(key.Removed), 1)
	if key.Removed {
		html = strings.Replace(html, "{{removedColor}}", "danger", 1)
	} else {
		html = strings.Replace(html, "{{removedColor}}", "", 1)
	}
	lastModified := time.Unix(0, int64(key.ChangeNumber)*int64(time.Millisecond))
	html = strings.Replace(html, "{{lastModified}}", lastModified.UTC().Format(time.UnixDate), 1)

	return html
}
