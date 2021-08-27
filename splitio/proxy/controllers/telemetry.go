package controllers

import (
	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-split-commons/v4/service/api"
	"github.com/splitio/split-synchronizer/v4/conf"
	"github.com/splitio/split-synchronizer/v4/log"
)

var telemetryRecorder *api.HTTPTelemetryRecorder

// InitializeTelemetryRecorder initializes impressionscount recorder
func InitializeTelemetryRecorder() {
	telemetryRecorder = api.NewHTTPTelemetryRecorder(conf.Data.APIKey, conf.ParseAdvancedOptions(), log.Instance)
}

// PostTelemetryConfig sends data to split
func PostTelemetryConfig(sdkVersion string, machineIP string, machineName string, data []byte) error {
	err := telemetryRecorder.RecordRaw("/metrics/config", data, dtos.Metadata{
		SDKVersion:  sdkVersion,
		MachineIP:   machineIP,
		MachineName: machineName,
	}, nil)
	if err != nil {
		log.Instance.Error(err)
		return err
	}
	return nil
}

// PostTelemetryStats sends data to split
func PostTelemetryStats(sdkVersion string, machineIP string, machineName string, data []byte) error {
	err := telemetryRecorder.RecordRaw("/metrics/usage", data, dtos.Metadata{
		SDKVersion:  sdkVersion,
		MachineIP:   machineIP,
		MachineName: machineName,
	}, nil)
	if err != nil {
		log.Instance.Error(err)
		return err
	}
	return nil
}
