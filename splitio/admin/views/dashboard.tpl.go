package views

import "html/template"

// Main is the main layout template for the dashboard to be rendered.
// Required vars:
// - Styles :: []string :: list of style sheets to be embedded at the head (Bootstrap)
// - HeadScripts :: []string :: list of scripts to be embedded at the head (ChartJS, JQuery, Bootstrap)
// - HeadScriptsSrc :: []string :: list of scripts with `src` = string to be embedded at the head
// - BottomScripts :: []string :: list of scripts with to be embedded at the bottom of the page
const main = `
<!doctype html>
<html lang="en">

<head>
<meta charset="utf-8">
<meta http-equiv="x-ua-compatible" content="ie=edge">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Split Sync - Dashboard | {{.DashboardTitle}}</title>
{{range .Styles}}
    <style>{{.}}</style>
{{end}}
{{range .HeadScripts}}
    <script>{{.}}</script>
{{end}}
{{range .HeadScriptsSrc}}
    <script src="{{.}}"></script>
{{end}}

<body>
  <div class="container-fluid">
    <div class="row">
      <div class="col-md-12" style="background-color: #182A3C;">
        <div class="logosvg pull-left">
          <p class="navbar-brand pull-right split-nav-title" href="#">| {{.RunningMode}}</p>
          <p class="navbar-brand pull-right split-nav-title" href="#">{{.DashboardTitle}}</p>
	  <!-- SPLIT LOGO GOES HERE -->
        </div>
        <div class="pull-right" style="text-align: center; padding-right: 10px;">
	  <p style="padding-top: 8px; color: white; margin: 0px; font-weight: bold; text-align: right"><span>{{.Version}}</span></p>
          <p class="navbar-text navbar-right" style="padding-top: 0px;margin-bottom: 15px;margin-top: 0px;color:white;min-width: 175px;height: 10px;">
            <a href="#" onclick="javascript:sendSignal('graceful'); return false;" class="navbar-link"><span class="label label-success">Graceful stop</span></a>&nbsp;&nbsp;
            <a href="#" onclick="javascript:sendSignal('force'); return false;" class="navbar-link"><span class="label label-danger">Force stop</span></a>
          </p>
        </div>
        <div class="pull-right">
          {{.MainMenu}}
        </div>
      </div>
    </div>

    <div class="tab-content">

      {{template "SdkStats" .}}

      {{if not .ProxyMode}}
	{{template "QueueStats" .}}
      {{end}}

      {{template "QueueManager" .}}
      {{template "DataInspector" .}}

    </div>
  </div>
  {{range .BottomScripts}}
    <script>{{.}}</script>
  {{end}}
</body>
</html>
`

