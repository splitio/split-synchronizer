package optimized

import (
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v5/dtos"
	"github.com/stretchr/testify/assert"
)

func TestHistoricSplitStorage(t *testing.T) {

	var historic HistoricChanges
	historic.Update([]dtos.SplitDTO{
		{Name: "f1", Sets: []string{"s1", "s2"}, Status: "ACTIVE", ChangeNumber: 1, TrafficTypeName: "tt1"},
	}, []dtos.SplitDTO{}, 1)
	assert.Equal(t,
		[]FeatureView{
			{Name: "f1", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s1", true, 1}, {"s2", true, 1}}, Active: true, LastUpdated: 1},
		},
		historic.GetUpdatedSince(-1, nil))

	// process an update with no change in flagsets / split status
	// - fetching from -1 && 1 should return the same paylaod as before with only `lastUpdated` bumped to 2
	// - fetching from 2 should return empty
	historic.Update([]dtos.SplitDTO{
		{Name: "f1", Sets: []string{"s1", "s2"}, Status: "ACTIVE", ChangeNumber: 2, TrafficTypeName: "tt1"},
	}, []dtos.SplitDTO{}, 1)

	// no filter
	assert.Equal(t,
		[]FeatureView{
			{Name: "f1", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s1", true, 1}, {"s2", true, 1}}, Active: true, LastUpdated: 2},
		},
		historic.GetUpdatedSince(-1, nil))
	assert.Equal(t,
		[]FeatureView{
			{Name: "f1", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s1", true, 1}, {"s2", true, 1}}, Active: true, LastUpdated: 2},
		},
		historic.GetUpdatedSince(1, nil))
	assert.Equal(t, []FeatureView{}, historic.GetUpdatedSince(2, nil))

	// filter by s1
	assert.Equal(t,
		[]FeatureView{
			{Name: "f1", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s1", true, 1}, {"s2", true, 1}}, Active: true, LastUpdated: 2},
		},
		historic.GetUpdatedSince(-1, []string{"s1"}))
	assert.Equal(t,
		[]FeatureView{
			{Name: "f1", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s1", true, 1}, {"s2", true, 1}}, Active: true, LastUpdated: 2},
		},
		historic.GetUpdatedSince(1, []string{"s1"}))
	assert.Equal(t, []FeatureView{}, historic.GetUpdatedSince(2, []string{"s1"}))

	// filter by s2
	assert.Equal(t,
		[]FeatureView{
			{Name: "f1", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s1", true, 1}, {"s2", true, 1}}, Active: true, LastUpdated: 2},
		},
		historic.GetUpdatedSince(-1, []string{"s2"}))
	assert.Equal(t,
		[]FeatureView{
			{Name: "f1", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s1", true, 1}, {"s2", true, 1}}, Active: true, LastUpdated: 2},
		},
		historic.GetUpdatedSince(1, []string{"s2"}))
	assert.Equal(t, []FeatureView{}, historic.GetUpdatedSince(2, []string{"s2"}))

	// -------------------

	// process an update with one extra split
	// - fetching from -1, & 1 should return the same payload
	// - fetching from 2 shuold only return f2
	// - fetching from 3 should return empty
	historic.Update([]dtos.SplitDTO{
		{Name: "f2", Sets: []string{"s2", "s3"}, Status: "ACTIVE", ChangeNumber: 3, TrafficTypeName: "tt1"},
	}, []dtos.SplitDTO{}, 1)

	// assert correct behaviours for CN == 1..3 and no flag sets filter
	assert.Equal(t,
		[]FeatureView{
			{Name: "f1", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s1", true, 1}, {"s2", true, 1}}, Active: true, LastUpdated: 2},
			{Name: "f2", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s2", true, 3}, {"s3", true, 3}}, Active: true, LastUpdated: 3},
		},
		historic.GetUpdatedSince(-1, nil))
	assert.Equal(t,
		[]FeatureView{
			{Name: "f1", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s1", true, 1}, {"s2", true, 1}}, Active: true, LastUpdated: 2},
			{Name: "f2", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s2", true, 3}, {"s3", true, 3}}, Active: true, LastUpdated: 3},
		},
		historic.GetUpdatedSince(1, nil))
	assert.Equal(t,
		[]FeatureView{
			{Name: "f2", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s2", true, 3}, {"s3", true, 3}}, Active: true, LastUpdated: 3},
		},
		historic.GetUpdatedSince(2, nil))
	assert.Equal(t, []FeatureView{}, historic.GetUpdatedSince(3, nil))

	// filtering by s1:
	assert.Equal(t,
		[]FeatureView{
			{Name: "f1", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s1", true, 1}, {"s2", true, 1}}, Active: true, LastUpdated: 2},
		},
		historic.GetUpdatedSince(-1, []string{"s1"}))
	assert.Equal(t,
		[]FeatureView{
			{Name: "f1", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s1", true, 1}, {"s2", true, 1}}, Active: true, LastUpdated: 2},
		},
		historic.GetUpdatedSince(1, []string{"s1"}))
	assert.Equal(t, []FeatureView{}, historic.GetUpdatedSince(2, []string{"s1"}))
	assert.Equal(t, []FeatureView{}, historic.GetUpdatedSince(3, []string{"s1"}))

	// filtering by s2:
	assert.Equal(t,
		[]FeatureView{
			{Name: "f1", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s1", true, 1}, {"s2", true, 1}}, Active: true, LastUpdated: 2},
			{Name: "f2", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s2", true, 3}, {"s3", true, 3}}, Active: true, LastUpdated: 3},
		},
		historic.GetUpdatedSince(-1, []string{"s2"}))
	assert.Equal(t,
		[]FeatureView{
			{Name: "f1", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s1", true, 1}, {"s2", true, 1}}, Active: true, LastUpdated: 2},
			{Name: "f2", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s2", true, 3}, {"s3", true, 3}}, Active: true, LastUpdated: 3},
		},
		historic.GetUpdatedSince(1, []string{"s2"}))
	assert.Equal(t,
		[]FeatureView{
			{Name: "f2", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s2", true, 3}, {"s3", true, 3}}, Active: true, LastUpdated: 3},
		},
		historic.GetUpdatedSince(2, []string{"s2"}))
	assert.Equal(t, []FeatureView{}, historic.GetUpdatedSince(3, []string{"s2"}))

	//filtering by s3
	assert.Equal(t,
		[]FeatureView{
			{Name: "f2", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s2", true, 3}, {"s3", true, 3}}, Active: true, LastUpdated: 3},
		},
		historic.GetUpdatedSince(-1, []string{"s3"}))
	assert.Equal(t,
		[]FeatureView{
			{Name: "f2", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s2", true, 3}, {"s3", true, 3}}, Active: true, LastUpdated: 3},
		},
		historic.GetUpdatedSince(1, []string{"s3"}))
	assert.Equal(t,
		[]FeatureView{
			{Name: "f2", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s2", true, 3}, {"s3", true, 3}}, Active: true, LastUpdated: 3},
		},
		historic.GetUpdatedSince(2, []string{"s3"}))
	assert.Equal(t, []FeatureView{}, historic.GetUpdatedSince(3, []string{"s3"}))

	// -------------------

	// process an update that removes f1 from flagset s1
	// - fetching without a filter should remain the same
	// - fetching with filter = s1 should not return f1 in CN=-1, should return it without the flagset in greater CNs
	historic.Update([]dtos.SplitDTO{
		{Name: "f1", Sets: []string{"s2"}, Status: "ACTIVE", ChangeNumber: 4, TrafficTypeName: "tt1"},
	}, []dtos.SplitDTO{}, 1)

	assert.Equal(t,
		[]FeatureView{
			{Name: "f2", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s2", true, 3}, {"s3", true, 3}}, Active: true, LastUpdated: 3},
			{Name: "f1", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s1", false, 4}, {"s2", true, 1}}, Active: true, LastUpdated: 4},
		},
		historic.GetUpdatedSince(-1, nil))

	// with filter = s1 (f2 never was associated with s1, f1 is no longer associated)
	assert.Equal(t,
		[]FeatureView{},
		historic.GetUpdatedSince(-1, []string{"s1"}))
	assert.Equal(t,
		[]FeatureView{
			{Name: "f1", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s1", false, 4}, {"s2", true, 1}}, Active: true, LastUpdated: 4},
		},
		historic.GetUpdatedSince(1, []string{"s1"}))
	assert.Equal(t,
		[]FeatureView{
			{Name: "f1", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s1", false, 4}, {"s2", true, 1}}, Active: true, LastUpdated: 4},
		},
		historic.GetUpdatedSince(2, []string{"s1"}))
	assert.Equal(t,
		[]FeatureView{
			{Name: "f1", TrafficTypeName: "tt1", FlagSets: []FlagSetView{{"s1", false, 4}, {"s2", true, 1}}, Active: true, LastUpdated: 4},
		},
		historic.GetUpdatedSince(3, []string{"s1"}))
	assert.Equal(t, []FeatureView{}, historic.GetUpdatedSince(4, []string{"s1"}))

}

// -- code below is for benchmarking random access using hashsets (map[string]struct{}) vs sorted slices + binary search

func setupRandomData(flagsetLength int, flagsetCount int, splits int, flagSetsPerSplitMax int, userSets int) benchmarkDataSlices {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	rand.Seed(time.Now().UnixNano())
	makeStr := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = letters[rand.Intn(len(letters))]
		}
		return string(b)
	}

	flagSets := make([]string, 0, flagsetCount)
	for flagsetCount > 0 {
		flagSets = append(flagSets, makeStr(flagsetLength))
		flagsetCount--
	}

	views := make([]FeatureView, 0, splits)
	for len(views) < cap(views) {
		fscount := rand.Intn(flagSetsPerSplitMax)
		setsForSplit := make([]FlagSetView, 0, fscount)
		for fscount > 0 {
			setsForSplit = append(setsForSplit, FlagSetView{
				Name:        flagSets[rand.Intn(len(flagSets))],
				Active:      rand.Intn(2) > 0,
				LastUpdated: rand.Int63n(2),
			})
			fscount--
		}
		sort.Slice(setsForSplit, func(i, j int) bool { return setsForSplit[i].Name < setsForSplit[j].Name })
		views = append(views, FeatureView{
			Name:            makeStr(20),
			Active:          rand.Intn(2) > 0, // rand bool
			LastUpdated:     rand.Int63n(2),   // 1 or 2 (still an int but behaving like a bool if we filter by since=1)
			TrafficTypeName: makeStr(10),
			FlagSets:        setsForSplit,
		})

	}
	sort.Slice(views, func(i, j int) bool { return views[i].LastUpdated < views[j].LastUpdated })
	return benchmarkDataSlices{views, flagSets}
}

type benchmarkDataSlices struct {
	views []FeatureView
	sets  []string
}

func (b *benchmarkDataSlices) toBenchmarkDataForMaps() benchmarkDataMaps {
	setMap := make(map[string]struct{}, len(b.sets))
	for _, s := range b.sets {
		setMap[s] = struct{}{}
	}

	return benchmarkDataMaps{
		views: b.views,
		sets:  setMap,
	}

}

type benchmarkDataMaps struct {
	views []FeatureView
	sets  map[string]struct{}
}

// reference implementation for benchmarking purposes only
func copyAndFilterUsingMaps(views []FeatureView, sets map[string]struct{}, since int64) []FeatureView {
	toRet := make([]FeatureView, 0, len(views))
	for idx := range views {
		for fsidx := range views[idx].FlagSets {
			if _, ok := sets[views[idx].FlagSets[fsidx].Name]; ok {
				fsinfo := views[idx].FlagSets[fsidx]
				if fsinfo.Active || fsinfo.LastUpdated > since {
					toRet = append(toRet, views[idx].clone())
				}
			}
		}

	}
	return toRet
}

func BenchmarkFlagSetProcessing(b *testing.B) {

	b.Run("sorted-slice", func(b *testing.B) {
		data := make([]benchmarkDataSlices, 0, b.N)
		for i := 0; i < b.N; i++ {
			data = append(data, setupRandomData(20, 50, 500, 20, 10))
		}

		b.ResetTimer() // to ignore setup time & allocs

		for i := 0; i < b.N; i++ {
			copyAndFilter(data[i].views, data[i].sets, 1)
		}
	})

	b.Run("maps", func(b *testing.B) {
		data := make([]benchmarkDataMaps, 0, b.N)
		for i := 0; i < b.N; i++ {
			d := setupRandomData(20, 50, 500, 20, 10)
			data = append(data, d.toBenchmarkDataForMaps())
		}

		b.ResetTimer() // to ignore setup time & allocs

		for i := 0; i < b.N; i++ {
			copyAndFilterUsingMaps(data[i].views, data[i].sets, 1)
		}
	})
}
