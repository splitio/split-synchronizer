package mocks

import (
	"github.com/splitio/split-synchronizer/v5/splitio/common/impressionlistener"
	"github.com/stretchr/testify/mock"

	"github.com/splitio/go-split-commons/v9/dtos"
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

type MockImpressionBulkListener struct {
	mock.Mock
}

func (l *MockImpressionBulkListener) Submit(imps []impressionlistener.ImpressionsForListener, metadata *dtos.Metadata) error {
	args := l.Called(imps, metadata)
	return args.Error(1)
}

func (l *MockImpressionBulkListener) Start() error {
	args := l.Called()
	return args.Error(1)
}

func (l *MockImpressionBulkListener) Stop(blocking bool) error {
	args := l.Called()
	return args.Error(1)
}
