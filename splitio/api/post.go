// Package api contains all functions and dtos Split APIs
package api

// PostImpressions send impressions to Split events service
func PostImpressions(data []byte, sdkVersion string, machineIP string) error {

	url := "/testImpressions/bulk"

	eventsClient.ResetHeaders()
	eventsClient.AddHeader("SplitSDKVersion", sdkVersion)
	eventsClient.AddHeader("SplitSDKMachineIP", machineIP)

	err := eventsClient.Post(url, data)
	if err != nil {
		return err
	}
	return nil
}
