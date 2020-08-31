package task

import "testing"

func TestRunner(t *testing.T) {
	if IsOperationRunning(ImpressionsOperation) {
		t.Error("It should not be running")
	}

	if IsOperationRunning(EventsOperation) {
		t.Error("It should not be running")
	}

	RequestOperation(ImpressionsOperation)
	if !IsOperationRunning(ImpressionsOperation) {
		t.Error("It should be running")
	}
	FinishOperation(ImpressionsOperation)
	if IsOperationRunning(ImpressionsOperation) {
		t.Error("It should not be running")
	}

	RequestOperation(EventsOperation)
	if !IsOperationRunning(EventsOperation) {
		t.Error("It should be running")
	}

	FinishOperation(EventsOperation)
	if IsOperationRunning(EventsOperation) {
		t.Error("It should not be running")
	}
}
