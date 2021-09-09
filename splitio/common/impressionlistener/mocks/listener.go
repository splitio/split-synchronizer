package mocks

import (
	"encoding/json"

	"github.com/splitio/go-split-commons/v4/dtos"
)

type ImpressionBulkListenerMock struct {
	SubmitCall func(imps json.RawMessage, metadata *dtos.Metadata) error
	StartCall  func() error
	StopCall   func(blocking bool) error
}

func (l *ImpressionBulkListenerMock) Submit(imps json.RawMessage, metadata *dtos.Metadata) error {
	return l.SubmitCall(imps, metadata)
}

func (l *ImpressionBulkListenerMock) Start() error {
	return l.StartCall()
}

func (l *ImpressionBulkListenerMock) Stop(blocking bool) error {
	return l.StopCall(blocking)
}
