package internal

import (
	"github.com/splitio/go-split-commons/v4/dtos"
)

//RawData represents the raw data submitted by an sdk when posting data with associated metadata
type RawData struct {
	Metadata dtos.Metadata
	Payload  []byte
}

func newRawData(metadata dtos.Metadata, payload []byte) *RawData {
	return &RawEvents{
		Metadata: metadata,
		Payload:  payload,
	}
}

// RawImpressions  represents a raw impression's bulk with associated impressions mode
type RawImpressions struct {
	RawData
	Mode string
}

// RawEvents represents the raw data submitted by an sdk when posting impressions
type RawEvents = RawData

// RawTelemetryConfig represents the raw data submitted by an sdk when posting sdk config
type RawTelemetryConfig = RawData

// RawTelemetryUsage represents the raw data submitted by an sdk when posting usage metrics
type RawTelemetryUsage = RawData

// RawImpressionCount represents the raw data submitted by an sdk when posting impression counts
type RawImpressionCount = RawData

// RawKeysClientSide represents the raw data submitted by an sdk when posting mtks for client side
type RawKeysClientSide = RawData

// RawKeysServerSide represents the raw data submitted by an sdk when posting mtks for server side
type RawKeysServerSide = RawData

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

// NewRawImpressionCounts constructs a RawImpressionCount wrapper object
func NewRawImpressionCounts(metadata dtos.Metadata, payload []byte) *RawImpressionCount {
	return newRawData(metadata, payload)
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

// NewRawTelemetryKeysClientSide constructs a RawEvents wrapper object
func NewRawTelemetryKeysClientSide(metadata dtos.Metadata, payload []byte) *RawKeysClientSide {
	return newRawData(metadata, payload)
}

// NewRawTelemetryKeysServerSide constructs a RawEvents wrapper object
func NewRawTelemetryKeysServerSide(metadata dtos.Metadata, payload []byte) *RawKeysServerSide {
	return newRawData(metadata, payload)
}
