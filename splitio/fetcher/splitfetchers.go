// Package fetcher implements all kind of Split/Segments fetchers
package fetcher

import (
	"github.com/splitio/go-agent/errors"
	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/api"
)

// HTTPSplitFetcher implemts SplitFetcher interface
type HTTPSplitFetcher struct {
	since *int64
}

// NewHTTPSplitFetcher returns an instance of HTTPSplitFetcher
func NewHTTPSplitFetcher(since int64) HTTPSplitFetcher {
	return HTTPSplitFetcher{since: &since}
}

// Fetch splits from Split servers
func (h HTTPSplitFetcher) Fetch() ([]api.SplitDTO, error) {
	splitChangesDTO, err := api.SplitChangesFetch(*h.since)
	if errors.IsError(err) {
		log.Error.Println("Error fetching splits via HTTP ", err)
		return nil, err
	}

	// Update since value for next request
	if splitChangesDTO.Till > *h.since {
		*h.since = splitChangesDTO.Till
		log.Info.Println("Saving next 'since' value at: ", *h.since)
	}

	return splitChangesDTO.Splits, nil
}
