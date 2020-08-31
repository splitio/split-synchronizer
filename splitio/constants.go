package splitio

const (
	// SuccessfulOperation Operation was executed successfuly
	SuccessfulOperation = iota
	// ExitInvalidConfiguration Invalid Configuration Code to Exit
	ExitInvalidConfiguration
	// ExitRedisInitializationFailed Failed initialization of Redis
	ExitRedisInitializationFailed
	// ExitErrorDB Failed initialization of DB
	ExitErrorDB
	// ExitTaskInitialization Failed
	ExitTaskInitialization
)

// DefaultSize indicates the default value for flushing Events and Impressions
const DefaultSize = int64(5000)

// MaxSizeToFlush indicates the maximmum size that can be flushing on a call
const MaxSizeToFlush = DefaultSize * 5
