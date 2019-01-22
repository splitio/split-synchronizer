package task

import (
	"fmt"
	"sync/atomic"
)

// EventOperation indicates if task is running for Events
var EventOperation atomic.Value

// ImpressionOperation indicates if task is running for Impressions
var ImpressionOperation atomic.Value

// CanPerformEventOperation Returns if an operation is running on Events
func CanPerformEventOperation() bool {
	fmt.Println("CANPERFORM?")
	fmt.Println("CANPERFORM?")
	fmt.Println("CANPERFORM?")
	fmt.Println("CANPERFORM?")
	fmt.Println(EventOperation.Load())
	if EventOperation.Load() == nil {
		return true
	}
	return !EventOperation.Load().(bool)
}

// SetEventOperation Sets the valiue for atomic Events
func SetEventOperation(value bool) {
	EventOperation.Store(value)
}

// CanPerformImpressionOperation Returns if an operation is running on Impressions
func CanPerformImpressionOperation() bool {
	fmt.Println("CANPERFORM?")
	fmt.Println("CANPERFORM?")
	fmt.Println("CANPERFORM?")
	fmt.Println("CANPERFORM?")
	fmt.Println(ImpressionOperation.Load())
	if ImpressionOperation.Load() == nil {
		return true
	}
	return !ImpressionOperation.Load().(bool)
}

// SetImpressionOperation Sets the valiue for atomic Impressions
func SetImpressionOperation(value bool) {
	ImpressionOperation.Store(value)
}
