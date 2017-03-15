// Package fetcher implements all kind of Split/Segments fetchers
package fetcher

import (
	"github.com/splitio/go-agent/errors"
	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/api"
)

// HTTPSplitFetcher implemts SplitFetcher interface
type HTTPSplitFetcher struct{}

// NewHTTPSplitFetcher returns an instance of HTTPSplitFetcher
func NewHTTPSplitFetcher() HTTPSplitFetcher {
	return HTTPSplitFetcher{}
}

// Fetch splits from Split servers
func (h HTTPSplitFetcher) Fetch(changeNumber int64) (*api.SplitChangesDTO, error) {
	splitChangesDTO, err := api.SplitChangesFetch(changeNumber)
	if errors.IsError(err) {
		log.Error.Println("Error fetching splits via HTTP ", err)
		return nil, err
	}

	return splitChangesDTO, nil
}
