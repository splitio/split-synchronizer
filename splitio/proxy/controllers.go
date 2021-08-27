package proxy

// const (
// 	split          = "sdk.splitChanges"
// 	segment        = "sdk.segmentChanges"
// 	mySegment      = "sdk.mySegments"
// 	impressions    = "sdk.impressions"
// 	events         = "sdk.events"
// 	metricTime     = "sdk.metrics.time"
// 	metricLatency  = "sdk.metrics.times"
// 	metricCounter  = "sdk.metrics.counter"
// 	metricCounters = "sdk.metrics.counters"
// 	metricGauge    = "sdk.metrics.gauge"
// 	localAPIOK     = "sdk.request.ok"
// 	localAPIError  = "sdk.request.error"
// )
//
// func validateAPIKey(keys []string, apiKey string) bool {
// 	for _, key := range keys {
// 		if apiKey == key {
// 			return true
// 		}
// 	}
//
// 	return false
// }

//-----------------------------------------------------------------------------
// SPLIT CHANGES
//-----------------------------------------------------------------------------
// func fetchSplitsFromDB(since int) ([]json.RawMessage, int64, error) {
//
// 	till := int64(since)
// 	splits := make([]json.RawMessage, 0)
//
// 	splitCollection := collections.NewSplitChangesCollection(boltdb.DBB)
// 	items, err := splitCollection.FetchAll()
// 	if err != nil {
// 		return splits, till, err
// 	}
//
// 	for _, split := range items {
// 		if split.ChangeNumber > int64(since) {
// 			if split.ChangeNumber > till {
// 				till = split.ChangeNumber
// 			}
// 			splits = append(splits, []byte(split.JSON))
// 		}
// 	}
//
// 	return splits, till, nil
// }
//
// func splitChanges(c *gin.Context) {
// 	c.Set(proxyStorage.EndpointKey, proxyStorage.SplitChangesEndpoint)
// 	log.Instance.Debug(fmt.Sprintf("Headers: %v", c.Request.Header))
// 	sinceParam := c.DefaultQuery("since", "-1")
// 	since, err := strconv.Atoi(sinceParam)
// 	if err != nil {
// 		since = -1
// 	}
// 	log.Instance.Debug(fmt.Sprintf("SDK Fetches Splits Since: %d", since))
//
// 	splits, till, errf := fetchSplitsFromDB(since)
// 	if errf != nil {
// 		switch errf {
// 		case boltdb.ErrorBucketNotFound:
// 			log.Instance.Warning("Maybe Splits are not yet synchronized")
// 		default:
// 			log.Instance.Error(errf)
// 		}
// 		interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.SplitChangesEndpoint, http.StatusInternalServerError)
// 		c.JSON(http.StatusInternalServerError, gin.H{"error": errf.Error()})
// 		return
// 	}
// 	interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.SplitChangesEndpoint, http.StatusOK)
// 	c.JSON(http.StatusOK, gin.H{"splits": splits, "since": since, "till": till})
// }

//-----------------------------------------------------------------------------
// SEGMENT CHANGES
//-----------------------------------------------------------------------------

// func fetchSegmentsFromDB(since int, segmentName string) ([]string, []string, int64, error) {
// 	added := make([]string, 0)
// 	removed := make([]string, 0)
// 	till := int64(since)
//
// 	segmentCollection := collections.NewSegmentChangesCollection(boltdb.DBB)
// 	item, err := segmentCollection.Fetch(segmentName)
// 	if err != nil {
// 		switch err {
// 		case boltdb.ErrorBucketNotFound:
// 			log.Instance.Warning("Bucket not found for segment [%s]\n", segmentName)
// 		default:
// 			log.Instance.Error(err)
// 		}
// 		return added, removed, till, err
// 	}
//
// 	if item == nil {
// 		return added, removed, till, err
// 	}
//
// 	for _, skey := range item.Keys {
// 		if skey.ChangeNumber > int64(since) {
// 			if skey.Removed {
// 				if since > 0 {
// 					removed = append(removed, skey.Name)
// 				}
// 			} else {
// 				added = append(added, skey.Name)
// 			}
//
// 			if since > 0 {
// 				if skey.ChangeNumber > till {
// 					till = skey.ChangeNumber
// 				}
// 			} else {
// 				if !skey.Removed && skey.ChangeNumber > till {
// 					till = skey.ChangeNumber
// 				}
// 			}
// 		}
// 	}
//
// 	return added, removed, till, nil
// }
//
// func segmentChanges(c *gin.Context) {
// 	c.Set(proxyStorage.EndpointKey, proxyStorage.SegmentChangesEndpoint)
// 	log.Instance.Debug(fmt.Sprintf("Headers: %v", c.Request.Header))
// 	sinceParam := c.DefaultQuery("since", "-1")
// 	since, err := strconv.Atoi(sinceParam)
// 	if err != nil {
// 		since = -1
// 	}
//
// 	segmentName := c.Param("name")
// 	log.Instance.Debug(fmt.Sprintf("SDK Fetches Segment: %s Since: %d", segmentName, since))
// 	added, removed, till, errf := fetchSegmentsFromDB(since, segmentName)
// 	if errf != nil {
// 		interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.SegmentChangesEndpoint, http.StatusNotFound)
// 		c.JSON(http.StatusNotFound, gin.H{"error": errf.Error()})
// 		return
// 	}
// 	c.JSON(http.StatusOK, gin.H{
// 		"name":    segmentName,
// 		"added":   added,
// 		"removed": removed,
// 		"since":   since,
// 		"till":    till,
// 	})
// 	interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.SegmentChangesEndpoint, http.StatusOK)
// }