const stats = `
{{define "SdkStats"}}
  <div role="tabpanel" class="tab-pane active" id="split-dashboard">
    <div class="row">
      <div class="col-md-3">
        <div class="gray1Box metricBox">
          <h4>Uptime</h4>
          <h1 id="uptime" class="centerText">{{.Uptime}}</h1>
        </div>
      </div>
      <div class="col-md-3">
        <div class="gray1Box metricBox">
          <h4>Healthy Since</h4>
          <h1 id="healthy_since" class="centerText">{{.HealthySince}}</h1>
        </div>
      </div>
      <div class="col-md-3">
        <div class="redBox metricBox">
          <h4>Logged Errors</h4>
          <h1 id="logged_errors" class="centerText">{{.LoggedErrors}}</h1>
        </div>
      </div>
  
      {{if .ProxyMode}}   
        <div class="col-md-3">
          <div class="gray2Box metricBox">
            <h4>SDKs Total Hits</h4>
            <h1 id="sdks_total_requests" class="centerText">{{.SdksTotalRequests}}</h1>
          </div>
        </div>
      {{else}}
        <div class="col-md-3">
          <div class="gray2Box metricBox">
            <h4>Backend Total Hits</h4>
            <h1 id="backend_total_requests" class="centerText">{{.BackendTotalRequests}}</h1>
          </div>
        </div>
      {{end}}
      
    </div>
  
    <div class="row">
      {{if .ProxyMode}} 
        <div class="col-md-6">
          <div class="gray2Box metricBox">
            <h4>Cached Splits</h4>
            <h1 id="splits_number" class="centerText">{{.SplitsNumber}}</h1>
          </div>
        </div>
        <div class="col-md-6">
          <div class="gray2Box metricBox">
            <h4>Cached Segments</h4>
            <h1 id="segments_number" class="centerText">{{.SegmentsNumber}}</h1>
          </div>
        </div>
      {{else}}
        <div class="col-md-2">
          <div class="gray1Box metricBox">
            <h4>Impressions Queue</h4>
            <h1 id="impressions_queue_value" class="centerText">{{.ImpressionsQueueSize}}</h1>
          </div>
        </div>
        <div class="col-md-2">
          <div class="gray1Box metricBox">
            <h4>Events Queue</h4>
            <h1 id="events_queue_value" class="centerText">{{.EventsQueueSize}}</h1>
          </div>
        </div>
        <div class="col-md-4">
          <div class="gray2Box metricBox">
            <h4>Cached Splits</h4>
            <h1 id="splits_number" class="centerText">{{.SplitsNumber}}</h1>
          </div>
        </div>
        <div class="col-md-4">
          <div class="gray2Box metricBox">
            <h4>Cached Segments</h4>
            <h1 id="segments_number" class="centerText">{{.SegmentsNumber}}</h1>
          </div>
        </div>
      {{end}}
    </div>
  
    {{if not .ProxyMode}} 
      <div class="row">
        <div class="col-md-2">
          <div class="gray1Box metricBox">
            <i class="popovers glyphicon glyphicon-question-sign"
              style="float: right;"
              data-toggle="popover-impressions"
              title="Lambda Impressions Eviction Calculation"
            >
            </i>
            <h4>Impressions Lambda</h4>
            <h1 id="impressions_lambda" class="centerText">{{.ImpressionsLambda}}</h1>
          </div>
        </div>
        <div class="col-md-2">
          <div class="gray1Box metricBox">
            <i class="popovers glyphicon glyphicon-question-sign"
              style="float: right;"
              data-toggle="popover-events"
              title="Lambda Events Eviction Calculation"
            >
            </i>
            <h4>Events Lambda</h4>
            <h1 id="events_lambda" class="centerText">{{.EventsLambda}}</h1>
          </div>
        </div>
        <div id="sdk_server_div_ok" class="col-md-2">
          <div class="green1Box metricBox">
            <h4>SDK Server</h4>
            <h1 id="sdk_server" class="centerText">OK</h1>
          </div>
        </div>
        <div id="sdk_server_div_error" class="col-md-2 hidden">
          <div class="red1Box metricBox">
            <h4>SDK Server</h4>
            <h1 id="sdk_server" class="centerText">ERROR</h1>
          </div>
        </div>
        <div id="event_server_div_ok" class="col-md-2">
          <div class="green1Box metricBox">
            <h4>Events Server</h4>
            <h1 id="event_server" class="centerText">OK</h1>
          </div>
        </div>
        <div id="event_server_div_error" class="col-md-2 hidden">
          <div class="red1Box metricBox">
            <h4>Events Server</h4>
            <h1 id="event_server" class="centerText">ERROR</h1>
          </div>
        </div>
        <div id="auth_server_div_ok" class="col-md-2">
          <div class="green1Box metricBox">
            <h4>Auth</h4>
            <h1 id="auth" class="centerText">OK</h1>
          </div>
        </div>
        <div id="auth_server_div_error" class="col-md-2 hidden">
          <div class="red1Box metricBox">
            <h4>Auth</h4>
            <h1 id="auth" class="centerText">ERROR</h1>
          </div>
        </div>
        <div id="storage_div_ok" class="col-md-1">
          <div class="green1Box metricBox">
            <h4>Storage</h4>
            <h1 id="storage" class="centerText">OK</h1>
          </div>
        </div>
        <div id="storage_div_error" class="col-md-1 hidden">
          <div class="red1Box metricBox">
            <h4>Storage</h4>
            <h1 id="storage" class="centerText">ERROR</h1>
          </div>
        </div>
        <div class="col-md-1">
          <div class="green1Box metricBox">
            <h4>Sync</h4>
            <h1 id="sync" class="centerText">OK</h1>
          </div>
        </div>
      </div>
    {{else}}
      <div class="row">
        <div id="sdk_server_div_ok" class="col-md-3">
          <div class="green1Box metricBox">
            <h4>SDK Server</h4>
            <h1 id="sdk_server" class="centerText">OK</h1>
          </div>
        </div>
        <div id="sdk_server_div_error" class="col-md-3 hidden">
          <div class="red1Box metricBox">
            <h4>SDK Server</h4>
            <h1 id="sdk_server" class="centerText">ERROR</h1>
          </div>
        </div>
        <div id="event_server_div_ok" class="col-md-3">
          <div class="green1Box metricBox">
            <h4>Events Server</h4>
            <h1 id="event_server" class="centerText">OK</h1>
          </div>
        </div>
        <div id="event_server_div_error" class="col-md-3 hidden">
          <div class="red1Box metricBox">
            <h4>Events Server</h4>
            <h1 id="event_server" class="centerText">ERROR</h1>
          </div>
        </div>
        <div id="auth_server_div_ok" class="col-md-3">
          <div class="green1Box metricBox">
            <h4>Auth</h4>
            <h1 id="auth" class="centerText">OK</h1>
          </div>
        </div>
        <div id="auth_server_div_error" class="col-md-3 hidden">
          <div class="red1Box metricBox">
            <h4>Auth</h4>
            <h1 id="auth" class="centerText">ERROR</h1>
          </div>
        </div>
        <div class="col-md-3">
          <div class="green1Box metricBox">
            <h4>Sync</h4>
            <h1 id="sync" class="centerText">OK</h1>
          </div>
        </div>
      </div>
    {{end}}
  
    <div class="row">
      <div class="col-md-12">
        <div class="bg-primary metricBox">
          <h4>Last Errors Log</h4>
          <table id="logged_messages" class="table table-condensed table-hover">
            <tbody class="text-danger">
            {{range .LoggedMessages}}<tr><td>{{.}}</td></tr>{{end}}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </div>
  
  {{if .ProxyMode}}
    <!-- SDK STATS -->
    <div role="tabpanel" class="tab-pane" id="sdk-stats">
  
      <div class="row">
        <div class="col-md-6">
          <div class="greenBox metricBox">
            <h4>Successful Requests</h4>
            <h1 id="request_ok_formatted" class="centerText">{{.RequestOkFormatted}}</h1>
          </div>
        </div>
        <div class="col-md-6">
          <div class="redBox metricBox">
            <h4>Error Requests</h4>
            <h1 id="request_error_formatted" class="centerText">{{.RequestErrorFormatted}}</h1>
          </div>
        </div>
      </div>
  
      <div class="row">
        <div class="col-md-8">
          <div class="bg-primary metricBox">
            <h4>Latencies group <small>(microseconds)</small></h4>
            <canvas id="LatencyBucket"></canvas>
          </div>
        </div>
        <div class="col-md-4">
          <div class="bg-primary metricBox">
            <h4>Requests: Ok vs Error</h4>
            <canvas id="RequestStatus"></canvas>
          </div>
        </div>
      </div>
  
    </div>
  {{end}}
{{end}}
`

