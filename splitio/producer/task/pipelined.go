package task

import (
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"

	tsync "github.com/splitio/go-toolkit/v5/sync"

	"github.com/splitio/go-toolkit/v5/common"
	"github.com/splitio/go-toolkit/v5/logging"
)

const (
	defaultInputBufferSize  = 1000
	defaultProcessBatchSize = 10000
	defaultMaxConcurrency   = 2000
	defaultMaxAccumSecs     = 5
	defaultHTTPTimeoutSecs  = 3
)

// Config contains the set of options/parameters to setup the eviction component
type Config struct {
	Name               string
	Logger             logging.LoggerInterface
	Worker             Worker
	InputBufferSize    int
	ProcessConcurrency int
	ProcessBatchSize   int
	PostConcurrency    int
	MaxAccumWait       time.Duration
	HTTPTimeout        time.Duration
}

// Worker defines the methods that should be implemented by pipeline-suited data-flows
type Worker interface {
	Fetch() ([]string, error)
	Process(rawData [][]byte, sink chan<- interface{}) error
	BuildRequest(data interface{}) (*http.Request, error)
}

func (c *Config) normalize() {
	if c.InputBufferSize == 0 {
		c.InputBufferSize = defaultInputBufferSize
	}

	if c.PostConcurrency == 0 {
		c.PostConcurrency = defaultMaxConcurrency
	}

	if c.ProcessConcurrency == 0 {
		c.ProcessConcurrency = runtime.NumCPU() / 2
	}

	if c.HTTPTimeout == 0 {
		c.HTTPTimeout = time.Second * defaultHTTPTimeoutSecs
	}

	if c.ProcessBatchSize == 0 {
		c.ProcessBatchSize = defaultProcessBatchSize
	}

	if c.MaxAccumWait == 0 {
		c.MaxAccumWait = defaultMaxAccumSecs * time.Second
	}
}

// PipelinedSyncTask implements a fetch-process-evict buffered flow
// the decoupling of such operations and use of buffers in between allows different
// steps to be scaled individually in order to maximize throughput
type PipelinedSyncTask struct {
	// dependencies
	logger     logging.LoggerInterface
	httpClient http.Client
	worker     Worker
	pool       taskMemoryPool

	// configs
	name               string
	postConcurrency    int
	processConcurrency int
	processBatchSize   int
	maxAccumWait       time.Duration

	// synchronization elements
	inputBuffer     chan []string
	preSubmitBuffer chan interface{}
	waiter          sync.WaitGroup
	running         *tsync.AtomicBool
	shutdown        chan struct{}
}

// NewPipelinedTask constructs a pipelined task
func NewPipelinedTask(config *Config) (*PipelinedSyncTask, error) {
	t := http.DefaultTransport.(*http.Transport).Clone()
	config.normalize()
	t.MaxConnsPerHost = config.PostConcurrency
	t.MaxIdleConns = config.PostConcurrency
	t.MaxIdleConnsPerHost = config.PostConcurrency
	return &PipelinedSyncTask{
		name:               config.Name,
		logger:             config.Logger,
		worker:             config.Worker,
		httpClient:         http.Client{Transport: t, Timeout: config.HTTPTimeout},
		pool:               newTaskMemoryPool(config.ProcessBatchSize),
		processBatchSize:   config.ProcessBatchSize,
		postConcurrency:    config.PostConcurrency,
		processConcurrency: config.ProcessConcurrency,
		maxAccumWait:       config.MaxAccumWait,
		running:            tsync.NewAtomicBool(true),
		inputBuffer:        make(chan []string, config.InputBufferSize),
		preSubmitBuffer:    make(chan interface{}, config.PostConcurrency*4),
		shutdown:           make(chan struct{}, 1),
	}, nil
}

// Start begins execution
func (p *PipelinedSyncTask) Start() {
	p.waiter.Add(p.postConcurrency + p.processConcurrency + 1)
	for idx := 0; idx < p.postConcurrency; idx++ {
		go p.sinker()
	}

	processWaiter := &sync.WaitGroup{}
	processWaiter.Add(p.processConcurrency)
	for idx := 0; idx < p.processConcurrency; idx++ {
		go func() {
			p.processor()
			processWaiter.Done()
		}()
	}

	go func() {
		processWaiter.Wait()
		close(p.preSubmitBuffer)
	}()

	go p.filler()
}

// Stop the task and drain the pipe
func (p *PipelinedSyncTask) Stop(blocking bool) error {
	if !p.running.TestAndClear() {
		return errTaskRunning
	}
	p.shutdown <- struct{}{}
	if blocking {
		p.waiter.Wait()
	}
	return nil
}

// IsRunning returns whether the task is running or not
func (p *PipelinedSyncTask) IsRunning() bool {
	return p.running.IsSet()
}

