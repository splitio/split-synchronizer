package mocks

import "github.com/stretchr/testify/mock"

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

type DeferredRecordingTaskMock struct {
	mock.Mock
}

func (t *DeferredRecordingTaskMock) Stage(rawData interface{}) error {
	args := t.Called(rawData)
	return args.Error(1)
}

func (t *DeferredRecordingTaskMock) Start() {
	t.Called()
}

func (t *DeferredRecordingTaskMock) Stop(blocking bool) error {
	args := t.Called(blocking)
	return args.Error(1)
}

func (t *DeferredRecordingTaskMock) IsRunning() bool {
	args := t.Called()
	return args.Get(0).(bool)
}
