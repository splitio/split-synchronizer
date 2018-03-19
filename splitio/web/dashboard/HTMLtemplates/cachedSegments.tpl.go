package HTMLtemplates

// CachedSegmentsTPLVars html representation of segments list
type CachedSegmentsTPLVars struct {
	Segments []*CachedSegmentRowTPLVars
}

// CachedSegmentRowTPLVars template variables
type CachedSegmentRowTPLVars struct {
	Name         string
	ActiveKeys   string
	LastModified string
}

// CachedSegmentsTPL is a html representation of segments list
var CachedSegmentsTPL = `
{{range .Segments}}
<tr>
	<td>
		<a id="showKeys-{{.Name}}" href="#" onclick="javascript:getKeys('{{.Name}}');return false;" class="showKeysLnk btn-xs"><span class="glyphicon glyphicon-menu-right" aria-hidden="true"></span></a>
	</td>
  <td>{{.Name}}</td>
  <td>{{.ActiveKeys}}</td>
  <td>{{.LastModified}}</td>
</tr>
<tr id="segmentKeysDetailedList-{{.Name}}" class="segmentKeysDetailedList filterDisplayNone">
	<td colspan="6">
	<div class="row">
		<div class="col-md-4">
			<div class="input-group">
				<input type="text" id="filterSegmentKeyInput-{{.Name}}" class="form-control input-sm filterSegmentKeyInput" placeholder="Filter by Key">
				<span class="input-group-btn">
					<button class="btn btn-default btn-sm" type="button" onclick="javascript:filterSegmentKeys('{{.Name}}');"><span class="glyphicon glyphicon-filter" aria-hidden="true"></span></button>
					<button class="btn btn-default btn-sm" type="button" onclick="javascript:resetFilterSegmentKeys();"><span class="glyphicon glyphicon-remove" aria-hidden="true"></span></button>
				</span>
			</div>
		</div>
	</div>
	<table class="table table-condensed table-hover">
		<thead>
			<tr>
				<th>Key</th>
			</tr>
			</thead>
			<tbody id="segmentKeysDetailedList-tbody-{{.Name}}" class="segmentKeysDetailedList-tbody">

			</tbody>
		</table>
	</td>
</tr>
{{end}}`

// CachedSegmentKeysTPLVars segment keys list template container
type CachedSegmentKeysTPLVars struct {
	SegmentKeys []string
}

//CachedSegmentKeysTPL segment keys html template
var CachedSegmentKeysTPL = `
{{range .SegmentKeys}}
<tr class="segmentKeyItem">
  <td><span class="segmentKeyItemName">{{.}}</span></td>
</tr>
{{end}}`