func (p *PipelinedSyncTask) filler() {
	p.logger.Debug(fmt.Sprintf("[pipelined/%s] - starting filling task", p.name))
	defer p.waiter.Done()
	timer := time.NewTimer(1 * time.Second)
	for p.running.IsSet() {
		timer.Reset(1 * time.Second)
		raw, err := p.worker.Fetch()
		if err != nil {
			p.logger.Error(fmt.Sprintf("[pipelined/%s] fetch function returned error: %s", p.name, err))
		}

		if len(raw) == 0 {
			select {
			case <-timer.C:
				continue
			case <-p.shutdown:
				close(p.inputBuffer)
				return
			}
		}
		howMany := len(raw)
		select {
		case p.inputBuffer <- raw:
			p.logger.Debug(fmt.Sprintf("[pipelined/%s] Pushed %d items into the processing buffer", p.name, howMany))
		default:
			p.logger.Warning(fmt.Sprintf(
				"[pipelined/%s] - dropping bulk of %d fetched items because processing buffer is full", p.name, len(raw),
			))
		}
	}
}

func (p *PipelinedSyncTask) processor() {
	p.logger.Debug(fmt.Sprintf("[pipelined/%s] - starting processing task", p.name))
	defer p.waiter.Done()
	timer := time.NewTimer(p.maxAccumWait)
	defer timer.Stop()
	processing := tsync.NewAtomicBool(true)

	for processing.IsSet() {
		func() {
			batch := p.pool.getRawBuffer() // acquire a buffer from the pool and schedule a release
			defer p.pool.releaseRawBuffer(batch)

			ready := false
			for !ready {
				timer.Reset(p.maxAccumWait)
				select {
				case raws, ok := <-p.inputBuffer:
					if !ok { // no more elements to process, this is the last iteration
						processing.Unset()
						ready = true
					}

					// Regular flow
					for idx := range raws {
						batch = append(batch, []byte(raws[idx]))
					}
					if len(batch) >= p.processBatchSize {
						ready = true
					}
				case <-timer.C:
					if len(batch) > 0 {
						ready = true
					}
				}
			}

			if len(batch) == 0 {
				return
			}

			howMany := len(batch)
			p.logger.Debug(fmt.Sprintf("[pipelined/%s] processing %d raw items.", p.name, howMany))
			err := p.worker.Process(batch, p.preSubmitBuffer) // process the raw data and put the results in the buffer
			if err != nil {
				p.logger.Error(fmt.Sprintf("[pipelined/%s] failed to process %d items: %s", p.name, howMany, err))
			}
		}()
	}
}

func (p *PipelinedSyncTask) sinker() {
	p.logger.Debug(fmt.Sprintf("[pipelined/%s] - starting posting task", p.name))
	defer p.waiter.Done()
	for {

		bulk, ok := <-p.preSubmitBuffer
		if !ok { // no more processed data available, end this goroutine
			return
		}

		func() {
			if asRecyblable, ok := bulk.(recyclable); ok {
				defer asRecyblable.recycle()
			}

			err := common.WithAttempts(3, func() error {
				p.logger.Debug(fmt.Sprintf("[pipelined/%s] - impressions post ready. making request", p.name))
				req, err := p.worker.BuildRequest(bulk)
				if err != nil {
					return fmt.Errorf(fmt.Sprintf("[pipelined/%s] error building request: %s", p.name, err))
				}

				resp, err := p.httpClient.Do(req)
				if err != nil {
					return fmt.Errorf(fmt.Sprintf("[pipelined/%s] error posting: %s", p.name, err))
				}

				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					return fmt.Errorf(fmt.Sprintf("[pipelined/%s] bad status code when sinking data: %d", p.name, resp.StatusCode))
				}

				if resp.Body != nil {
					resp.Body.Close()
				}
				p.logger.Debug(fmt.Sprintf("[pipelined/%s] - impressions posted successfully", p.name))
				return nil
			})
			if err != nil {
				p.logger.Error(err)
			}
		}()
	}
}

type rawBuffer = [][]byte

type taskMemoryPool interface {
	getRawBuffer() rawBuffer
	releaseRawBuffer(b rawBuffer)
}

type taskMemoryPoolImpl struct {
	processBatchSlicePool *sync.Pool
}

func newTaskMemoryPool(processBatchSize int) *taskMemoryPoolImpl {
	return &taskMemoryPoolImpl{
		processBatchSlicePool: &sync.Pool{New: func() interface{} { return make([][]byte, 0, processBatchSize) }},
	}
}

func (t *taskMemoryPoolImpl) getRawBuffer() rawBuffer {
	return t.processBatchSlicePool.Get().(rawBuffer)[:0]
}

func (t *taskMemoryPoolImpl) releaseRawBuffer(b rawBuffer) {
	t.processBatchSlicePool.Put(b)
}

type recyclable interface {
	recycle()
}

var errHTTP = errors.New("http")
var errTaskRunning = errors.New("task already running")
