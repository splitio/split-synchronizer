package controllers

import (
	"time"

	"github.com/splitio/go-split-commons/v4/storage"
	"github.com/splitio/go-split-commons/v4/telemetry"

	"github.com/splitio/split-synchronizer/v4/splitio/admin/views/dashboard"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb/collections"
	proxyStorage "github.com/splitio/split-synchronizer/v4/splitio/proxy/storage"
)

func bundleSplitInfo(splitStorage storage.SplitStorageConsumer) []dashboard.SplitSummary {
	all := splitStorage.All()
	summaries := make([]dashboard.SplitSummary, 0, len(all))
	for _, split := range all {
		treatments := make(map[string]struct{})
		for _, condition := range split.Conditions {
			for _, partition := range condition.Partitions {
				treatments[partition.Treatment] = struct{}{}
			}
		}

		treatmentsS := make([]string, 0, len(treatments))
		for t := range treatments {
			treatmentsS = append(treatmentsS, t)
		}

		summaries = append(summaries, dashboard.SplitSummary{
			Name:             split.Name,
			Active:           split.Status == "ACTIVE",
			Killed:           split.Killed,
			DefaultTreatment: split.DefaultTreatment,
			Treatments:       treatmentsS,
			LastModified:     time.Unix(0, split.ChangeNumber*int64(time.Millisecond)).UTC().Format(time.UnixDate),
		})
	}
	return summaries
}

func bundleSegmentInfo(splitStorage storage.SplitStorage, segmentStorage storage.SegmentStorageConsumer) []dashboard.SegmentSummary {
	names := splitStorage.SegmentNames()
	summaries := make([]dashboard.SegmentSummary, 0, names.Size())
	for _, name := range names.List() {
		strName, ok := name.(string)
		if !ok {
			continue
		}

		keys := segmentStorage.Keys(strName)
		cn, _ := segmentStorage.ChangeNumber(strName)
		removed := int(segmentStorage.CountRemovedKeys(strName))
		summaries = append(summaries, dashboard.SegmentSummary{
			Name:         strName,
			ActiveKeys:   keys.Size(),
			RemovedKeys:  removed,
			TotalKeys:    keys.Size() + removed,
			LastModified: time.Unix(0, cn*int64(time.Millisecond)).UTC().Format(time.UnixDate),
		})
	}

	return summaries
}

func bundleSegmentKeysInfo(name string, segmentStorage storage.SegmentStorageConsumer) []dashboard.SegmentKeySummary {

	keys := segmentStorage.Keys(name)
	segmentKeys := make([]dashboard.SegmentKeySummary, 0, keys.Size())

	if keys != nil {
		for _, key := range keys.List() {
			switch k := key.(type) {
			case collections.SegmentKey:
				segmentKeys = append(segmentKeys, dashboard.SegmentKeySummary{
					Name:         k.Name,
					Removed:      k.Removed,
					ChangeNumber: k.ChangeNumber,
				})
			case string:
				segmentKeys = append(segmentKeys, dashboard.SegmentKeySummary{Name: k})
			}
		}
	}

	return segmentKeys
}

func successfulRequests(t storage.TelemetryPeeker) int64 {
	var count int64
	for _, resource := range []int{telemetry.SplitSync, telemetry.SegmentSync, telemetry.ImpressionSync, telemetry.ImpressionCountSync,
		telemetry.EventSync, telemetry.TelemetrySync} {
		lats := t.PeekHTTPLatencies(resource)
		for _, lat := range lats {
			count += lat
		}
	}
	return count
}

