// Package recorder implements all kind of data recorders just like impressions and metrics
package recorder

import "github.com/splitio/go-agent/splitio/api"

// ImpressionsRecorder interface to be implemented by Impressions loggers
type ImpressionsRecorder interface {
	Post(impressions []api.ImpressionsDTO, sdkVersion string, machineIP string) error
}
