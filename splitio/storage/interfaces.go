// Package storage implements different kind of storages for split information
package storage

import "github.com/splitio/go-agent/splitio/api"

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

// ImpressionStorage interface defines the impressions data storage actions
type ImpressionStorage interface {
	//The map key must be the name of the feature
	RetrieveImpressions() ([]api.ImpressionsDTO, error)
}
