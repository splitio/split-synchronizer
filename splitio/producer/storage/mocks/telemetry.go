package mocks

import (
	"github.com/splitio/go-split-commons/v6/dtos"
)

// RedisTelemetryConsumerMultiMock is a mock
type RedisTelemetryConsumerMultiMock struct {
	PopLatenciesCall  func() map[dtos.Metadata]dtos.MethodLatencies
	PopExceptionsCall func() map[dtos.Metadata]dtos.MethodExceptions
	PopConfigsCall    func() map[dtos.Metadata]dtos.Config
}

func (r *RedisTelemetryConsumerMultiMock) PopLatencies() map[dtos.Metadata]dtos.MethodLatencies {
	return r.PopLatenciesCall()
}

func (r *RedisTelemetryConsumerMultiMock) PopExceptions() map[dtos.Metadata]dtos.MethodExceptions {
	return r.PopExceptionsCall()
}

func (r *RedisTelemetryConsumerMultiMock) PopConfigs() map[dtos.Metadata]dtos.Config {
	return r.PopConfigsCall()
}
