package task

import "sync"

var opsMutex = sync.RWMutex{}
var ops = make(map[string]bool)

// EventsOperation tag to register an operation on Events
const EventsOperation = "eventsOperation"

// ImpressionsOperation tag to register an operation on Impressions
const ImpressionsOperation = "impressionsOperation"

// RequestOperation Checks if the operation can be executed
func RequestOperation(operation string) bool {
	opsMutex.Lock()
	defer opsMutex.Unlock()
	opStatus, ok := ops[operation]
	if !ok {
		// The operation is not registered on the Hashmap, it will be the first execution
		ops[operation] = true
		return true
	}

	if !opStatus {
		// The operation is not running at this time, set a flag to true to execute
		ops[operation] = true
		return true
	}

	// Operation is currently running
	return false
}

// FinishOperation finished an operation already executed
func FinishOperation(operation string) {
	opsMutex.Lock()
	defer opsMutex.Unlock()
	ops[operation] = false
}

// IsOperationRunning Indicates if the operation is running or not
func IsOperationRunning(operation string) bool {
	opsMutex.RLock()
	defer opsMutex.RUnlock()
	opStatus, _ := ops[operation]
	return opStatus
}
