package tasks

import (
	"errors"
	"sync"

	"github.com/splitio/go-split-commons/v4/tasks"
	"github.com/splitio/go-toolkit/v5/asynctask"
	"github.com/splitio/go-toolkit/v5/logging"
	gtSync "github.com/splitio/go-toolkit/v5/sync"
	"github.com/splitio/go-toolkit/v5/workerpool"
)

// Right now, proxy mode has impressions, events & telemetry refresh rate properties. It's not really clear whether they add value or not
// to have such behaviour, but in order to not break everything now, we'll try to maintain that behavior.
// In order to do so, we're replacing the prevoius struct with multiple nested maps for 2 channels.
// An explicit one, which will capture incoming data from POST requests and an implicit one managed by a worker pool which will have N goroutines
// blocked there waiting for data to be pushed into so that they can post it to our BE.
// An async task will periodically flush the queue, by moving those elements into the worker pool's one. (we're using pointers
// the cost of copying those structs everywhere).
// The worker pool now defines the level of concurrency when posting data
// The size of the incoming  & worker pool channels, define the amount of impression posts that can be kept in memory

// ErrQueueFull is returned when attempting to add data to a full queue
var ErrQueueFull = errors.New("queue is full, data not pushed")

// DeferredRecordingTask defines the interface for a task that accepts POSTs and submits them asyncrhonously
type DeferredRecordingTask interface {
	Stage(rawData interface{}) error
	tasks.Task
}

// WorkerFactory defines the signature of a function for instantiating workers
type WorkerFactory = func() workerpool.Worker

type genericQueue = chan interface{}

// DeferredRecordingTaskImpl is in charge of fetching impressions from the queue and posting them to the split server BE
type DeferredRecordingTaskImpl struct {
	logger          logging.LoggerInterface
	task            *asynctask.AsyncTask
	drainInProgress *gtSync.AtomicBool
	pool            *workerpool.WorkerAdmin
	queue           genericQueue
	mutex           sync.Mutex
}

func newDeferredFlushTask(logger logging.LoggerInterface, wfactory WorkerFactory, period int, queueSize int, threads int) *DeferredRecordingTaskImpl {
	drainFlag := gtSync.NewAtomicBool(false)
	queue := make(genericQueue, queueSize)
	pool := workerpool.NewWorkerAdmin(queueSize, logger)
	trigger := func(loger logging.LoggerInterface) error {
		if !drainFlag.TestAndSet() {
			logger.Warning("Impressions flush requested while another one is in progress. Ignoring.")
			return nil
		}
		defer drainFlag.Unset() // clear the flag after we're done
		for len(queue) > 0 {
			pool.QueueMessage(<-queue)
		}
		return nil
	}

	for i := 0; i < threads; i++ {
		pool.AddWorker(wfactory())
	}

	return &DeferredRecordingTaskImpl{
		logger:          logger,
		task:            asynctask.NewAsyncTask("impressions-recorder", trigger, period, nil, nil, logger),
		drainInProgress: drainFlag,
		pool:            pool,
		queue:           queue,
	}
}

// Stage queues impressions to be sent when the timer expires or the queue is filled.
func (t *DeferredRecordingTaskImpl) Stage(data interface{}) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	select {
	case t.queue <- data:
	default:
		return ErrQueueFull
	}

	if len(t.queue) == cap(t.queue) { // The queue has become full with this new element we added
		t.task.WakeUp()
	}
	return nil
}

// Start starts the flushing task
func (t *DeferredRecordingTaskImpl) Start() {
	t.task.Start()
}

// Stop stops the flushing task
func (t *DeferredRecordingTaskImpl) Stop(blocking bool) error {
	return t.task.Stop(blocking)
}

// IsRunning returns whether the task is running
func (t *DeferredRecordingTaskImpl) IsRunning() bool {
	return t.IsRunning()
}

var _ DeferredRecordingTask = (*DeferredRecordingTaskImpl)(nil)
