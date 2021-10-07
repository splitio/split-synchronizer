package dashboard

const mainScript = `
{{define "MainScript"}}
  <script>
    function sendSignal(sigType){
      if(confirm("The proccess will be stopped, are you sure?")) {
        console.log("Shutting proccess down as",sigType)
        var processUrl
        if(sigType == 'graceful') {
          processUrl = "/admin/stop/graceful"
        } else {
          processUrl = "/admin/stop/force"
        }
  
        $.get(processUrl, function(data) {
          console.log("Response:", data);
        })
      }
    }
  
    function getKeys(segment) {
      if($("#showKeys-"+segment+" span").hasClass("glyphicon-menu-down")){
        $("#showKeys-"+segment+" span").removeClass("glyphicon-menu-down");
        $("#showKeys-"+segment+" span").addClass("glyphicon-menu-right");
        $('#segmentKeysDetailedList-'+segment).addClass("filterDisplayNone");
        return false;
      }
      $("tr.segmentKeysDetailedList").addClass("filterDisplayNone");
      $("a.showKeysLnk span").removeClass("glyphicon-menu-down");
      $("a.showKeysLnk span").addClass("glyphicon-menu-right");
      $("#showKeys-"+segment+" span").removeClass("glyphicon-menu-right");
      $("#showKeys-"+segment+" span").addClass("glyphicon-menu-down");
      $('#segmentKeysDetailedList-'+segment).removeClass("filterDisplayNone");
  
      $('.segmentKeysDetailedList-tbody').html("");
      $('#segmentKeysDetailedList-tbody-'+segment).html('<tr><td colspan="3"><p>Loading keys...</p></td></tr>');
      $.get("/admin/dashboard/segmentKeys/"+segment, function(data) {
	let html = '';
	html = data.reduce(function(block, item) {
	    const rows = [
		'<tr class="segmentKeyItem">',
		'<td><span class="' + (item.removed ? '"redbox" "' : '') + 'segmentKeyItemName">' + item.name + '</span></td>',
	    ];
	    {{if .ProxyMode}}
		rows.push('<td>' + item.removed + '</td>')
		rows.push('<td>' + item.cn + '</td>')
	    {{end}}
	    rows.push('</tr>');
	    return block + rows.join('');
	}, '');

        $('#segmentKeysDetailedList-tbody-'+segment).html(html);
      })
    }
  
    function filterSegmentKeys(segmentName){
      $("tr.segmentKeyItem").removeClass("filterDisplayNone");
      var filter = $("#filterSegmentKeyInput-"+segmentName).val();
  
      $("tr.segmentKeyItem").each(function() {
        $this = $(this);
        var segmentName = $this.find("span.segmentKeyItemName").html();
        if (segmentName.indexOf(filter.trim()) == -1) {
          $this.addClass("filterDisplayNone");
        }
      });
    }
  
    function resetFilterSegmentKeys(){
      $("tr.segmentKeyItem").removeClass("filterDisplayNone");
      $(".filterSegmentKeyInput").val("");
    }
  
    function resetFilterSplits(){
      $("tr.splitItem").removeClass("filterDisplayNone");
      $("#filterSplitNameInput").val("");
    }
  
    function filterSplits(){
      $("tr.splitItem").removeClass("filterDisplayNone");
      var filter = $("#filterSplitNameInput").val();
      $("tr.splitItem").each(function() {
        $this = $(this);
        var splitName = $this.find("span.splitItemName").html();
        if (splitName.indexOf(filter.trim()) == -1) {
          $this.addClass("filterDisplayNone");
        }
      });
    }
  
    function dropImpressions(){
      if(confirm("This action will drop all the impressions, are you sure?")) {
        console.log("Dropping impressions")
  
        $.post("{{.DataControllerPath}}/impressions/drop", function(data) {
          console.log("Response:", data);
        })
      }
    }
  
    function flushImpressions(){
      if(confirm("This action will flush impressions to the server, are you sure?")) {
        console.log("Flushing impressions")
	const baseUrl = "{{.DataControllerPath}}/impressions/flush"
        const size = document.getElementById("impressionsSize").value;
        const finalUrl = size === "" ? baseUrl : (baseUrl + "?size=" + size);
  
        $.post(finalUrl, function(data) {
          console.log("Response:", data);
        })
      }
    }
  
    function dropEvents(){
      if(confirm("This action will drop all the events, are you sure?")) {
        console.log("Dropping events")
  
        $.post("{{.DataControllerPath}}/events/drop", function(data) {
          console.log("Response:", data);
        })
      }
    }
  
    function flushEvents(){
      if(confirm("This action will flush events to the server, are you sure?")) {
        console.log("Flushing events")
	const baseUrl = "/admin/events/flush"
  
        const size = document.getElementById("eventsSize").value;
        const api = size === "" ? baseUrl : (baseUrl + "?size=" + size);
  
        $.post(api, function(data) {
          console.log("Response:", data);
        })
      }
    }
  
    $(function () {
      $('[data-toggle="tooltip"]').tooltip()
    })
  
  </script>
  
  <script>
  // SDKs charts
  function renderSDKChart(latenciesGroupData) {
    const ctxL = document.getElementById("LatencyBucket").getContext('2d');
  
    let datasets = []
    if (typeof latenciesGroupData === 'string' || latenciesGroupData instanceof String) {
      if (currentData["latenciesGroupData"] && currentData["latenciesGroupData"] === latenciesGroupData) {
        return
      }
      currentData["latenciesGroupData"] = latenciesGroupData;
      const str = JSON.stringify(eval("(" + latenciesGroupData + ")"));
      datasets = JSON.parse(str);
    } else {
      datasets = latenciesGroupData
    }
  
    const myChart = new Chart(ctxL, {
      type: 'horizontalBar',
      data: {
        labels: ["1000", "1000-1500", "1500-2250", "2250-3375", "3375-5063", "5063-7594", "7594-11391", "11391-17086", "17086-25629", "25629-38443", "38443-57665", "57665-86498", "86498-129746", "129746-194620", "194620-291929", "291929-437894", "437894-656841", "656841-985261", "985261-1477892", "1477892-2216838", "2216838-3325257", "3325257-4987885", "4987885-7481828"],
        datasets: datasets
      },
      options: {
        scales: {
          yAxes: [{
            ticks: {
              beginAtZero:true
            }
          }]
        }
      }
    });
  }
  
  //Error & Success - PolarArea
  function renderErrorAndSuccessSDK(requestOk, requestError) {
    const sdkRequestOk = requestOk && Number(requestOk) > 0 ? requestOk : 0;
    const sdkRequestError = requestError && Number(requestError) > 0 ? requestError : 0;
    if (currentData["sdkRequestOk"] !== undefined && currentData["sdkRequestOk"] === sdkRequestOk && currentData["sdkRequestError"] !== undefined && currentData["sdkRequestError"] === sdkRequestError) {
      return
    }
    currentData["sdkRequestOk"] = sdkRequestOk;
    currentData["sdkRequestError"] = sdkRequestError;
    const ctxR = document.getElementById("RequestStatus").getContext('2d');
    const myChart = new Chart(ctxR, {
      type: 'pie',
      data: {
        labels: ["Ok", "Error"],
        datasets: [{
          data: [sdkRequestOk, sdkRequestError],
          backgroundColor: [
            'rgba(75, 192, 192, 0.2)',
            //'#c8e5bc',
            'rgba(255, 99, 132, 0.2)'
          ],
          borderColor: [
            'rgba(75, 192, 192, 1)',
            //'#b2dba1',
            'rgba(255,99,132,1)'
          ],
          borderWidth: 1
        }],
      }
    });
  }
  
  const currentData = {};
  
  // BACKEND STATS
  function renderBackendStatsChart(latenciesGroupDataBackend) {
    const ctxLB = document.getElementById("LatencyBucketBackend").getContext('2d');
  
    let datasets = []
    if (typeof latenciesGroupDataBackend === 'string' || latenciesGroupDataBackend instanceof String) {
      if (currentData["latenciesGroupDataBackend"] && currentData["latenciesGroupDataBackend"] === latenciesGroupDataBackend) {
        return
      }
      currentData["latenciesGroupDataBackend"] = latenciesGroupDataBackend;
      const str = JSON.stringify(eval("(" + latenciesGroupDataBackend + ")"));
      datasets = JSON.parse(str);
    } else {
      datasets = latenciesGroupDataBackend
    }
  
    const myChartB = new Chart(ctxLB, {
      type: 'horizontalBar',
      data: {
        labels: ["1000", "1000-1500", "1500-2250", "2250-3375", "3375-5063", "5063-7594", "7594-11391", "11391-17086", "17086-25629", "25629-38443", "38443-57665", "57665-86498", "86498-129746", "129746-194620", "194620-291929", "291929-437894", "437894-656841", "656841-985261", "985261-1477892", "1477892-2216838", "2216838-3325257", "3325257-4987885", "4987885-7481828"],
        datasets: datasets
      },
      options: {
        scales: {
          yAxes: [{
            ticks: {
              beginAtZero:true
            }
          }]
        }
      }
    });
  }
  
  //Error & Success - PolarArea
  function renderErrorAndSuccess(backendRequestOk, backendRequestError) {
    const bRequestOK = backendRequestOk && Number(backendRequestOk) > 0 ? backendRequestOk : 0;
    const bRequestError = backendRequestError && Number(backendRequestError) > 0 ? backendRequestError : 0;
    if (currentData["bRequestOK"] !== undefined && currentData["bRequestOK"] === bRequestOK && currentData["bRequestError"] !== undefined && currentData["bRequestError"] === bRequestError) {
      return
    }
    currentData["bRequestOK"] = bRequestOK;
    currentData["bRequestError"] = bRequestError;
    const ctxRB = document.getElementById("RequestStatusBackend").getContext('2d');
    const myChart = new Chart(ctxRB, {
      type: 'pie',
      data: {
        labels: ["Ok", "Error"],
        datasets: [{
          data: [bRequestOK, bRequestError],
            backgroundColor: [
              'rgba(75, 192, 192, 0.2)',
              //'#c8e5bc',
              'rgba(255, 99, 132, 0.2)'
            ],
            borderColor: [
              'rgba(75, 192, 192, 1)',
              //'#b2dba1',
              'rgba(255,99,132,1)'
            ],
            borderWidth: 1
        }],
      }
    });
  }
  
  function handleHealthcheck(response) {
    $('#healthy_since').text(response.healthySince.time);
    $('#uptime').text(response.uptime);
  
    if (response.sdk.healthy) {
      $('#sdk_server_div_error').addClass('hidden')
      $('#sdk_server_div_ok').removeClass('hidden')
    } else {
      $('#sdk_server_div_ok').addClass('hidden')
      $('#sdk_server_div_error').removeClass('hidden')
    }
  
    if (response.events.healthy) {
      $('#event_server_div_error').addClass('hidden')
      $('#event_server_div_ok').removeClass('hidden')
    } else {
      $('#event_server_div_ok').addClass('hidden')
      $('#event_server_div_error').removeClass('hidden')
    }
  
    if (response.auth.healthy) {
      $('#auth_server_div_error').addClass('hidden')
      $('#auth_server_div_ok').removeClass('hidden')
    } else {
      $('#auth_server_div_ok').addClass('hidden')
      $('#auth_server_div_error').removeClass('hidden')
    }
  
    if (response.sync) {
      if (response.storage.healthy) {
        $('#storage_div_error').addClass('hidden')
        $('#storage_div_ok').removeClass('hidden')
      } else {
        $('#storage_div_ok').addClass('hidden')
        $('#storage_div_error').removeClass('hidden')
      }
    }
  
    setTimeout(function() {
      refreshHealthcheck();
    }, {{.RefreshTime}});
  };
  
  function refreshHealthcheck() {
    $.ajax({
      url: "healthcheck",
      cache: false,
      dataType: "json",
      success: function(response) {
        handleHealthcheck(response)
      },
      error: function(response) {
        if (response.status == 500 && response.responseJSON && response.responseJSON.sdk) {
          handleHealthcheck(response.responseJSON)
        }
      }
    });
  };

  function formatTreatments(split) {
    return split.treatments
      .map(t => (t == split.defaultTreatment) ?  ('<strong>' + t + '</strong>') : t)
      .join('\n');
  };

  function formatSplit(split) {
    return (
      '<tr class="splitItem">' +
      '  <td><span class="splitItemName">' + split.name + '</span></td>' +
         (!split.active ? '<td class="danger">ARCHIVED</td>' : '<td class="">ACTIVE</td>') +
         (split.killed ? '<td class="danger">true</td>' : '<td class="">false</td>') +
      '  <td>' + formatTreatments(split) + '</td>' +
      '  <td>' + split.cn + '</td>' +
      '</tr>\n');
  };

  function updateSplits(splits) {
    const formatted = splits.map(formatSplit).join('\n');
    $('#split_rows tbody').empty();
    $('#split_rows tbody').append(formatted);
  };

  function formatSegment(segment) {
    return '<tr>' + 
          '<td><a id="showKeys-' + segment.name + '" href="#" onclick="javascript:getKeys(\'' + segment.name + '\');return false;" class="showKeysLnk btn-xs">' +
          '  <span class="glyphicon glyphicon-menu-right" aria-hidden="true"></span>' +
	  '</a></td>' +
          '<td>' + segment.name + '</td>' +
          {{if $.ProxyMode}}
            '<td>' + segment.totalKeys + '</td>' +
            '<td>' + segment.removedKeys + '</td>' +
          {{end}}
          '<td>' + segment.activeKeys + '</td>' +
          '<td>' + segment.cn + '</td>' +
        '</tr>' +
        '<tr id="segmentKeysDetailedList-' + segment.name + '" class="segmentKeysDetailedList filterDisplayNone">' +
          '<td colspan="6">' +
            '<div class="row">' +
              '<div class="col-md-4">' +
                '<div class="input-group">' +
                  '<input type="text" id="filterSegmentKeyInput-' + segment.name + '" class="form-control input-sm filterSegmentKeyInput" placeholder="Filter by Key">' +
                  '<span class="input-group-btn">' +
          	  '<button class="btn btn-default btn-sm" type="button" onclick="javascript:filterSegmentKeys(\'' + segment.name + '\');">' +
	  	  '  <span class="glyphicon glyphicon-filter" aria-hidden="true"></span>' +
	  	  '</button>' +
                    '<button class="btn btn-default btn-sm" type="button" onclick="javascript:resetFilterSegmentKeys();">' +
	  	    '<span class="glyphicon glyphicon-remove" aria-hidden="true"></span>' +
	  	  '</button>' +
          	'</span>' +
                '</div>' +
              '</div>' +
            '</div>' +
            '<table class="table table-condensed table-hover">' +
              '<thead>' +
                '<tr>' +
                  '<th>Key</th>' +
                  {{if $.ProxyMode}}
                    '<th>Removed</th>' +
                    '<th>Last Modified</th>' +
	          {{end}}
                '</tr>' +
              '</thead>' +
              '<tbody id="segmentKeysDetailedList-tbody-' + segment.name + '" class="segmentKeysDetailedList-tbody"></tbody>' +
            '</table>' +
          '</td>' +
        '</tr>';
  }

  function updateSegments(segments) {
    const formatted = segments.map(formatSegment).join('\n');
    $('#segment_rows tbody').empty();
    $('#segment_rows tbody').append(formatted);
  };

  function updateMetricCards(stats) {
    $('#impressions_queue_value_section').html(stats.impressionsQueueSize);
    $('#impressions_lambda_section').html(stats.impressionsLambda);
    $('#events_queue_value_section').html(stats.eventsQueueSize);
    $('#events_lambda_section').html(stats.eventsLambda);
    $('#uptime').html(stats.uptime);
    $('#logged_errors').html(stats.loggedErrors);
    $('#sdks_total_requests').html(stats.sdksTotalRequests);
    $('#backend_total_requests').html(stats.backendTotalRequests);
    $('#splits_number').html(stats.splits.length);
    $('#segments_number').html(stats.segments.length);
    $('#impressions_queue_value').html(stats.impressionsQueueSize);
    $('#events_queue_value').html(stats.eventsQueueSize);
    $('#splits_number').html();
    $('#segments_number').html();
    $('#impressions_lambda').html(stats.impressionsLambda);
    $('#events_lambda').html(stats.eventsLambda);
    $('#requests_ok').html(stats.requestsOk);
    $('#requests_error').html(stats.requestsErrored);
    $('#backend_requests_ok').html(stats.backendRequestsOk);
    $('#backend_requests_error').html(stats.backendRequestsErrored);
  };

  function updateHealthCards(health) {
      $('#healthy_since').html(health.healthySince);
      if (health.sdkServerStatus) {
        $('#sdk_server_div_error').addClass('hidden')
      } else {
        $('#sdk_server_div_ok').addClass('hidden')
        $('#sdk_server_div_error').removeClass('hidden')
      }
  
      if (health.eventsServerStatus) {
        $('#event_server_div_error').addClass('hidden')
      } else {
        $('#event_server_div_ok').addClass('hidden')
        $('#event_server_div_error').removeClass('hidden')
      }
  
      if (health.authServerStatus) {
        $('#auth_server_div_error').addClass('hidden')
      } else {
        $('#auth_server_div_ok').addClass('hidden')
        $('#auth_server_div_error').removeClass('hidden')
      }
  
      {{if .ProxyMode}}
        const isStorageStatus = health.storage;
        if (isStorageStatus) {
          $('#storage_div_error').addClass('hidden')
        } else {
          $('#storage_div_ok').addClass('hidden')
          $('#storage_div_error').removeClass('hidden')
        }
      {{end}}
  };

  function updateLogEntries(messages) {
    $('#logged_messages').empty()
    $('#logged_messages').append(
      messages
        .map(m => '<tbody class="text-danger"><tr><td>' + m + '</td></tr></tbody>')
        .join(''));
  }

  function processHealth(health) {
    updateHealthCards(health);
  }

  function processStats(stats) {
    updateMetricCards(stats)
    updateSplits(stats.splits);
    updateSegments(stats.segments);
    updateLogEntries(stats.loggedMessages);

    renderBackendStatsChart(stats.backendLatencies);
    {{if .ProxyMode}}
        renderSDKChart(stats.latencies);
    {{end}}
  };

  function refreshStats() {
    $.getJSON("/admin/dashboard/stats", processStats);
  };

  function refreshHealth() {
    $.getJSON("/admin/dashboard/health", processHealth);
  };

 
  $(document).on('click', function (e) {
    $('.popovers').each(function () {
        if (!$(this).is(e.target) && $(this).has(e.target).length === 0 && $('.popover').has(e.target).length === 0) {                
            (($(this).popover('hide').data('bs.popover')||{}).inState||{}).click = false
        }
    });
  });
  
  $(document).ready(function () {
    const initialData = {{serializeIncomingData .}};
    const popOverData = {
      container: 'body',
      content: '<div><ul><li>If <b>ℷ >= 1 (lambda)</b>: the current configuration is processing Events or Impressions without keeping elements in the stack. In other words,eviction rate >= generation rate. Split Synchronizer is able flush data as it arrives to the system from the SDKs.</li><li>If <b>ℷ < 1 (lambda)</b>: the current configuration may not be enough to process all the data coming in, and over time it may produce an always-increasing memory footprint. Recommendation: increase the number of threads or reduce the frequency for evicting elements. We recommend increasing the number of threads if they are still using the default value of 1, and to not exceed the number of cores. On the other hand, when reducing the frequency of element eviction (flush operation), decrease the value in a conservative manner by increments of ten or twenty percent each time.</li></ul><p>For further information you can visit <a href="https://help.split.io/hc/en-us/articles/360018343391-Split-Synchronizer-Runbook">Split Synchronizer Runbook</a>.</p></div>',
      html: true,
      placement: "auto",
    };
    $('[data-toggle="popover-impressions"]').popover(popOverData);
    $('[data-toggle="popover-events"]').popover(popOverData);
  
    processStats(initialData.stats);
    processHealth(initialData.health);

  
    setInterval(function() {
      refreshStats();
      refreshHealth();
    }, {{.RefreshTime}});
  });

  </script>
{{end}}
`