package common

import (
	"time"
)

// Runtime defines the interface
type Runtime interface {
	Uptime() time.Duration
	Shutdown()
	ShutdownWithMessage(message string)
}

// RuntimeImpl provides an implementation for the Runtime interface
type RuntimeImpl struct {
	startup time.Time
}

// NewRuntime constructs a RuntimeImpl object
func NewRuntime() *RuntimeImpl {
	return &RuntimeImpl{startup: time.Now()}
}

// Uptime returns how long the sync has been running
func (r *RuntimeImpl) Uptime() time.Duration {
	return time.Now().Sub(r.startup)
}

// Shutdown stops sends a SIGTERM to the current process
func (r *RuntimeImpl) Shutdown() {
	// TODO(mredolatti): implement!
}

// ShutdownWithMessage logs a message and then sends a SIGTERM to the current process
func (r *RuntimeImpl) ShutdownWithMessage(message string) {
	// TODO(mredolatti): implement!
}
