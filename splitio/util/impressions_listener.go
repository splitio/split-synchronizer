package util

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/splitio/go-agent/splitio/api"
)

type SecondaryHTTPImpressionRecorder struct{}

type impressionBundle struct {
	Impressions []api.ImpressionsDTO `json:"impressions"`
	SdkVersion  string               `json:"sdkVersion"`
	MachineIP   string               `json:"machineIP"`
	MachineName string               `json:"machineName:`
}

// Default queue sizes to 10 in case they're not specified at config time
var ImpressionListenerMainQueueSize int = 10
var ImpressionListenerFailedQueueSize int = 10

func (r SecondaryHTTPImpressionRecorder) Post(
	impressions []api.ImpressionsDTO,
	sdkVersion string,
	machineIP string,
	machineName string) error {

	client := &http.Client{}

	bundle := impressionBundle{
		Impressions: impressions,
		SdkVersion:  sdkVersion,
		MachineIP:   machineIP,
		MachineName: machineName,
	}

	data, err := json.Marshal(bundle)
	if err != nil {
		return err
	}

	request, _ := http.NewRequest("POST", "http://localhost:8888", bytes.NewBuffer(data))
	request.Close = true
	response, err := client.Do(request)
	if err != nil {
		return err
	} else {
		defer response.Body.Close()
	}

	return nil
}
