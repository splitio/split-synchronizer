package api

import (
	"encoding/json"
)

//
// Splits DTOs
//

// SplitChangesDTO structure to map JSON message sent by Split servers.
type SplitChangesDTO struct {
	Till      int64      `json:"till"`
	Since     int64      `json:"since"`
	Splits    []SplitDTO `json:"splits"`
	RawSplits []*json.RawMessage
}

// SplitDTO structure to map an Split definition fetched from JSON message.
type SplitDTO struct {
	ChangeNumber          int64          `json:"changeNumber"`
	TrafficTypeName       string         `json:"trafficTypeName"`
	Name                  string         `json:"name"`
	TrafficAllocation     int            `json:"trafficAllocation"`
	TrafficAllocationSeed int64          `json:"trafficAllocationSeed"`
	Seed                  int64          `json:"seed"`
	Status                string         `json:"status"`
	Killed                bool           `json:"killed"`
	DefaultTreatment      string         `json:"defaultTreatment"`
	Algo                  int            `json:"algo"`
	Conditions            []ConditionDTO `json:"conditions"`
}

// MarshalBinary exports SplitDTO to JSON string
func (s SplitDTO) MarshalBinary() (data []byte, err error) {
	return json.Marshal(s)
}

// ConditionDTO structure to map a Condition fetched from JSON message.
type ConditionDTO struct {
	ConditionType string          `json:"conditionType"`
	MatcherGroup  MatcherGroupDTO `json:"matcherGroup"`
	Partitions    []PartitionDTO  `json:"partitions"`
	Label         string          `json:"label"`
}

// PartitionDTO structure to map a Partition definition fetched from JSON message.
type PartitionDTO struct {
	Treatment string `json:"treatment"`
	Size      int    `json:"size"`
}

// MatcherGroupDTO structure to map a Matcher Group definition fetched from JSON message.
type MatcherGroupDTO struct {
	Combiner string       `json:"combiner"`
	Matchers []MatcherDTO `json:"matchers"`
}

// MatcherDTO structure to map a Matcher definition fetched from JSON message.
type MatcherDTO struct {
	KeySelector        *KeySelectorDTO                   `json:"keySelector"`
	MatcherType        string                            `json:"matcherType"`
	Negate             bool                              `json:"negate"`
	UserDefinedSegment *UserDefinedSegmentMatcherDataDTO `json:"userDefinedSegmentMatcherData"`
	UnaryNumeric       *UnaryNumericMatcherDataDTO       `json:"unaryNumericMatcherData"`
	Whitelist          *WhitelistMatcherDataDTO          `json:"whitelistMatcherData"`
	Between            *BetweenMatcherDataDTO            `json:"betweenMatcherData"`
}

// UserDefinedSegmentMatcherDataDTO structure to map a Matcher definition fetched from JSON message.
type UserDefinedSegmentMatcherDataDTO struct {
	SegmentName string `json:"segmentName"`
}

// BetweenMatcherDataDTO structure to map a Matcher definition fetched from JSON message.
type BetweenMatcherDataDTO struct {
	DataType string `json:"dataType"` //NUMBER or DATETIME
	Start    int64  `json:"start"`
	End      int64  `json:"end"`
}

// UnaryNumericMatcherDataDTO structure to map a Matcher definition fetched from JSON message.
type UnaryNumericMatcherDataDTO struct {
	DataType string `json:"dataType"` //NUMBER or DATETIME
	Value    int64  `json:"value"`
}

// WhitelistMatcherDataDTO structure to map a Matcher definition fetched from JSON message.
type WhitelistMatcherDataDTO struct {
	Whitelist []string `json:"whitelist"`
}

// KeySelectorDTO structure to map a Key slector definition fetched from JSON message.
type KeySelectorDTO struct {
	TrafficType string  `json:"trafficType"`
	Attribute   *string `json:"attribute"`
}

//
// Segment DTO
//

// SegmentChangesDTO struct to map a segment change message.
type SegmentChangesDTO struct {
	Name    string   `json:"name"`
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Since   int64    `json:"since"`
	Till    int64    `json:"till"`
}

//
// Impression DTO
//

// ImpressionDTO struct to map an impression
type ImpressionDTO struct {
	KeyName      string `json:"keyName"`
	Treatment    string `json:"treatment"`
	Time         int64  `json:"time"`
	ChangeNumber int64  `json:"changeNumber"`
	Label        string `json:"label"`
	BucketingKey string `json:"bucketingKey,omitempty"`
}

// MarshalBinary exports ImpressionDTO to JSON string
func (s ImpressionDTO) MarshalBinary() (data []byte, err error) {
	return json.Marshal(s)
}

// ImpressionsDTO struct mapping impressions to post
type ImpressionsDTO struct {
	TestName       string          `json:"testName"`
	KeyImpressions []ImpressionDTO `json:"keyImpressions"`
}

// LatenciesDTO struct mapping latencies post
type LatenciesDTO struct {
	MetricName string  `json:"name"`
	Latencies  []int64 `json:"latencies"`
}

// CounterDTO struct mapping counts post
type CounterDTO struct {
	MetricName string `json:"name"`
	Count      int64  `json:"delta"`
}

// GaugeDTO struct mapping gauges post
type GaugeDTO struct {
	MetricName string  `json:"name"`
	Gauge      float64 `json:"value"`
}

// MySegmentDTO struct mapping segment data for mySegments endpoint
type MySegmentDTO struct {
	Name string `json:"name"`
}

//
// Events DTOs
//

// EventDTO struct mapping events json
type EventDTO struct {
	Key             string   `json:"key"`
	TrafficTypeName string   `json:"trafficTypeName"`
	EventTypeID     string   `json:"eventTypeId"`
	Value           *float64 `json:"value"`
	Timestamp       int64    `json:"timestamp"`
}

// RedisStoredMachineMetadataDTO maps sdk version, machine IP and machine name
type RedisStoredMachineMetadataDTO struct {
	SDKVersion  string `json:"s"`
	MachineIP   string `json:"i"`
	MachineName string `json:"n"`
}

// RedisStoredEventDTO maps the stored JSON object in redis by SDKs
type RedisStoredEventDTO struct {
	Metadata RedisStoredMachineMetadataDTO `json:"m"`
	Event    EventDTO                      `json:"e"`
}
