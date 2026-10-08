package overrides

import (
	"encoding/json"
	"testing"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/stretchr/testify/assert"
)

func testOverrides(entries ...Entry) *Overrides {
	o := &Overrides{Entries: map[string]Entry{}}
	for _, e := range entries {
		o.Entries[e.Flag] = e
		o.order = append(o.order, e.Flag)
	}
	return o
}

func sref(s string) *string { return &s }

func richFlag() dtos.SplitDTO {
	attr := "plan"
	return dtos.SplitDTO{
		ChangeNumber:          123,
		TrafficTypeName:       "user",
		Name:                  "search_rerank",
		TrafficAllocation:     20,
		TrafficAllocationSeed: 7,
		Seed:                  9,
		Status:                "ACTIVE",
		Killed:                true,
		DefaultTreatment:      "v1",
		Algo:                  2,
		Sets:                  []string{"search", "ranking"},
		ImpressionsDisabled:   true,
		Prerequisites:         []dtos.Prerequisite{{FeatureFlagName: "other", Treatments: []string{"on"}}},
		Configurations:        map[string]string{"v1": `{"model":"big"}`, "v2": `{"model":"medium"}`},
		Conditions: []dtos.ConditionDTO{
			{
				ConditionType: "WHITELIST",
				Label:         "whitelisted",
				MatcherGroup: dtos.MatcherGroupDTO{Combiner: "AND", Matchers: []dtos.MatcherDTO{
					{MatcherType: "WHITELIST", KeySelector: &dtos.KeySelectorDTO{TrafficType: "user", Attribute: &attr}},
				}},
				Partitions: []dtos.PartitionDTO{{Treatment: "v1", Size: 100}},
			},
			{
				ConditionType: "ROLLOUT",
				Label:         "default rule",
				Partitions:    []dtos.PartitionDTO{{Treatment: "v1", Size: 50}, {Treatment: "v2", Size: 50}},
			},
		},
	}
}

func TestApplyRewritesTargeting(t *testing.T) {
	o := testOverrides(Entry{Flag: "search_rerank", Treatment: "v2"})
	got := o.Apply(richFlag())

	if assert.Len(t, got.Conditions, 1) {
		cond := got.Conditions[0]
		assert.Equal(t, "ROLLOUT", cond.ConditionType)
		assert.Equal(t, "proxy override", cond.Label)
		assert.Equal(t, []dtos.PartitionDTO{{Treatment: "v2", Size: 100}}, cond.Partitions)
		assert.Equal(t, "AND", cond.MatcherGroup.Combiner)
		if assert.Len(t, cond.MatcherGroup.Matchers, 1) {
			m := cond.MatcherGroup.Matchers[0]
			assert.Equal(t, "ALL_KEYS", m.MatcherType)
			assert.False(t, m.Negate)
			if assert.NotNil(t, m.KeySelector) {
				assert.Equal(t, "user", m.KeySelector.TrafficType)
				assert.Nil(t, m.KeySelector.Attribute)
			}
		}
	}
	assert.Equal(t, 100, got.TrafficAllocation)
	assert.Equal(t, "v2", got.DefaultTreatment)
	assert.Empty(t, got.Prerequisites)
	assert.False(t, got.Killed)
}

func TestApplyKeepsEverythingElse(t *testing.T) {
	o := testOverrides(Entry{Flag: "search_rerank", Treatment: "v2"})
	orig := richFlag()
	got := o.Apply(orig)

	assert.Equal(t, orig.ChangeNumber, got.ChangeNumber)
	assert.Equal(t, orig.TrafficTypeName, got.TrafficTypeName)
	assert.Equal(t, orig.Name, got.Name)
	assert.Equal(t, orig.TrafficAllocationSeed, got.TrafficAllocationSeed)
	assert.Equal(t, orig.Seed, got.Seed)
	assert.Equal(t, orig.Status, got.Status)
	assert.Equal(t, orig.Algo, got.Algo)
	assert.Equal(t, orig.Sets, got.Sets)
	assert.Equal(t, orig.ImpressionsDisabled, got.ImpressionsDisabled)
	assert.Equal(t, orig.Configurations, got.Configurations, "without config, every treatment's config is served unchanged")
}

func TestApplyConfig(t *testing.T) {
	o := testOverrides(Entry{Flag: "search_rerank", Treatment: "v2", Config: sref(`{"model":"small"}`)})
	got := o.Apply(richFlag())
	assert.Equal(t, map[string]string{"v1": `{"model":"big"}`, "v2": `{"model":"small"}`}, got.Configurations)

	t.Run("config on a flag that had none", func(t *testing.T) {
		flag := richFlag()
		flag.Configurations = nil
		got := o.Apply(flag)
		assert.Equal(t, map[string]string{"v2": `{"model":"small"}`}, got.Configurations)
	})
}

func TestApplyDoesNotMutateItsInput(t *testing.T) {
	o := testOverrides(Entry{Flag: "search_rerank", Treatment: "v2", Config: sref(`{"model":"small"}`)})
	orig := richFlag()
	snapshotBefore, err := json.Marshal(orig)
	assert.Nil(t, err)

	got := o.Apply(orig)

	snapshotAfter, err := json.Marshal(orig)
	assert.Nil(t, err)
	assert.JSONEq(t, string(snapshotBefore), string(snapshotAfter))

	// Writing to the result must not reach the input's shared maps and slices.
	got.Configurations["v1"] = "changed"
	got.Conditions[0].Label = "changed"
	assert.Equal(t, `{"model":"big"}`, orig.Configurations["v1"])
	assert.Equal(t, "whitelisted", orig.Conditions[0].Label)
}

func TestApplyPassesThroughFlagsWithoutAnOverride(t *testing.T) {
	o := testOverrides(Entry{Flag: "other_flag", Treatment: "on"})
	orig := richFlag()
	assert.Equal(t, orig, o.Apply(orig))

	empty := testOverrides()
	assert.Equal(t, orig, empty.Apply(orig))
}

func TestStamp(t *testing.T) {
	assert.Equal(t, int64(1790712345678), Stamp(1790712345678, 500), "clock ahead of the change number")
	assert.Equal(t, int64(2001), Stamp(1000, 2000), "change number ahead of the clock")
	assert.Equal(t, int64(1001), Stamp(1000, 1000), "equal values still move forward")
}
