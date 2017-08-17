package fetcher

import (
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
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
	if err != nil {
		log.Error.Println("Error fetching splits via HTTP ", err)
		return nil, err
	}

	return splitChangesDTO, nil
}
