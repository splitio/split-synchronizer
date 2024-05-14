package mocks

import (
	"github.com/splitio/go-split-commons/v6/dtos"
	"github.com/splitio/split-synchronizer/v5/splitio/common/impressionlistener"
)

type ImpressionBulkListenerMock struct {
	SubmitCall func(imps []impressionlistener.ImpressionsForListener, metadata *dtos.Metadata) error
	StartCall  func() error
	StopCall   func(blocking bool) error
}

func (l *ImpressionBulkListenerMock) Submit(imps []impressionlistener.ImpressionsForListener, metadata *dtos.Metadata) error {
	return l.SubmitCall(imps, metadata)
}

func (l *ImpressionBulkListenerMock) Start() error {
	return l.StartCall()
}

func (l *ImpressionBulkListenerMock) Stop(blocking bool) error {
	return l.StopCall(blocking)
}
