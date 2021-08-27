package views

// CachedSegmentsTPLVars html representation of segments list
type CachedSegmentsTPLVars struct {
	Segments []*CachedSegmentRowTPLVars
}

// CachedSegmentRowTPLVars template variables
type CachedSegmentRowTPLVars struct {
	ProxyMode    bool
	Name         string
	ActiveKeys   string
	LastModified string
	TotalKeys    string
	RemovedKeys  string
	AddedKeys    string
}

// CachedSegmentsTPL is a html representation of segments list
var CachedSegmentsTPL = `
{{range .Segments}}
  <tr>
    {{if .ProxyMode}}
      <td>
        <a id="showKeys-{{.Name}}" href="#" onclick="javascript:getKeys('{{.Name}}');return false;" class="showKeysLnk btn-xs">
          <span class="glyphicon glyphicon-menu-right" aria-hidden="true"></span>
        </a>
      </td>
      <td>{{.Name}}</td>
      <td>{{.TotalKeys}}</td>
      <td>{{.RemovedKeys}}</td>
      <td>{{.ActiveKeys}}</td>
      <td>{{.LastModified}}</td>
    {{else}}
      <td>
        <a id="showKeys-{{.Name}}" href="#" onclick="javascript:getKeys('{{.Name}}');return false;" class="showKeysLnk btn-xs">
          <span class="glyphicon glyphicon-menu-right" aria-hidden="true"></span>
        </a>
      </td>
      <td>{{.Name}}</td>
      <td>{{.ActiveKeys}}</td>
      <td>{{.LastModified}}</td>
    {{end}}
  </tr>
  <tr id="segmentKeysDetailedList-{{.Name}}" class="segmentKeysDetailedList filterDisplayNone">
    <td colspan="6">
      <div class="row">
        <div class="col-md-4">
          <div class="input-group">
            <input type="text" id="filterSegmentKeyInput-{{.Name}}" class="form-control input-sm filterSegmentKeyInput" placeholder="Filter by Key">
            <span class="input-group-btn">
              <button class="btn btn-default btn-sm" type="button" onclick="javascript:filterSegmentKeys('{{.Name}}');">
                <span class="glyphicon glyphicon-filter" aria-hidden="true"></span>
              </button>
              <button class="btn btn-default btn-sm" type="button" onclick="javascript:resetFilterSegmentKeys();">
                <span class="glyphicon glyphicon-remove" aria-hidden="true"></span>
              </button>
            </span>
          </div>
        </div>
      </div>
      <table class="table table-condensed table-hover">
        <thead>
          <tr>
            {{if .ProxyMode}}
              <th>Key</th>
              <th>Removed</th>
              <th>Last Modified</th>
            {{else}}
              <th>Key</th>
            {{end}}
          </tr>
        </thead>
        <tbody id="segmentKeysDetailedList-tbody-{{.Name}}" class="segmentKeysDetailedList-tbody"></tbody>
      </table>
     </td>
  </tr>
{{end}}`

// CachedSegmentKeysTPLVars segment keys list template container
type CachedSegmentKeysTPLVars struct {
	ProxyMode   bool
	SegmentKeys []CachedSegmentKeysRowTPLVars
}

// CachedSegmentKeysRowTPLVars key data details
type CachedSegmentKeysRowTPLVars struct {
	Name         string
	RemovedColor string
	Removed      string
	LastModified string
}

//CachedSegmentKeysTPL segment keys html template
var CachedSegmentKeysTPL = `
{{if .ProxyMode}}
	{{range .SegmentKeys}}
		<tr class="{{.RemovedColor}} segmentKeyItem">
			<td><span class="segmentKeyItemName">{{.Name}}</span></td>
			<td>{{.Removed}}</td>
			<td>{{.LastModified}}</td>
	  </tr>

	{{end}}
{{else}}
	{{range .SegmentKeys}}
		<tr class="segmentKeyItem">
			<td><span class="segmentKeyItemName">{{.Name}}</span></td>
		</tr>
	{{end}}
{{end}}
`
