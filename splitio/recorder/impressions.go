package recorder

import (
	"encoding/json"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/api"
)

// ImpressionsHTTPRecorder implrements ImpressionsRecorder interface
type ImpressionsHTTPRecorder struct{}

// Post send impressions to Split Events servers
func (r ImpressionsHTTPRecorder) Post(impressions []api.ImpressionsDTO, sdkVersion string, machineIP string) error {

	data, err := json.Marshal(impressions)
	if err != nil {
		log.Error.Println("Error marshaling JSON", err.Error())
		return err
	}
	log.Verbose.Println(string(data))

	if err := api.PostImpressions(data, sdkVersion, machineIP); err != nil {
		log.Error.Println("Error posting impressions", err.Error())
		return err
	}

	return nil
}
