package dashboard

const cards = `
{{define "Cards"}}
  <div role="tabpanel" class="tab-pane active" id="split-dashboard">
    <div class="row">
      <div class="col-md-2">
        <div class="gray1Box metricBox">
          <h4>Uptime</h4>
          <h1 id="uptime" class="centerText"></h1>
        </div>
      </div>
      <div class="col-md-4">
        <div class="gray1Box metricBox">
          <h4>Healthy Since</h4>
          <h1 id="healthy_since" class="centerText"></h1>
        </div>
      </div>
      <div class="col-md-3">
        <div class="redBox metricBox">
          <h4>Logged Errors</h4>
          <h1 id="logged_errors" class="centerText"></h1>
        </div>
      </div>
  
      {{if .ProxyMode}}   
        <div class="col-md-3">
          <div class="gray2Box metricBox">
            <h4>SDKs Total Hits</h4>
            <h1 id="sdks_total_requests" class="centerText"></h1>
          </div>
        </div>
      {{else}}
        <div class="col-md-3">
          <div class="gray2Box metricBox">
            <h4>Backend Total Hits</h4>
            <h1 id="backend_total_requests" class="centerText"></h1>
          </div>
        </div>
      {{end}}
      
    </div>
  
    <div class="row">
      {{if .ProxyMode}} 
        <div class="col-md-6">
          <div class="gray2Box metricBox">
            <h4>Cached Splits</h4>
            <h1 id="splits_number" class="centerText"></h1>
          </div>
        </div>
        <div class="col-md-6">
          <div class="gray2Box metricBox">
            <h4>Cached Segments</h4>
            <h1 id="segments_number" class="centerText"></h1>
          </div>
        </div>
      {{else}}
        <div class="col-md-2">
          <div class="gray1Box metricBox">
            <h4>Impressions Queue</h4>
            <h1 id="impressions_queue_value" class="centerText"></h1>
          </div>
        </div>
        <div class="col-md-2">
          <div class="gray1Box metricBox">
            <h4>Events Queue</h4>
            <h1 id="events_queue_value" class="centerText"></h1>
          </div>
        </div>
        <div class="col-md-4">
          <div class="gray2Box metricBox">
            <h4>Cached Splits</h4>
            <h1 id="splits_number" class="centerText"></h1>
          </div>
        </div>
        <div class="col-md-4">
          <div class="gray2Box metricBox">
            <h4>Cached Segments</h4>
            <h1 id="segments_number" class="centerText"></h1>
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
            <h1 id="impressions_lambda" class="centerText"></h1>
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
            <h1 id="events_lambda" class="centerText"></h1>
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
        <div id="event_server_div_ok" class="col-md-1">
          <div class="green1Box metricBox">
            <h4>Events</h4>
            <h1 id="event_server" class="centerText">OK</h1>
          </div>
        </div>
        <div id="streaming_div_error" class="col-md-1 hidden">
          <div class="red1Box metricBox">
            <h4>Events</h4>
            <h1 id="streamingr" class="centerText">ERROR</h1>
          </div>
        </div>
        <div id="streaming_div_ok" class="col-md-1">
          <div class="green1Box metricBox">
            <h4>Streaming</h4>
            <h1 id="streaming" class="centerText">OK</h1>
          </div>
      </div>
      <div id="streaming_div_error" class="col-md-1 hidden">
        <div class="red1Box metricBox">
          <h4>Streaming</h4>
          <h1 id="streaming" class="centerText">ERROR</h1>
        </div>
      </div>
        <div id="auth_server_div_ok" class="col-md-1">
          <div class="green1Box metricBox">
            <h4>Auth</h4>
            <h1 id="auth" class="centerText">OK</h1>
          </div>
        </div>
        <div id="auth_server_div_error" class="col-md-1 hidden">
          <div class="red1Box metricBox">
            <h4>Auth</h4>
            <h1 id="auth" class="centerText">ERROR</h1>
          </div>
        </div>
        <div id="telemetry_server_div_ok" class="col-md-1">
        <div class="green1Box metricBox">
          <h4>Telemetry</h4>
          <h1 id="telemetry" class="centerText">OK</h1>
        </div>
      </div>
      <div id="telemetry_server_div_error" class="col-md-1 hidden">
        <div class="red1Box metricBox">
          <h4>Telemetry</h4>
          <h1 id="telemetry" class="centerText">ERROR</h1>
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
        <div id="telemetry_server_div_ok" class="col-md-2">
        <div class="green1Box metricBox">
          <h4>Telemetry</h4>
          <h1 id="telemetry" class="centerText">OK</h1>
        </div>
      </div>
      <div id="telemetry_server_div_error" class="col-md-2 hidden">
        <div class="red1Box metricBox">
          <h4>Telemetry</h4>
          <h1 id="telemetry" class="centerText">ERROR</h1>
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
        <div id="streaming_div_error" class="col-md-2 hidden">
        <div class="red1Box metricBox">
          <h4>Events</h4>
          <h1 id="streamingr" class="centerText">ERROR</h1>
        </div>
      </div>
      <div id="streaming_div_ok" class="col-md-2">
        <div class="green1Box metricBox">
          <h4>Streaming</h4>
          <h1 id="streaming" class="centerText">OK</h1>
        </div>
    </div>
        <div class="col-md-2">
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
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </div>
{{end}}
`

const sdkStats = `
{{define "SdkStats"}}
  <!-- SDK STATS -->
  <div role="tabpanel" class="tab-pane" id="sdk-stats">
    <div class="row">
      <div class="col-md-6">
        <div class="greenBox metricBox">
          <h4>Successful Requests</h4>
          <h1 id="requests_ok" class="centerText"></h1>
        </div>
      </div>
      <div class="col-md-6">
        <div class="redBox metricBox">
          <h4>Error Requests</h4>
          <h1 id="requests_error" class="centerText"></h1>
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
`

const upstreamStats = `
{{define "UpstreamStats"}}
  <div role="tabpanel" class="tab-pane" id="backend-stats">
    <div class="row">
      <div class="col-md-6">
        <div class="greenBox metricBox">
          <h4>Successful Requests</h4>
          <h1 id="backend_requests_ok" class="centerText"></h1>
        </div>
      </div>
      <div class="col-md-6">
        <div class="redBox metricBox">
          <h4>Error Requests</h4>
          <h1 id="backend_requests_error" class="centerText"></h1>
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
