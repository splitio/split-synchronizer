package controllers

import (
	"sort"
	"strings"
	"time"

	"github.com/splitio/go-split-commons/v5/storage"
	"github.com/splitio/go-split-commons/v5/telemetry"

	"github.com/splitio/split-synchronizer/v5/splitio/admin/views/dashboard"
	"github.com/splitio/split-synchronizer/v5/splitio/producer/evcalc"
	proxyStorage "github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"
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

		if split.Sets == nil {
			split.Sets = make([]string, 0)
		}

		summaries = append(summaries, dashboard.SplitSummary{
			Name:             split.Name,
			Active:           split.Status == "ACTIVE",
			Killed:           split.Killed,
			DefaultTreatment: split.DefaultTreatment,
			Treatments:       treatmentsS,
			FlagSets:         split.Sets,
			LastModified:     time.Unix(0, split.ChangeNumber*int64(time.Millisecond)).UTC().Format(time.UnixDate),
			ChangeNumber:     split.ChangeNumber,
		})
	}
	return summaries
}

func bundleSegmentInfo(splitStorage storage.SplitStorage, segmentStorage storage.SegmentStorageConsumer) []dashboard.SegmentSummary {
	names := splitStorage.SegmentNames()
	summaries := make([]dashboard.SegmentSummary, 0, names.Size())

	// see if the segment storage is able to count segment keys and if so provide an appropriate function.
	// otherwise, just return 0.
	removedKeyCounter := func(segmentName string) int { return 0 }
	if withRemovedKeyCount, ok := segmentStorage.(proxyStorage.ProxySegmentStorage); ok {
		removedKeyCounter = func(segmentName string) int { return withRemovedKeyCount.CountRemovedKeys(segmentName) }
	}

	for _, name := range names.List() {
		strName, ok := name.(string)
		if !ok {
			continue
		}

		keys := segmentStorage.Keys(strName)
		if keys == nil {
			// error or segment not found
			continue
		}
		cn, _ := segmentStorage.ChangeNumber(strName)
		removed := removedKeyCounter(strName)
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
			case persistent.SegmentKey:
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

func getEventsSize(eventStorage storage.EventMultiSdkConsumer) int64 {
	if eventStorage == nil {
		return 0
	}

	return eventStorage.Count()
}

func getFlagSetsInfo(splitsStorage storage.SplitStorage) []dashboard.FlagSetsSummary {
	flagSetNames := splitsStorage.GetAllFlagSetNames()

	summaries := make([]dashboard.FlagSetsSummary, 0, len(flagSetNames))
	featureFlagsBySets := splitsStorage.GetNamesByFlagSets(flagSetNames)

	for key, featureFlags := range featureFlagsBySets {
		summaries = append(summaries, dashboard.FlagSetsSummary{
			Name:                   key,
			FeatureFlagsAssociated: int64(len(featureFlags)),
			FeatureFlags:           strings.Join(featureFlags, ", "),
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[j].Name > summaries[i].Name
	})

	return summaries
}

func getImpressionSize(impressionStorage storage.ImpressionMultiSdkConsumer) int64 {
	if impressionStorage == nil {
		return 0
	}

	return impressionStorage.Count()
}

func getLambda(monitor evcalc.Monitor) float64 {
	if monitor == nil {
		return 0
	}
	return monitor.Lambda()
}

func getUpstreamRequestCount(metrics storage.TelemetryRuntimeConsumer) (ok int64, errored int64) {
	asPeeker := metrics.(storage.TelemetryPeeker)
	resources := []int{telemetry.SplitSync, telemetry.SegmentSync, telemetry.ImpressionSync, telemetry.ImpressionCountSync, telemetry.EventSync}
	var totalCount int64
	var errorCount int64
	for _, res := range resources {
		for _, bucket := range asPeeker.PeekHTTPLatencies(res) {
			totalCount += bucket
		}

		for _, counter := range asPeeker.PeekHTTPErrors(res) {
			errorCount += int64(counter)
		}
	}

	return totalCount - errorCount, errorCount
}

func getProxyRequestCount(metrics storage.TelemetryRuntimeConsumer) (ok int64, errored int64) {
	asPeeker, k := metrics.(proxyStorage.ProxyTelemetryPeeker)
	if !k { // This will be the case when runnning in producer mode
		return 0, 0
	}

	resources := []int{proxyStorage.AuthEndpoint, proxyStorage.SplitChangesEndpoint, proxyStorage.SegmentChangesEndpoint,
		proxyStorage.MySegmentsEndpoint, proxyStorage.ImpressionsBulkEndpoint, proxyStorage.ImpressionsBulkBeaconEndpoint,
		proxyStorage.ImpressionsCountEndpoint, proxyStorage.ImpressionsBulkBeaconEndpoint, proxyStorage.EventsBulkEndpoint,
		proxyStorage.EventsBulkBeaconEndpoint}
	var okCount int64
	var errorCount int64
	for _, res := range resources {
		for code, count := range asPeeker.PeekEndpointStatus(res) {
			if code >= 200 && code < 300 {
				okCount += count
				continue
			}
			errorCount = +count
		}
	}

	return okCount, errorCount
}
