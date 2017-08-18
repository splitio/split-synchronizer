package fetcher

import "github.com/splitio/split-synchronizer/splitio/api"

// SplitFetcher interface to be implemented by Split Fetchers
type SplitFetcher interface {
	Fetch(changeNumber int64) (*api.SplitChangesDTO, error)
}

// SegmentFetcher interface to be implemented by Segment Fetchers
type SegmentFetcher interface {
	Fetch(name string, changeNumber int64) (*api.SegmentChangesDTO, error)
}

// SegmentFetcherFactory interface to be implemented by Segment Fetchers Factories
type SegmentFetcherFactory interface {
	NewInstance() SegmentFetcher
}
