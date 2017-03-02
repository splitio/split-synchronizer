// Package storage implements different kind of storages for split information
package storage

// SplitStorage interface defines the split data storage actions
type SplitStorage interface {
	Save(split interface{}) error
	SaveTill(till int64) error
	Remove(split interface{}) error
	RegisterSegment(name string) error
}

// SegmentStorage interface defines the segments data storage actions
type SegmentStorage interface {
	RegisteredSegmentNames() ([]string, error)
	AddToSegment(segmentName string, keys []string) error
	RemoveFromSegment(segmentName string, keys []string) error
	SetChangeNumber(segmentName string, changeNumber int64) error
	ChangeNumber(segmentName string) (int64, error)
}
