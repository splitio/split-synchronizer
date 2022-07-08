package task

import (
	"fmt"
	"net/http"

	"github.com/splitio/go-toolkit/v5/logging"
)

// UniqueKeysPipelineWorker implements all the required  methods to work with a pipelined task
type UniqueKeysPipelineWorker struct {
	logger logging.LoggerInterface

	url       string
	apikey    string
	fetchSize int64
}

func NewUniqueKeysWorker() Worker {
	return &UniqueKeysPipelineWorker{}
}

func (u *UniqueKeysPipelineWorker) Fetch() ([]string, error) {
	// TODO: implement this
	return []string{}, nil
}
func (u *UniqueKeysPipelineWorker) Process(rawData [][]byte, sink chan<- interface{}) error {
	// TODO: implement this
	return nil
}

func (u *UniqueKeysPipelineWorker) BuildRequest(data interface{}) (*http.Request, func(), error) {
	// TODO: implement this
	req, err := http.NewRequest("POST", u.url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("error building events post request: %w", err)
	}

	return req, nil, nil
}