const queueManager = `
{{define "QueueManager"}}
  <div role="tabpanel" class="tab-pane" id="queue-manager">
  
    <div class="row">
      <div class="col-md-6">
        <div class="gray1Box metricBox">
          <h4>Impressions Queue Size</h4>
          <h1 id="impressions_queue_value_section" class="centerText">{{.ImpressionsQueueSize}}</h1>
        </div>
      </div>
      <div class="col-md-6">
        <div class="gray1Box metricBox">
          <h4>Events Queue Size</h4>
          <h1 id="events_queue_value_section" class="centerText">{{.EventsQueueSize}}</h1>
        </div>
      </div>
    </div>
  
    <div class="row">
      <div class="col-md-6">
        <div class="gray1Box metricBox">
          <h4>Impressions Lambda</h4>
          <h1 id="impressions_lambda_section" class="centerText">{{.ImpressionsLambda}}</h1>
        </div>
      </div>
      <div class="col-md-6">
        <div class="gray1Box metricBox">
          <h4>Events lambda</h4>
          <h1 id="events_lambda_section" class="centerText">{{.EventsLambda}}</h1>
        </div>
      </div>
    </div>
  
    <div class="row">
      <div class="col-md-2" style="text-align: center;">
        <button type="button" class="btn btn-danger btn-lg btn-block drop" onclick="javascript:dropImpressions();"
          style="padding-top: 4px; padding-bottom: 4px"
          data-toggle="tooltip" data-placement="top"
          title="This action will remove all the impressions from the Synchronizer">
          <span class="btn-label"><i class="glyphicon glyphicon-trash"></i></span>
          Drop Impressions
        </button>
      </div>
      <div class="col-md-4" style="text-align: center;  float: left">
        <div class="input-group input-group-lg">
          <input type="text" class="form-control" placeholder="Size" aria-label="Size" aria-describedby="basic-addon2"
            id="impressionsSize" default="">
          <span class="input-group-lg input-group-btn">
            <button class="btn btn-success btn-lg flush" type="button" onClick="javascript:flushImpressions();"
              data-toggle="tooltip" data-placement="top"
              title="This action will flush all the impressions from the Synchronizer">
              <span>
                <i class="glyphicon glyphicon-share-alt"></i>
              </span>Flush Impressions
            </button>
          </span>
        </div>
      </div>
  
      <div class="col-md-2" style="text-align: center">
        <button type="button" class="btn btn-danger btn-lg btn-block drop" onclick="javascript:dropEvents();"
          style="padding-top: 4px; padding-bottom: 4px"
          data-placement="top"
          data-toggle="tooltip"
          title="This action will remove all the events from the Synchronizer">
          <span class="btn-label">
            <i class="glyphicon glyphicon-trash"></i>
          </span>Drop Events
        </button>
      </div>
      <div class="col-md-4" style="text-align: center;  float: left">
        <div class="input-group input-group-lg">
          <input type="text" class="form-control" placeholder="Size" aria-label="Size" aria-describedby="basic-addon2"
            id="eventsSize" default="">
          <span class="input-group-lg input-group-btn">
            <button class="btn btn-success btn-lg flush" type="button" onClick="javascript:flushEvents();"
              data-toggle="tooltip" data-placement="top"
              title="This action will flush all the events from the Synchronizer">
              <span>
                <i class="glyphicon glyphicon-share-alt"></i>
              </span>Flush Events
            </button>
          </span>
        </div>
      </div>
    </div>
    </br>
    </br>
    </br>
    <div class="alert alert-info" role="alert">
      <h3 class="alert-heading">Lambda Eviction Calculation</h3>
      <p>Lambda calculation measures the capacity of the Synchronizer to process and fine tuning it if the default settings are not sufficient.</p>
      <hr>
      <ul>
        <li>
          <p class="mb-0">If <b>ℷ >= 1 (lambda)</b>: the current configuration is processing Events or Impressions without keeping elements in the stack. In other words, eviction rate >= generation rate. Split Synchronizer is able flush data as it arrives to the system from the SDKs.</p>
        </li>
        <li>
          <p class="mb-0">If <b>ℷ < 1 (lambda)</b>: the current configuration may not be enough to process all the data coming in, and over time it may produce an always-increasing memory footprint. Recommendation: increase the number of threads or reduce the frequency for evicting elements. We recommend increasing the number of threads if they are still using the default value of 1, and to not exceed the number of cores. On the other hand, when reducing the frequency of element eviction (flush operation), decrease the value in a conservative manner by increments of ten or twenty percent each time.</p>
        </li>
      </ul>
      <p>For further information you can visit <a href="https://help.split.io/hc/en-us/articles/360018343391-Split-Synchronizer-Runbook" class="alert-link">Split Synchronizer Runbook</a>.</p>
    </div>
  </div>
{{end}}
`

