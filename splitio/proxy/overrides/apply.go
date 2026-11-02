package overrides

import (
	"maps"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/engine/grammar/constants"
)

const overrideLabel = "proxy override"

// Apply returns flag with its override applied. A flag with no override is returned as is.
//
// The input is never modified: storage hands out DTOs that can share maps and slices with the in-memory snapshot.
func (o *Overrides) Apply(flag dtos.SplitDTO) dtos.SplitDTO {
	entry, ok := o.Entries[flag.Name]
	if !ok {
		return flag
	}

	flag.Conditions = []dtos.ConditionDTO{{
		ConditionType: "ROLLOUT",
		Label:         overrideLabel,
		MatcherGroup: dtos.MatcherGroupDTO{
			Combiner: "AND",
			Matchers: []dtos.MatcherDTO{{
				MatcherType: constants.MatcherTypeAllKeys,
				KeySelector: &dtos.KeySelectorDTO{TrafficType: flag.TrafficTypeName},
			}},
		},
		Partitions: []dtos.PartitionDTO{{Treatment: entry.Treatment, Size: 100}},
	}}
	flag.TrafficAllocation = 100
	flag.DefaultTreatment = entry.Treatment
	flag.Prerequisites = nil
	flag.Killed = false

	if entry.Config != nil {
		configs := maps.Clone(flag.Configurations)
		if configs == nil {
			configs = make(map[string]string, 1)
		}
		configs[entry.Treatment] = *entry.Config
		flag.Configurations = configs
	}
	return flag
}

// Stamp returns the change number that marks every flag as changed at startup: the greater of the current time in
// milliseconds and the snapshot's change number plus one.
func Stamp(nowMs, snapshotCN int64) int64 {
	return max(nowMs, snapshotCN+1)
}