//-----------------------------------------------------------------------------
// MY SEGMENTS
//-----------------------------------------------------------------------------
// func mySegments(c *gin.Context) {
// 	c.Set(proxyStorage.EndpointKey, proxyStorage.MySegmentsEndpoint)
// 	log.Instance.Debug(fmt.Sprintf("Headers: %v", c.Request.Header))
// 	key := c.Param("key")
// 	var mysegments = make([]dtos.MySegmentDTO, 0)
//
// 	segmentCollection := collections.NewSegmentChangesCollection(boltdb.DBB)
// 	segments, errs := segmentCollection.FetchAll()
// 	if errs != nil {
// 		log.Instance.Warning(errs)
// 		interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.MySegmentsEndpoint, http.StatusInternalServerError)
// 		c.JSON(http.StatusInternalServerError, gin.H{})
// 	} else {
// 		for _, segment := range segments {
// 			for _, skey := range segment.Keys {
// 				if !skey.Removed && skey.Name == key {
// 					mysegments = append(mysegments, dtos.MySegmentDTO{Name: segment.Name})
// 					break
// 				}
// 			}
// 		}
// 	}
//
// 	c.JSON(http.StatusOK, gin.H{"mySegments": mysegments})
// 	interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.MySegmentsEndpoint, http.StatusOK)
// }

