// Package fetcher implements all kind of Split/Segments fetchers
package fetcher

import (
	"github.com/splitio/go-agent/errors"
	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/api"
)

// NewHTTPSegmentFetcher returns an instance of HTTPSegmentFetcher
func NewHTTPSegmentFetcher(since int64) HTTPSegmentFetcher {
	return HTTPSegmentFetcher{}
}

// HTTPSegmentFetcher implements SegmentFetcher interface
type HTTPSegmentFetcher struct {
	since map[string]int64
}

// Fetch splits from Split servers
func (h HTTPSegmentFetcher) Fetch(name string) (*api.SegmentChangesDTO, error) {
	segmentChangesDTO, err := api.SegmentChangesFetch(name, h.since[name])
	if errors.IsError(err) {
		log.Error.Println("Error fetching segments via HTTP ", err)
		return nil, err
	}

	// Update since value for next request
	if segmentChangesDTO.Till > h.since[name] {
		h.since[name] = segmentChangesDTO.Till
	}

	return segmentChangesDTO, nil
}
