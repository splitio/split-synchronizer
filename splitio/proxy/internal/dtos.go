package internal

import (
	"github.com/splitio/go-split-commons/v4/dtos"
)

//RawData represents the raw data submitted by an sdk when posting data with associated metadata
type RawData struct {
	Metadata dtos.Metadata
	Payload  []byte
}

// RawImpressions  represents a raw impression's bulk with associated impressions mode
type RawImpressions struct {
	RawData
	Mode string
}

// RawEvents represent the raw data submitted by an sdk when posting impressions
type RawEvents = RawData

// RawTelemetryConfig represent the raw data submitted by an sdk when posting sdk config
type RawTelemetryConfig = RawData

// RawTelemetryUsage represent the raw data submitted by an sdk when posting usage metrics
type RawTelemetryUsage = RawData

func newRawData(metadata dtos.Metadata, payload []byte) *RawData {
	return &RawEvents{
		Metadata: metadata,
		Payload:  payload,
	}
}

// NewRawImpressions constructs a RawImpressions wrapper object
func NewRawImpressions(metadata dtos.Metadata, mode string, payload []byte) *RawImpressions {
	return &RawImpressions{
		RawData: RawData{
			Metadata: metadata,
			Payload:  payload,
		},
		Mode: mode,
	}
}

// NewRawEvents constructs a RawEvents wrapper object
func NewRawEvents(metadata dtos.Metadata, payload []byte) *RawEvents {
	return newRawData(metadata, payload)
}

// NewRawTelemetryConfig constructs a RawEvents wrapper object
func NewRawTelemetryConfig(metadata dtos.Metadata, payload []byte) *RawTelemetryConfig {
	return newRawData(metadata, payload)
}

// NewRawTelemetryUsage constructs a RawEvents wrapper object
func NewRawTelemetryUsage(metadata dtos.Metadata, payload []byte) *RawTelemetryUsage {
	return newRawData(metadata, payload)
}