//-----------------------------------------------------------------
//                 I M P R E S S I O N S
//-----------------------------------------------------------------
// func submitImpressions(
// 	impressionListenerEnabled bool,
// 	sdkVersion string,
// 	machineIP string,
// 	machineName string,
// 	impressionsMode string,
// 	data []byte,
// ) {
// 	if impressionListenerEnabled {
// 		var rawPayload []dtos.ImpressionsDTO
// 		err := json.Unmarshal(data, &rawPayload)
// 		if err == nil && rawPayload != nil && len(rawPayload) > 0 {
// 			impressionsListenerDTO := make([]common.ImpressionsListener, 0, len(rawPayload))
// 			for _, impressionsDTO := range rawPayload {
// 				impressionListenerDTO := make([]common.ImpressionListener, 0, len(impressionsDTO.KeyImpressions))
// 				for _, impression := range impressionsDTO.KeyImpressions {
// 					impressionListenerDTO = append(impressionListenerDTO, common.ImpressionListener{
// 						BucketingKey: impression.BucketingKey,
// 						ChangeNumber: impression.ChangeNumber,
// 						KeyName:      impression.KeyName,
// 						Label:        impression.Label,
// 						Pt:           impression.Pt,
// 						Time:         impression.Time,
// 						Treatment:    impression.Treatment,
// 					})
// 				}
// 				impressionsListenerDTO = append(impressionsListenerDTO, common.ImpressionsListener{
// 					TestName:       impressionsDTO.TestName,
// 					KeyImpressions: impressionListenerDTO,
// 				})
// 			}
//
// 			serializedImpression, err := json.Marshal(impressionsListenerDTO)
// 			if err == nil {
// 				_ = task.QueueImpressionsForListener(&task.ImpressionBulk{
// 					Data:        json.RawMessage(serializedImpression),
// 					SdkVersion:  sdkVersion,
// 					MachineIP:   machineIP,
// 					MachineName: machineName,
// 				})
// 			}
// 		}
// 	}
//
// 	controllers.AddImpressions(data, sdkVersion, machineIP, machineName, impressionsMode)
// }
//
// func postImpressionBulk(impressionListenerEnabled bool) gin.HandlerFunc {
// 	return func(c *gin.Context) {
// 		c.Set(proxyStorage.EndpointKey, proxyStorage.ImpressionsBulkEndpoint)
// 		sdkVersion := c.Request.Header.Get("SplitSDKVersion")
// 		machineIP := c.Request.Header.Get("SplitSDKMachineIP")
// 		machineName := c.Request.Header.Get("SplitSDKMachineName")
// 		impressionsMode := c.Request.Header.Get("SplitSDKImpressionsMode")
// 		data, err := ioutil.ReadAll(c.Request.Body)
// 		if err != nil {
// 			log.Instance.Error(err)
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.ImpressionsBulkEndpoint, http.StatusInternalServerError)
// 			c.JSON(http.StatusInternalServerError, nil)
// 			return
// 		}
// 		if impressionListenerEnabled {
// 			err = task.QueueImpressionsForListener(&task.ImpressionBulk{
// 				Data:        json.RawMessage(data),
// 				SdkVersion:  sdkVersion,
// 				MachineIP:   machineIP,
// 				MachineName: machineName,
// 			})
// 		}
//
// 		submitImpressions(impressionListenerEnabled, sdkVersion, machineIP, machineName, impressionsMode, data)
// 		c.JSON(http.StatusOK, nil)
// 		interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.ImpressionsBulkEndpoint, http.StatusOK)
// 	}
// }
//
// func postImpressionBeacon(keys []string, impressionListenerEnabled bool) gin.HandlerFunc {
// 	return func(c *gin.Context) {
// 		c.Set(proxyStorage.EndpointKey, proxyStorage.ImpressionsBulkBeaconEndpoint)
// 		if c.Request.Body == nil {
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.ImpressionsBulkBeaconEndpoint, http.StatusBadRequest)
// 			c.JSON(http.StatusBadRequest, nil)
// 			return
// 		}
//
// 		data, err := ioutil.ReadAll(c.Request.Body)
// 		if err != nil {
// 			log.Instance.Error(err)
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.ImpressionsBulkBeaconEndpoint, http.StatusInternalServerError)
// 			c.JSON(http.StatusInternalServerError, nil)
// 			return
// 		}
//
// 		type BeaconImpressions struct {
// 			Entries []dtos.ImpressionsDTO `json:"entries"`
// 			Sdk     string                `json:"sdk"`
// 			Token   string                `json:"token"`
// 		}
// 		var body BeaconImpressions
// 		if err := json.Unmarshal([]byte(data), &body); err != nil {
// 			log.Instance.Error(err)
// 			c.JSON(http.StatusBadRequest, nil)
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.ImpressionsBulkBeaconEndpoint, http.StatusBadRequest)
// 			return
// 		}
//
// 		if !validateAPIKey(keys, body.Token) {
// 			c.AbortWithStatus(401)
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.ImpressionsBulkBeaconEndpoint, http.StatusUnauthorized)
// 			return
// 		}
//
// 		impressions, err := json.Marshal(body.Entries)
// 		if err != nil {
// 			log.Instance.Error(err)
// 			c.JSON(http.StatusInternalServerError, nil)
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.ImpressionsBulkBeaconEndpoint, http.StatusInternalServerError)
// 			return
// 		}
//
// 		submitImpressions(impressionListenerEnabled, body.Sdk, "NA", "NA", "", impressions)
// 		c.JSON(http.StatusNoContent, nil)
// 		interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.ImpressionsBulkBeaconEndpoint, http.StatusOK)
// 	}
// }
//
// func postImpressionsCount() gin.HandlerFunc {
// 	return func(c *gin.Context) {
// 		c.Set(proxyStorage.EndpointKey, proxyStorage.ImpressionsCountEndpoint)
// 		sdkVersion := c.Request.Header.Get("SplitSDKVersion")
// 		machineIP := c.Request.Header.Get("SplitSDKMachineIP")
// 		machineName := c.Request.Header.Get("SplitSDKMachineName")
// 		data, err := ioutil.ReadAll(c.Request.Body)
// 		if err != nil {
// 			log.Instance.Error(err)
// 			c.JSON(http.StatusInternalServerError, nil)
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.ImpressionsCountEndpoint, http.StatusInternalServerError)
// 			return
// 		}
//
// 		code := http.StatusOK
// 		err = controllers.PostImpressionsCount(sdkVersion, machineIP, machineName, data)
// 		if err != nil {
// 			code = http.StatusInternalServerError
// 			if httpError, ok := err.(*dtos.HTTPError); ok {
// 				code = httpError.Code
// 			}
// 		}
// 		c.JSON(code, nil)
// 		interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.ImpressionsCountEndpoint, code)
// 	}
// }
//
// func postImpressionsCountBeacon(keys []string) gin.HandlerFunc {
// 	return func(c *gin.Context) {
// 		c.Set(proxyStorage.EndpointKey, proxyStorage.ImpressionsCountBeaconEndpoint)
// 		if c.Request.Body == nil {
// 			c.JSON(http.StatusBadRequest, nil)
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.ImpressionsCountBeaconEndpoint, http.StatusBadRequest)
// 			return
// 		}
//
// 		data, err := ioutil.ReadAll(c.Request.Body)
// 		if err != nil {
// 			log.Instance.Error(err)
// 			c.JSON(http.StatusInternalServerError, nil)
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.ImpressionsCountBeaconEndpoint, http.StatusInternalServerError)
// 			return
// 		}
//
// 		type BeaconImpressionsCount struct {
// 			Entries dtos.ImpressionsCountDTO `json:"entries"`
// 			Sdk     string                   `json:"sdk"`
// 			Token   string                   `json:"token"`
// 		}
// 		var body BeaconImpressionsCount
// 		if err := json.Unmarshal([]byte(data), &body); err != nil {
// 			log.Instance.Error(err)
// 			c.JSON(http.StatusBadRequest, nil)
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.ImpressionsCountBeaconEndpoint, http.StatusBadRequest)
// 			return
// 		}
//
// 		if !validateAPIKey(keys, body.Token) {
// 			c.AbortWithStatus(401)
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.ImpressionsCountBeaconEndpoint, http.StatusUnauthorized)
// 			return
// 		}
//
// 		impressionsCount, err := json.Marshal(body.Entries)
// 		if err != nil {
// 			log.Instance.Error(err)
// 			c.JSON(http.StatusInternalServerError, nil)
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.ImpressionsCountBeaconEndpoint, http.StatusInternalServerError)
// 			return
// 		}
//
// 		if len(body.Entries.PerFeature) == 0 {
// 			c.JSON(http.StatusNoContent, nil)
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.ImpressionsCountBeaconEndpoint, http.StatusNoContent)
// 			return
// 		}
//
// 		code := http.StatusNoContent
// 		err = controllers.PostImpressionsCount(body.Sdk, "NA", "NA", impressionsCount)
// 		if err != nil {
// 			code = http.StatusInternalServerError
// 			if httpError, ok := err.(*dtos.HTTPError); ok {
// 				code = httpError.Code
// 			}
// 		}
// 		c.JSON(code, nil)
// 		interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.ImpressionsCountBeaconEndpoint, code)
// 	}
// }

