package HTMLtemplates

// MainMenuTPLVars template variables
type MainMenuTPLVars struct {
	ProxyMode bool
}

// MainMenuTPL main menu string template
var MainMenuTPL = `
<ul class="nav nav-tabs split-main-tabs" role="tablist">
	<li role="presentation" class="active"><a href="#split-dashboard" aria-controls="split-dashboard" role="tab" data-toggle="tab"><span class="glyphicon glyphicon-th" aria-hidden="true"></span>&nbsp;Dashboard</a></li>
	{{if .ProxyMode}}
		<li role="presentation"><a href="#sdk-stats" aria-controls="sdk-stats" role="tab" data-toggle="tab"><span class="glyphicon glyphicon-stats" aria-hidden="true"></span>&nbsp;SDK stats</a></li>
	{{end}}
	<li role="presentation"><a href="#backend-stats" aria-controls="backend-stats" role="tab" data-toggle="tab"><span class="glyphicon glyphicon-stats" aria-hidden="true"></span>&nbsp;Split stats</a></li>
	<li role="presentation"><a href="#data-inspector" aria-controls="data-inspector" role="tab" data-toggle="tab"><span class="glyphicon glyphicon-search" aria-hidden="true"></span>&nbsp;Data inspector</a></li>
</ul>
`