const upstreamStats = `
{{define "UpstreamStats"}}
  <div role="tabpanel" class="tab-pane" id="backend-stats"">
    <div class="row">
      <div class="col-md-6">
        <div class="greenBox metricBox">
          <h4>Successful Requests</h4>
          <h1 id="backend_request_ok_formatted" class="centerText">{{.BackendRequestOkFormatted}}</h1>
        </div>
      </div>
      <div class="col-md-6">
        <div class="redBox metricBox">
          <h4>Error Requests</h4>
          <h1 id="backend_request_error_formatted" class="centerText">{{.BackendRequestErrorFormatted}}</h1>
        </div>
      </div>
    </div>
    
    <div class="row">
      <div class="col-md-8">
        <div class="bg-primary metricBox">
          <h4>Latencies group <small>(microseconds)</small></h4>
          <canvas id="LatencyBucketBackend"></canvas>
        </div>
      </div>
      <div class="col-md-4">
        <div class="bg-primary metricBox">
          <h4>Requests: Ok vs Error</h4>
          <canvas id="RequestStatusBackend"></canvas>
        </div>
      </div>
    </div>
  </div>
{{end}}
`

const logo = `
{{define "SplitLogo"}}
  <svg class="icon icon-logo" viewBox="0 0 278 117" width="100" height="50" version="1.1" xmlns="http://www.w3.org/2000/svg">
    <title>reversed-logo</title>
    <g id="Symbols" stroke="none" stroke-width="1" fill="none" fill-rule="evenodd">
      <g id="split-logo-copy" transform="translate(-12.453125, -0.484375)">
        <g id="Best-Practices">
          <g id="Split-Best-Practices-Copy-4">
            <g id="logo">
              <g id="Group">
                <g id="Quick-Preso">
                  <g id="a-copy-2">
                    <g id="Group">
                      <g id="Logo--Copy-5" transform="translate(126.590802, 18.022484)" fill="#FFFFFF">
                        <path d="M20.3175225,65.414235 C32.8034446,65.414235 39.622679,59.2124082 39.622679,51.1023266 C39.622679,32.7830841 11.577377,38.8894983 11.577377,30.6840042 C11.577377,27.4399716 14.9389714,25.0546536 20.221477,25.0546536 C25.9842102,25.0546536 31.1706701,27.4399716 33.9559913,30.5885915 L38.1819957,23.52805 C33.9559913,19.8069538 28.0011668,17.0399849 20.1254314,17.0399849 C8.40787368,17.0399849 1.87677596,23.52805 1.87677596,31.0656551 C1.87677596,48.7170086 29.922078,42.2289435 29.922078,51.2931524 C29.922078,54.9188358 26.8486202,57.4949792 20.7977503,57.4949792 C14.7468803,57.4949792 8.31182812,54.3463593 5.04627926,51.0069141 L0.43609264,58.258281 C5.23837037,62.9335041 12.441787,65.414235 20.3175225,65.414235 L20.3175225,65.414235 L20.3175225,65.414235 L20.3175225,65.414235 Z M72.2908625,65.414235 C84.2965566,65.414235 92.844611,56.3500266 92.844611,41.1794036 C92.844611,26.0087808 84.2965566,17.0399849 72.2908625,17.0399849 C66.2399926,17.0399849 60.8614415,19.9023666 57.4038014,24.4821772 L57.4038014,18.1849376 L47.3190184,18.1849376 L47.3190184,81.8252232 L57.4038014,81.8252232 L57.4038014,57.8766299 C61.1495783,62.8380912 66.4320838,65.414235 72.2908625,65.414235 L72.2908625,65.414235 L72.2908625,65.414235 L72.2908625,65.414235 Z M70.5558468,56.540852 C65.8496148,56.540852 60.8552457,53.6784704 58.6461983,50.3390247 L58.6461983,32.115195 C60.8552457,28.680337 65.8496148,25.913368 70.5558468,25.913368 C78.5276276,25.913368 83.6180424,32.3060205 83.6180424,41.1794036 C83.6180424,50.1481994 78.5276276,56.540852 70.5558468,56.540852 L70.5558468,56.540852 L70.5558468,56.540852 L70.5558468,56.540852 Z M108.570048,64.2692825 L108.570048,0.628996721 L98.4852642,0.628996721 L98.4852642,64.2692825 L108.570048,64.2692825 L108.570048,64.2692825 L108.570048,64.2692825 Z M124.763321,12.555587 C128.220961,12.555587 131.006282,9.78861808 131.006282,6.35376003 C131.006282,2.91890205 128.220961,0.151933109 124.763321,0.151933109 C121.401726,0.151933109 118.52036,2.91890205 118.52036,6.35376003 C118.52036,9.78861808 121.401726,12.555587 124.763321,12.555587 L124.763321,12.555587 L124.763321,12.555587 L124.763321,12.555587 Z M129.853735,64.2692825 L129.853735,18.1849376 L119.768952,18.1849376 L119.768952,64.2692825 L129.853735,64.2692825 L129.853735,64.2692825 L129.853735,64.2692825 Z M163.866557,62.4564409 L161.465418,54.9188358 C160.697053,55.7775501 158.872187,56.540852 156.951276,56.540852 C154.06991,56.540852 152.533181,54.2509464 152.533181,51.1023266 L152.533181,26.962908 L161.945646,26.962908 L161.945646,18.1849376 L152.533181,18.1849376 L152.533181,5.59045827 L142.448397,5.59045827 L142.448397,18.1849376 L134.764753,18.1849376 L134.764753,26.962908 L142.448397,26.962908 L142.448397,53.5830575 C142.448397,61.2160751 146.578356,65.414235 154.358046,65.414235 C159.064279,65.414235 162.041691,64.1738696 163.866557,62.4564409 L163.866557,62.4564409 L163.866557,62.4564409 Z" id="split"></path>
                      </g>
                      <g id="New-Mark-Copy-2" transform="translate(60.616215, 59.555514) rotate(-30.000000) translate(-60.616215, -59.555514) translate(15.488942, 17.082786)">
                        <polygon id="Combined-Shape" fill="#67C7FF" points="40.6414493 83.2525092 63.1555675 83.2524261 74.1304124 83.2533241 85.1024071 64.2462616 59.0205466 19.0711541 37.0797234 19.0801135 63.1555675 64.2468686 10.9918465 64.2468686 0.0291940249 83.2526584 40.6414493 83.2525092 29.6688748 64.2474524 51.6148707 64.2474524 40.6418728 83.2532422"></polygon>
                        <polygon id="Combined-Shape" fill="#1F8CEB" points="59.0205393 19.079957 77.7022797 19.079957 88.6649321 0.0741670952 25.5385586 0.0743990989 14.5637142 0.0735008917 3.59171904 19.0805636 29.6735796 64.2556715 51.6144026 64.2467117 25.5385586 19.079957 37.0784192 19.079957 48.0494791 0.0775237434"></polygon>
                        <polygon id="Polygon-Copy-3" fill="#1B73C0" transform="translate(40.641873, 73.750349) scale(1, -1) translate(-40.641873, -73.750349) " points="40.6418728 64.2474541 51.6148707 83.2532439 29.6688748 83.2532439"></polygon>
                        <polygon id="Polygon-Copy-3" fill="#1B73C0" points="48.0494791 0.0775237434 59.0224769 19.0833137 37.0764812 19.0833137"></polygon>
                      </g>
                    </g>
                  </g>
        	      </g>
              </g>
            </g>
          </g>
        </g>
      </g>
    </g>
  </svg>
{{end}}
`