//-----------------------------------------------------------------------------
// EVENTS - RESULTS
//-----------------------------------------------------------------------------
// func submitEvents(sdkVersion string, machineIP string, machineName string, data []byte) {
// 	controllers.AddEvents(data, sdkVersion, machineIP, machineName)
// }
//
// func postEvents(c *gin.Context) {
// 	c.Set(proxyStorage.EndpointKey, proxyStorage.EventsBulkEndpoint)
// 	sdkVersion := c.Request.Header.Get("SplitSDKVersion")
// 	machineIP := c.Request.Header.Get("SplitSDKMachineIP")
// 	machineName := c.Request.Header.Get("SplitSDKMachineName")
// 	data, err := ioutil.ReadAll(c.Request.Body)
// 	if err != nil {
// 		log.Instance.Error(err)
// 		c.JSON(http.StatusInternalServerError, nil)
// 		interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.EventsBulkEndpoint, http.StatusInternalServerError)
// 		return
// 	}
//
// 	submitEvents(sdkVersion, machineIP, machineName, data)
// 	c.JSON(http.StatusOK, nil)
// 	interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.EventsBulkEndpoint, http.StatusOK)
// }
//
// func postEventsBeacon(keys []string) gin.HandlerFunc {
// 	return func(c *gin.Context) {
// 		c.Set(proxyStorage.EndpointKey, proxyStorage.EventsBulkBeaconEndpoint)
// 		if c.Request.Body == nil {
// 			c.JSON(http.StatusBadRequest, nil)
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.EventsBulkBeaconEndpoint, http.StatusBadGateway)
// 			return
// 		}
//
// 		data, err := ioutil.ReadAll(c.Request.Body)
// 		if err != nil {
// 			log.Instance.Error(err)
// 			c.JSON(http.StatusInternalServerError, nil)
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.EventsBulkBeaconEndpoint, http.StatusInternalServerError)
// 			return
// 		}
//
// 		type BeaconEvents struct {
// 			Entries []dtos.EventDTO `json:"entries"`
// 			Sdk     string          `json:"sdk"`
// 			Token   string          `json:"token"`
// 		}
// 		var body BeaconEvents
// 		if err := json.Unmarshal([]byte(data), &body); err != nil {
// 			log.Instance.Error(err)
// 			c.JSON(http.StatusBadRequest, nil)
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.EventsBulkBeaconEndpoint, http.StatusBadRequest)
// 			return
// 		}
//
// 		if !validateAPIKey(keys, body.Token) {
// 			c.AbortWithStatus(401)
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.EventsBulkBeaconEndpoint, http.StatusUnauthorized)
// 			return
// 		}
//
// 		events, err := json.Marshal(body.Entries)
// 		if err != nil {
// 			log.Instance.Error(err)
// 			c.JSON(http.StatusInternalServerError, nil)
// 			interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.EventsBulkBeaconEndpoint, http.StatusInternalServerError)
// 			return
// 		}
//
// 		submitEvents(body.Sdk, "NA", "NA", events)
// 		c.JSON(http.StatusNoContent, nil)
// 		interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.EventsBulkBeaconEndpoint, http.StatusNoContent)
// 	}
// }
//