func bundleProxyLatencies(localTelemetry storage.TelemetryRuntimeConsumer) []dashboard.ChartJSData {
	asPeeker, ok := localTelemetry.(proxyStorage.ProxyTelemetryPeeker)
	if !ok { // This will be the case when runnning in producer mode
		return nil
	}

	// TODO(mredolatti): we should start tracking beacon endpoints as well
	return []dashboard.ChartJSData{
		{
			Label:           "/api/splitChanges",
			Data:            int64ToInterfaceSlice(asPeeker.PeekEndpointLatency(proxyStorage.SplitChangesEndpoint)),
			BackgroundColor: dashboard.MakeRGBA(255, 159, 64, 0.2),
			BorderColor:     dashboard.MakeRGBA(255, 159, 64, 1),
			BorderWidth:     1,
		},
		{
			Label:           "/api/segmentChanges",
			Data:            int64ToInterfaceSlice(asPeeker.PeekEndpointLatency(proxyStorage.SegmentChangesEndpoint)),
			BackgroundColor: dashboard.MakeRGBA(54, 162, 235, 0.2),
			BorderColor:     dashboard.MakeRGBA(54, 162, 235, 1),
			BorderWidth:     1,
		},
		{
			Label:           "/api/testImpressions/bulk",
			Data:            int64ToInterfaceSlice(asPeeker.PeekEndpointLatency(proxyStorage.ImpressionsBulkEndpoint)),
			BackgroundColor: dashboard.MakeRGBA(75, 192, 192, 0.2),
			BorderColor:     dashboard.MakeRGBA(75, 192, 192, 1),
			BorderWidth:     1,
		},
		{
			Label:           "/api/events/bulk",
			Data:            int64ToInterfaceSlice(asPeeker.PeekEndpointLatency(proxyStorage.EventsBulkEndpoint)),
			BackgroundColor: dashboard.MakeRGBA(255, 205, 86, 0.2),
			BorderColor:     dashboard.MakeRGBA(255, 205, 86, 1),
			BorderWidth:     1,
		},
		{
			Label:           "/api/mySegments",
			Data:            int64ToInterfaceSlice(asPeeker.PeekEndpointLatency(proxyStorage.MySegmentsEndpoint)),
			BackgroundColor: dashboard.MakeRGBA(153, 102, 255, 0.2),
			BorderColor:     dashboard.MakeRGBA(153, 102, 255, 1),
			BorderWidth:     1,
		},
	}
}

func int64ToInterfaceSlice(in []int64) []interface{} {
	out := make([]interface{}, 0, len(in))
	for _, item := range in {
		out = append(out, item)
	}
	return out
}

func bundleLocalSyncLatencies(localTelemetry storage.TelemetryRuntimeConsumer) []dashboard.ChartJSData {
	asPeeker := localTelemetry.(storage.TelemetryPeeker)
	return []dashboard.ChartJSData{
		{
			Label:           "/api/splitChanges",
			Data:            int64ToInterfaceSlice(asPeeker.PeekHTTPLatencies(telemetry.SplitSync)),
			BackgroundColor: dashboard.MakeRGBA(255, 159, 64, 0.2),
			BorderColor:     dashboard.MakeRGBA(255, 159, 64, 1),
			BorderWidth:     1,
		},
		{
			Label:           "/api/segmentChanges",
			Data:            int64ToInterfaceSlice(asPeeker.PeekHTTPLatencies(telemetry.SegmentSync)),
			BackgroundColor: dashboard.MakeRGBA(54, 162, 235, 0.2),
			BorderColor:     dashboard.MakeRGBA(54, 162, 235, 1),
			BorderWidth:     1,
		},
		{
			Label:           "/api/testImpressions/bulk",
			Data:            int64ToInterfaceSlice(asPeeker.PeekHTTPLatencies(telemetry.ImpressionSync)),
			BackgroundColor: dashboard.MakeRGBA(75, 192, 192, 0.2),
			BorderColor:     dashboard.MakeRGBA(75, 192, 192, 1),
			BorderWidth:     1,
		},
		{
			Label:           "/api/events/bulk",
			Data:            int64ToInterfaceSlice(asPeeker.PeekHTTPLatencies(telemetry.EventSync)),
			BackgroundColor: dashboard.MakeRGBA(255, 205, 86, 0.2),
			BorderColor:     dashboard.MakeRGBA(255, 205, 86, 1),
			BorderWidth:     1,
		},
	}
}

// func formatNumber(n int64) string {
// 	//Hundred
// 	if n < 1000 {
// 		return fmt.Sprintf("%d", n)
// 	}
//
// 	//Thousand
// 	if n < 1000000 {
// 		k := float64(n) / float64(1000)
// 		return fmt.Sprintf("%.2f k", k)
// 	}
//
// 	//Million
// 	if n < 1000000000 {
// 		m := float64(n) / float64(1000000)
// 		return fmt.Sprintf("%.2f M", m)
// 	}
//
// 	//Billion
// 	if n < 1000000000000 {
// 		g := float64(n) / float64(1000000000)
// 		return fmt.Sprintf("%.2f G", g)
// 	}
//
// 	//Trillion
// 	if n < 1000000000000000 {
// 		t := float64(n) / float64(1000000000000)
// 		return fmt.Sprintf("%.2f T", t)
// 	}
//
// 	//Quadrillion
// 	q := float64(n) / float64(1000000000000000)
// 	return fmt.Sprintf("%.2f P", q)
// }
//
// func toRGBAString(r int, g int, b int, a float32) string {
// 	if a < 1 {
// 		return fmt.Sprintf("rgba(%d, %d, %d, %.1f)", r, g, b, a)
// 	}
//
// 	return fmt.Sprintf("rgba(%d, %d, %d, %d)", r, g, b, int(a))
// }

