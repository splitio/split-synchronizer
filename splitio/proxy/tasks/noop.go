package tasks

// noopRecordingTask accepts SDK posts and discards them without queueing or dialing.
type noopRecordingTask struct{}

// NewNoopRecordingTask returns a sink that drops payloads locally.
func NewNoopRecordingTask() DeferredRecordingTask {
	return noopRecordingTask{}
}

func (noopRecordingTask) Stage(interface{}) error { return nil }

func (noopRecordingTask) Start() {}

func (noopRecordingTask) Stop(bool) error { return nil }

func (noopRecordingTask) IsRunning() bool { return false }
