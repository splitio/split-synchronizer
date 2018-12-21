package recorder

import (
	"encoding/json"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
)

// ImpressionsHTTPRecorder implrements ImpressionsRecorder interface
type ImpressionsHTTPRecorder struct{}

// Post send impressions to Split Events servers
func (r ImpressionsHTTPRecorder) Post(impressions []api.ImpressionsDTO, metadata api.SdkMetadata) error {

	data, err := json.Marshal(impressions)
	if err != nil {
		log.Error.Println("Error marshaling JSON", err.Error())
		return err
	}
	log.Verbose.Println(string(data))

	if err := api.PostImpressions(data, metadata.SdkVersion, metadata.MachineIP, metadata.MachineName); err != nil {
		log.Error.Println("Error posting impressions", err.Error())
		return err
	}

	return nil
}
