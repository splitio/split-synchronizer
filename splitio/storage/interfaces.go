package storage

import "github.com/splitio/split-synchronizer/splitio/api"

// SplitStorage interface defines the split data storage actions
type SplitStorage interface {
	Save(split interface{}) error
	Remove(split interface{}) error
	RegisterSegment(name string) error
	SetChangeNumber(changeNumber int64) error
	ChangeNumber() (int64, error)
}

// SegmentStorage interface defines the segments data storage actions
type SegmentStorage interface {
	RegisteredSegmentNames() ([]string, error)
	AddToSegment(segmentName string, keys []string) error
	RemoveFromSegment(segmentName string, keys []string) error
	SetChangeNumber(segmentName string, changeNumber int64) error
	ChangeNumber(segmentName string) (int64, error)
}

// SegmentStorageFactory interface defines the segment storage Adapter
type SegmentStorageFactory interface {
	// NewInstance returns an instance of implemented SegmentStorage interface
	NewInstance() SegmentStorage
}

// ImpressionStorage interface defines the impressions data storage actions
type ImpressionStorage interface {
	//Returns a map of impressions. The map key must be the name of the feature
	RetrieveImpressions() (map[string]map[string][]api.ImpressionsDTO, error)
}

// MetricsStorage interface defines the metrics data storage actions
type MetricsStorage interface {
	//returns [sdkNameAndVersion][machineIP][metricName] = int64
	RetrieveCounters() (map[string]map[string]map[string]int64, error)
	//returns [sdkNameAndVersion][machineIP][metricName] = [0,0,0,0,0,0,0,0,0,0,0 ... ]
	RetrieveLatencies() (map[string]map[string]map[string][]int64, error)
	//returns [sdkNameAndVersion][machineIP][metricName] = float64
	RetrieveGauges() (map[string]map[string]map[string]float64, error)
}

// EventStorage interface defines events storage actions
type EventStorage interface {
	//returns the first N elements from events queue
	PopN(n int64) ([]api.RedisStoredEventDTO, error)
	//Size returns the number of items into storage
	Size() int64
}
