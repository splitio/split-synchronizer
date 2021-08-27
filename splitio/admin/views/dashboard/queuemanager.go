package dashboard

const queueManager = `
{{define "QueueManager"}}
  <div role="tabpanel" class="tab-pane" id="queue-manager">
  
    <div class="row">
      <div class="col-md-6">
        <div class="gray1Box metricBox">
          <h4>Impressions Queue Size</h4>
          <h1 id="impressions_queue_value_section" class="centerText"></h1>
        </div>
      </div>
      <div class="col-md-6">
        <div class="gray1Box metricBox">
          <h4>Events Queue Size</h4>
          <h1 id="events_queue_value_section" class="centerText"></h1>
        </div>
      </div>
    </div>
  
    <div class="row">
      <div class="col-md-6">
        <div class="gray1Box metricBox">
          <h4>Impressions Lambda</h4>
          <h1 id="impressions_lambda_section" class="centerText"></h1>
        </div>
      </div>
      <div class="col-md-6">
        <div class="gray1Box metricBox">
          <h4>Events lambda</h4>
          <h1 id="events_lambda_section" class="centerText"></h1>
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
