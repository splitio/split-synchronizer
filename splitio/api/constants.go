package api

// DefaultSize indicates the default value for flushing Events and Impressions
const DefaultSize = int64(5000)

// MaxSizeToFlush indicates the maximmum size that can be flushing on a call
const MaxSizeToFlush = DefaultSize * 5
