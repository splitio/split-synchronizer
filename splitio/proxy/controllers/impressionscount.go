package controllers

import (
	"github.com/splitio/go-split-commons/dtos"
	"github.com/splitio/go-split-commons/service/api"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
)

var impressionsCountRecorder *api.HTTPImpressionRecorder

// InitializeImpressionsCountRecorder initializes impressionscount recorder
func InitializeImpressionsCountRecorder() {
	impressionsCountRecorder = api.NewHTTPImpressionRecorder(conf.Data.APIKey, conf.ParseAdvancedOptions(), log.Instance)
}

// PostImpressionsCount sends data to split
func PostImpressionsCount(sdkVersion string, machineIP string, machineName string, data []byte) error {
	err := impressionsCountRecorder.RecordRaw("/testImpressions/count", data, dtos.Metadata{
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
