package dashboard

const dataInspector = `
{{define "DataInspector"}}
  <div role="tabpanel" class="tab-pane" id="data-inspector">
    <div class="row">
      <div class="col-md-12">
        <ul class="nav nav-pills nav-justified"  role="tablist">
          <li role="presentation" class="active">
            <a href="#feature-flags-data" aria-controls="profile" role="tab" data-toggle="tab">
              <span class="glyphicon" style="vertical-align:bottom" aria-hidden="true">
  	      <svg class="icon icon-split-menu nav-title__icon" width="24" height="24" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
  	        <title>Icon/Segment-Dynamic</title>
  	        <g fill="none" fill-rule="evenodd">
  	          <path d="M0 0h24v24H0z"></path>
  	    	  <g stroke="#101828" stroke-linecap="round" stroke-linejoin="round">
  	    	    <path d="M6 16.5l-2-2 2-2M12.5 22.5v-20M18 8.5l2 2-2 2M10.5 4l2-2 2 2"></path>
  	    	    <path d="M12.5 21v-4.497c0-1.106-.887-2.003-1.998-2.003H4.5M12.5 18v-5.49c0-1.11.887-2.01 2.006-2.01H19.5"></path>
  	    	  </g>
  	        </g>
  	      </svg>
  	    </span>
        &nbsp;Feature Flags
  	  </a>
          </li>
          <li role="presentation" class="">
            <a href="#segments-data" aria-controls="profile" role="tab" data-toggle="tab">
              <span class="glyphicon" style="vertical-align:bottom" aria-hidden="true">
  	      <svg class="icon icon-segment-menu nav-title__icon" width="24" height="24" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
  	        <title>Icon/Segment-Static</title>
  		<g fill="none" fill-rule="evenodd">
  		  <path d="M0 0h24v24H0z"></path>
  		  <path stroke="#101828" stroke-linecap="round" stroke-linejoin="round" d="M2.5 2.5h19v19h-19z"></path>
  		  <g transform="translate(7 7)" fill="#101828">
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

        <!-- FEATURE FLAGS DATA -->
        <div role="tabpanel" class="tab-pane active" id="feature-flags-data">
          <div class="row">
            <div class="col-md-12">
              <div class="bg-primary metricBox">
                <!-- <h4>Feature Flags in proxy</h4> -->
                <div class="row">
                  <div class="col-md-4 col-md-offset-8">
                    <div class="input-group">
                      <input type="text" id="filterFeatureFlagNameInput" class="form-control" placeholder="Filter by Feature Flag name">
                      <span class="input-group-btn">
                        <button class="btn btn-default" type="button" onclick="javascript:filterFeatureFlags();">
  		        <span class="glyphicon glyphicon-filter" aria-hidden="true"></span>
  		      </button>
                        <button class="btn btn-default" type="button" onclick="javascript:resetFilterFeatureFlags();">
  		        <span class="glyphicon glyphicon-remove" aria-hidden="true"></span>
  		      </button>
                      </span>
                    </div>
                  </div>
                </div>
                <div class="row">
                  <div class="col-md-12">
                    <table id="feature_flag_rows" class="table table-condensed table-hover">
                      <thead>
                        <tr>
                          <th>Feature Flag</th>
                          <th>Status</th>
                          <th>Killed</th>
                          <th>Treatments</th>
                          <th>Last Modified</th>
                        </tr>
                      </thead>
                      <tbody>
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