const dataInspector = `
{{define "DataInspector"}}
  <div role="tabpanel" class="tab-pane" id="data-inspector">
    <div class="row">
      <div class="col-md-12">
        <ul class="nav nav-pills nav-justified"  role="tablist">
          <li role="presentation" class="active">
            <a href="#splits-data" aria-controls="profile" role="tab" data-toggle="tab">
              <span class="glyphicon" style="vertical-align:bottom" aria-hidden="true">
  	      <svg class="icon icon-split-menu nav-title__icon" width="24" height="24" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
  	        <title>Icon/Segment-Dynamic</title>
  	        <g fill="none" fill-rule="evenodd">
  	          <path d="M0 0h24v24H0z"></path>
  	    	  <g stroke="#FFF" stroke-linecap="round" stroke-linejoin="round">
  	    	    <path d="M6 16.5l-2-2 2-2M12.5 22.5v-20M18 8.5l2 2-2 2M10.5 4l2-2 2 2"></path>
  	    	    <path d="M12.5 21v-4.497c0-1.106-.887-2.003-1.998-2.003H4.5M12.5 18v-5.49c0-1.11.887-2.01 2.006-2.01H19.5"></path>
  	    	  </g>
  	        </g>
  	      </svg>
  	    </span>
  	    &nbsp;Splits
  	  </a>
          </li>
          <li role="presentation" class="">
            <a href="#segments-data" aria-controls="profile" role="tab" data-toggle="tab">
              <span class="glyphicon" style="vertical-align:bottom" aria-hidden="true">
  	      <svg class="icon icon-segment-menu nav-title__icon" width="24" height="24" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
  	        <title>Icon/Segment-Static</title>
  		<g fill="none" fill-rule="evenodd">
  		  <path d="M0 0h24v24H0z"></path>
  		  <path stroke="#FFF" stroke-linecap="round" stroke-linejoin="round" d="M2.5 2.5h19v19h-19z"></path>
  		  <g transform="translate(7 7)" fill="#FFF">
  		    <rect width="4" height="4" rx=".5"></rect>
  		    <rect x="6" width="4" height="4" rx=".5"></rect>
  		    <rect y="6" width="4" height="4" rx=".5"></rect>
  		    <rect x="6" y="6" width="4" height="4" rx=".5"></rect>
  		  </g>
  		</g>
  	      </svg>
  	    </span>
  	    &nbsp;Segments
  	  </a>
          </li>
        </ul>
        </div>
      </div>
  
      <div class="tab-content">
  
        <!-- SPLITS DATA -->
        <div role="tabpanel" class="tab-pane active" id="splits-data">
          <div class="row">
            <div class="col-md-12">
              <div class="bg-primary metricBox">
                <!-- <h4>Splits in proxy</h4> -->
                <div class="row">
                  <div class="col-md-4 col-md-offset-8">
                    <div class="input-group">
                      <input type="text" id="filterSplitNameInput" class="form-control" placeholder="Filter by Split name">
                      <span class="input-group-btn">
                        <button class="btn btn-default" type="button" onclick="javascript:filterSplits();">
  		        <span class="glyphicon glyphicon-filter" aria-hidden="true"></span>
  		      </button>
                        <button class="btn btn-default" type="button" onclick="javascript:resetFilterSplits();">
  		        <span class="glyphicon glyphicon-remove" aria-hidden="true"></span>
  		      </button>
                      </span>
                    </div>
                  </div>
                </div>
                <div class="row">
                  <div class="col-md-12">
                    <table id="split_rows" class="table table-condensed table-hover">
                      <thead>
                        <tr>
                          <th>Split</th>
                          <th>Status</th>
                          <th>Killed</th>
                          <th>Treatments</th>
                          <th>Last Modified</th>
                        </tr>
                      </thead>
                      <tbody>
                        {{.SplitRows}}
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
  
        <!-- SEGMENTS DATA -->
        <div role="tabpanel" class="tab-pane" id="segments-data">
          <div class="row">
            <div class="col-md-12">
              <div class="bg-primary metricBox">
                <!-- <h4>Segments in proxy</h4> -->
                <table id="segment_rows" class="table table-condensed table-hover">
                  <thead>
                    <tr>
                      {{if .ProxyMode}}
                        <th>&nbsp;</th>
                        <th>Segment</th>
                        <th>Total Keys</th>
                        <th>Removed Keys</th>
                        <th>Active Keys</th>
                        <th>Last Modified</th>
                      {{else}}
                        <th>&nbsp;</th>
                        <th>Segment</th>
                        <th>Active Keys</th>
                        <th>Last Modified</th>
                      {{end}}
                    </tr>
                  </thead>
                  <tbody>
                    {{.SegmentRows}}
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        </div>
      </div>
  </div>
{{end}}
`

// AssembleDashboardTemplate concatenates the pieces in correct order to build the whole dashboard layout template
func AssembleDashboardTemplate() (*template.Template, error) {
	return template.New("DashboardLayout").Parse("sarasa")
}
