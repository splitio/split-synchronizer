package appcontext

import "github.com/splitio/split-synchronizer/v4/splitio"

const (
	_ = iota
	// ProxyMode mode proxy on
	ProxyMode

	// ProducerMode mode producer on
	ProducerMode
)

var mode int

// Initialize appcontext module
func Initialize(m int) {
	mode = m
}

// ExecutionMode returns the initialized mode
func ExecutionMode() int {
	return mode
}

// VersionHeader returns the version header based on execution mode
func VersionHeader() string {
	if mode == ProducerMode {
		return "SplitSyncProducerMode-" + splitio.Version
	}
	return "SplitSyncProxyMode-" + splitio.Version
}