/*
func auth(c *gin.Context) {
	c.Set(proxyStorage.EndpointKey, proxyStorage.AuthEndpoint)
	log.Instance.Debug(fmt.Sprintf("Headers: %v", c.Request.Header))
	c.JSON(http.StatusOK, gin.H{"pushEnabled": false, "token": ""})
	interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.AuthEndpoint, http.StatusNoContent)
}

//-----------------------------------------------------------------------------
// DUMMY LEGACY METRICS ENDPOINTS
//-----------------------------------------------------------------------------
func postMetricsTimes(c *gin.Context)    { dummyHandle(c, proxyStorage.LegacyTimeEndpoint) }
func postMetricsTime(c *gin.Context)     { dummyHandle(c, proxyStorage.LegacyTimesEndpoint) }
func postMetricsCounters(c *gin.Context) { dummyHandle(c, proxyStorage.LegacyCountersEndpoint) }
func postMetricsCounter(c *gin.Context)  { dummyHandle(c, proxyStorage.LegacyCounterEndpoint) }
func postMetricsGauge(c *gin.Context)    { dummyHandle(c, proxyStorage.LegacyGaugeEndpoint) }
func dummyHandle(c *gin.Context, endpoint int) {
	interfaces.LocalTelemetry.IncrEndpointStatus(endpoint, http.StatusOK)
	c.Set(proxyStorage.EndpointKey, endpoint)
	c.JSON(http.StatusOK, "")
}

//-----------------------------------------------------------------------------
// TELEMETRY ENDPOINTS
//-----------------------------------------------------------------------------
func postTelemetryConfig(c *gin.Context) {
	c.Set(proxyStorage.EndpointKey, proxyStorage.TelemetryConfigEndpoint)
	sdkVersion := c.Request.Header.Get("SplitSDKVersion")
	machineIP := c.Request.Header.Get("SplitSDKMachineIP")
	machineName := c.Request.Header.Get("SplitSDKMachineName")
	data, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		log.Instance.Error(err)
		c.JSON(http.StatusInternalServerError, nil)
		interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.TelemetryConfigEndpoint, http.StatusInternalServerError)
		return
	}

	code := http.StatusOK
	err = controllers.PostTelemetryConfig(sdkVersion, machineIP, machineName, data)
	if err != nil {
		code = http.StatusInternalServerError
		if httpError, ok := err.(*dtos.HTTPError); ok {
			code = httpError.Code
		}
	}
	c.JSON(code, nil)
	interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.TelemetryConfigEndpoint, code)
}

func postTelemetryStats(c *gin.Context) {
	c.Set(proxyStorage.EndpointKey, proxyStorage.TelemetryRuntimeEndpoint)
	sdkVersion := c.Request.Header.Get("SplitSDKVersion")
	machineIP := c.Request.Header.Get("SplitSDKMachineIP")
	machineName := c.Request.Header.Get("SplitSDKMachineName")
	data, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		log.Instance.Error(err)
		c.JSON(http.StatusInternalServerError, nil)
		interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.TelemetryRuntimeEndpoint, http.StatusInternalServerError)
		return
	}

	code := http.StatusOK
	err = controllers.PostTelemetryStats(sdkVersion, machineIP, machineName, data)
	if err != nil {
		code = http.StatusInternalServerError
		if httpError, ok := err.(*dtos.HTTPError); ok {
			code = httpError.Code
		}
	}
	c.JSON(code, nil)
	interfaces.LocalTelemetry.IncrEndpointStatus(proxyStorage.TelemetryRuntimeEndpoint, code)
}

*/
