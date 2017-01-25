// Package fetcher implements all kind of Split/Segments fetchers
package fetcher

import (
	"github.com/splitio/go-agent/errors"
	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/api"
)

// SegmentFetcherFactory creates segment fetcher instance based on configuration
type SegmentFetcherFactory struct {
}

// NewInstance returns an instance of implemented SegmentFetcher interface
func (f SegmentFetcherFactory) NewInstance() SegmentFetcher {
	return NewHTTPSegmentFetcher()
}

// NewHTTPSegmentFetcher returns an instance of HTTPSegmentFetcher
func NewHTTPSegmentFetcher() HTTPSegmentFetcher {
	return HTTPSegmentFetcher{since: -1}
}

// HTTPSegmentFetcher implements SegmentFetcher interface
type HTTPSegmentFetcher struct {
	since int64
}

// NewInstance returns a new instance
func (h HTTPSegmentFetcher) NewInstance() HTTPSegmentFetcher {
	return HTTPSegmentFetcher{since: -1}
}

// Fetch splits from Split servers
func (h HTTPSegmentFetcher) Fetch(name string) (*api.SegmentChangesDTO, error) {

	segmentChangesDTO, err := api.SegmentChangesFetch(name, h.since)
	if errors.IsError(err) {
		log.Error.Println("Error fetching segments via HTTP ", err)
		return nil, err
	}

	// Update since value for next request
	if segmentChangesDTO.Till > h.since {
		h.since = segmentChangesDTO.Till
	}

	return segmentChangesDTO, nil
}
