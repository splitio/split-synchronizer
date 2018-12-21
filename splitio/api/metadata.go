package api

// SdkMetadata struct wraps all the information that the sdk needs to send when posting impressions, metrics and events.
type SdkMetadata struct {
	SdkVersion  string
	MachineIP   string
	MachineName string
}
