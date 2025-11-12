package common

import (
	"github.com/splitio/go-split-commons/v8/engine/grammar/constants"
	"github.com/splitio/go-split-commons/v8/storage"
)

var ProducerFeatureFlagsRules = []string{constants.MatcherTypeAllKeys, constants.MatcherTypeInSegment, constants.MatcherTypeWhitelist, constants.MatcherTypeEqualTo, constants.MatcherTypeGreaterThanOrEqualTo, constants.MatcherTypeLessThanOrEqualTo, constants.MatcherTypeBetween,
	constants.MatcherTypeEqualToSet, constants.MatcherTypePartOfSet, constants.MatcherTypeContainsAllOfSet, constants.MatcherTypeContainsAnyOfSet, constants.MatcherTypeStartsWith, constants.MatcherTypeEndsWith, constants.MatcherTypeContainsString, constants.MatcherTypeInSplitTreatment,
	constants.MatcherTypeEqualToBoolean, constants.MatcherTypeMatchesString, constants.MatcherEqualToSemver, constants.MatcherTypeGreaterThanOrEqualToSemver, constants.MatcherTypeLessThanOrEqualToSemver, constants.MatcherTypeBetweenSemver, constants.MatcherTypeInListSemver,
	constants.MatcherTypeInRuleBasedSegment}

var ProducerRuleBasedSegmentRules = []string{constants.MatcherTypeAllKeys, constants.MatcherTypeInSegment, constants.MatcherTypeWhitelist, constants.MatcherTypeEqualTo, constants.MatcherTypeGreaterThanOrEqualTo, constants.MatcherTypeLessThanOrEqualTo, constants.MatcherTypeBetween,
	constants.MatcherTypeEqualToSet, constants.MatcherTypePartOfSet, constants.MatcherTypeContainsAllOfSet, constants.MatcherTypeContainsAnyOfSet, constants.MatcherTypeStartsWith, constants.MatcherTypeEndsWith, constants.MatcherTypeContainsString,
	constants.MatcherTypeEqualToBoolean, constants.MatcherTypeMatchesString, constants.MatcherEqualToSemver, constants.MatcherTypeGreaterThanOrEqualToSemver, constants.MatcherTypeLessThanOrEqualToSemver, constants.MatcherTypeBetweenSemver, constants.MatcherTypeInListSemver,
	constants.MatcherTypeInRuleBasedSegment}

// Storages wraps storages in one struct
type Storages struct {
	SplitStorage             storage.SplitStorage
	SegmentStorage           storage.SegmentStorage
	LocalTelemetryStorage    storage.TelemetryRuntimeConsumer
	EventStorage             storage.EventMultiSdkConsumer
	ImpressionStorage        storage.ImpressionMultiSdkConsumer
	UniqueKeysStorage        storage.UniqueKeysMultiSdkConsumer
	LargeSegmentStorage      storage.LargeSegmentsStorage
	RuleBasedSegmentsStorage storage.RuleBasedSegmentsStorage
}
