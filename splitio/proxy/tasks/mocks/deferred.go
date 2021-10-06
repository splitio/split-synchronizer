package mocks

type MockDeferredRecordingTask struct {
	StageCall     func(rawData interface{}) error
	StartCall     func()
	StopCall      func(blocking bool) error
	IsRunningCall func() bool
}

func (t *MockDeferredRecordingTask) Stage(rawData interface{}) error {
	return t.StageCall(rawData)
}

func (t *MockDeferredRecordingTask) Start() {
	t.StartCall()
}

func (t *MockDeferredRecordingTask) Stop(blocking bool) error {
	return t.StopCall(blocking)
}

func (t *MockDeferredRecordingTask) IsRunning() bool {
	return t.IsRunningCall()
}