// func bundleLocalSyncLatencies(localTelemetry storage.TelemetryRuntimeConsumer) string {
// 	asPeeker, ok := localTelemetry.(storage.TelemetryPeeker)
// 	if !ok {
// 		return ""
// 	}
//
// 	latencies := []views.LatencyForChart{
// 		views.NewLatencyBucketsForChart(
// 			"/api/splitChanges",
// 			asPeeker.PeekHTTPLatencies(telemetry.SplitSync),
// 			toRGBAString(255, 159, 64, 0.2),
// 			toRGBAString(255, 159, 64, 1)),
// 		views.NewLatencyBucketsForChart(
// 			"/api/segmentChanges",
// 			asPeeker.PeekHTTPLatencies(telemetry.SegmentSync),
// 			toRGBAString(54, 162, 235, 0.2),
// 			toRGBAString(54, 162, 235, 1)),
// 		views.NewLatencyBucketsForChart(
// 			"/api/testImpressions/bulk",
// 			asPeeker.PeekHTTPLatencies(telemetry.ImpressionSync),
// 			toRGBAString(75, 192, 192, 0.2),
// 			toRGBAString(75, 192, 192, 1)),
// 		views.NewLatencyBucketsForChart(
// 			"/api/events/bulk",
// 			asPeeker.PeekHTTPLatencies(telemetry.EventSync),
// 			toRGBAString(255, 205, 86, 0.2),
// 			toRGBAString(255, 205, 86, 1)),
// 	}
//
// 	serialized, _ := json.Marshal(latencies)
// 	return string(serialized)
// }
//
// func bundleSegmentInsights(splitStorage storage.SplitStorage, segmentStorage storage.SegmentStorage) []views.CachedSegmentRowTPLVars {
// 	cachedSegments := splitStorage.SegmentNames()
//
// 	toRender := make([]views.CachedSegmentRowTPLVars, 0, cachedSegments.Size())
// 	for _, s := range cachedSegments.List() {
//
// 		segment, _ := s.(string)
// 		activeKeys := segmentStorage.Keys(segment)
// 		size := 0
// 		if activeKeys != nil {
// 			size = activeKeys.Size()
// 		}
//
// 		removedKeys := 0
// 		if appcontext.ExecutionMode() == appcontext.ProxyMode {
// 			removedKeys = int(segmentStorage.CountRemovedKeys(segment))
// 		}
//
// 		// LAST MODIFIED
// 		changeNumber, err := segmentStorage.ChangeNumber(segment)
// 		if err != nil {
// 			log.Instance.Warning(fmt.Sprintf("Error fetching last update for segment %s\n", segment))
// 		}
// 		lastModified := time.Unix(0, changeNumber*int64(time.Millisecond))
//
// 		toRender = append(toRender,
// 			views.CachedSegmentRowTPLVars{
// 				ProxyMode:    appcontext.ExecutionMode() == appcontext.ProxyMode,
// 				Name:         segment,
// 				ActiveKeys:   strconv.Itoa(size),
// 				LastModified: lastModified.UTC().Format(time.UnixDate),
// 				RemovedKeys:  strconv.Itoa(removedKeys),
// 				TotalKeys:    strconv.Itoa(removedKeys + size),
// 			})
// 	}
//
// 	return toRender
// }
//
// func parseEventsSize(eventStorage storage.EventsStorage) string {
// 	if appcontext.ExecutionMode() == appcontext.ProxyMode {
// 		return "0"
// 	}
//
// 	size := eventStorage.Count()
// 	eventsSize := strconv.FormatInt(size, 10)
//
// 	return eventsSize
// }
//
// func parseImpressionSize(impressionStorage storage.ImpressionStorage) string {
// 	if appcontext.ExecutionMode() == appcontext.ProxyMode {
// 		return "0"
// 	}
//
// 	size := impressionStorage.Count()
// 	impressionsSize := strconv.FormatInt(size, 10)
//
// 	return impressionsSize
// }
//
// func parseEventsLambda() string {
// 	if appcontext.ExecutionMode() == appcontext.ProxyMode {
// 		return "0"
// 	}
// 	lambda := task.GetEventsLambda()
// 	if lambda > 10 {
// 		lambda = 10
// 	}
// 	return strconv.FormatFloat(lambda, 'f', 2, 64)
// }
//
// func parseImpressionsLambda() string {
// 	if appcontext.ExecutionMode() == appcontext.ProxyMode {
// 		return "0"
// 	}
// 	lambda := task.GetImpressionsLambda()
// 	if lambda > 10 {
// 		lambda = 10
// 	}
// 	return strconv.FormatFloat(lambda, 'f', 2, 64)
// }
