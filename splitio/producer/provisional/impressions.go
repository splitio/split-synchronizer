package provisional

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/splitio/go-split-commons/v4/conf"
	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-split-commons/v4/provisional"
	"github.com/splitio/go-split-commons/v4/storage"
	"github.com/splitio/go-toolkit/v5/common"
	"github.com/splitio/go-toolkit/v5/logging"
)

var errHTTP = errors.New("http")

const (
	defaultBulkSize         = 5000
	defaultBufferSize       = 10000000
	defaultProcessBatchSize = 5000
	defaultMaxConcurrency   = 5000
	defaultRedisFetchBulk   = 50000
	defaultHTTPTimeoutSecs  = 3
)

// ImpressionsEvictioner defines the interface for an impressions eviction component
type ImpressionsEvictioner interface {
	Start()
	Stop(blocking bool) error
}

// Config contains the set of options/parameters to setup the eviction component
type Config struct {
	Apikey             string
	EventsHost         string
	RedisFetchSize     int64
	ProcessConcurrency int
	ProcessBatcSize    int
	PostConcurrency    int
	BufferSize         int

	HTTPTimeout time.Duration
}

func (c *Config) normalize() {
	if c.EventsHost == "" {
		c.EventsHost = "https://events.split.io"
	}

	if c.PostConcurrency == 0 {
		c.PostConcurrency = defaultMaxConcurrency
	}

	if c.ProcessConcurrency == 0 {
		c.ProcessConcurrency = runtime.NumCPU() / 2
	}

	if c.RedisFetchSize == 0 {
		c.RedisFetchSize = defaultRedisFetchBulk
	}

	if c.HTTPTimeout == 0 {
		c.HTTPTimeout = time.Second * defaultHTTPTimeoutSecs
	}

	if c.BufferSize == 0 {
		c.BufferSize = defaultBufferSize
	}

	if c.ProcessBatcSize == 0 {
		c.ProcessBatcSize = defaultProcessBatchSize
	}
}

// ImpressionsEvictionerImpl implements the ImpressionsEvictioner interface
type ImpressionsEvictionerImpl struct {
	inputBuffer     chan []string
	preSubmitBuffer chan impsWithMetadata
	storage         storage.ImpressionMultiSdkConsumer
	httpClient      http.Client
	logger          logging.LoggerInterface
	status          *status
	impManager      provisional.ImpressionManager

	// pools
	processBatchSlicePool *sync.Pool

	// parsed from config
	headers            http.Header
	url                string
	postConcurrency    int
	processConcurrency int
	processBatchSize   int
	redisFetchSize     int64
}

// NewImpressionsEvictioner constructs an impressions evictioner
func NewImpressionsEvictioner(
	storage storage.ImpressionMultiSdkConsumer,
	telemetry storage.TelemetryRuntimeProducer,
	logger logging.LoggerInterface,
	config Config,
) *ImpressionsEvictionerImpl {

	impressionManager, err := provisional.NewImpressionManager(conf.ManagerConfig{}, nil, telemetry)
	if err != nil {
		panic(err.Error())
	}

	t := http.DefaultTransport.(*http.Transport).Clone()
	config.normalize()
	t.MaxConnsPerHost = config.PostConcurrency
	t.MaxIdleConns = config.PostConcurrency
	t.MaxIdleConnsPerHost = config.PostConcurrency
	ret := &ImpressionsEvictionerImpl{
		inputBuffer:           make(chan []string, defaultProcessBatchSize),
		preSubmitBuffer:       make(chan impsWithMetadata, config.PostConcurrency*4),
		impManager:            impressionManager,
		logger:                logger,
		status:                newStatus(),
		processBatchSize:      config.ProcessBatcSize,
		processBatchSlicePool: &sync.Pool{New: func() interface{} { return make([][]byte, 0, config.ProcessBatcSize) }},
		storage:               storage,
		url:                   fmt.Sprintf("%s/testImpressions/bulk", config.EventsHost),
		postConcurrency:       config.PostConcurrency,
		processConcurrency:    config.ProcessConcurrency,
		redisFetchSize:        config.RedisFetchSize,
		httpClient: http.Client{
			Transport: t,
			Timeout:   config.HTTPTimeout,
		},
	}
	ret.headers = http.Header{}
	ret.headers.Add("Authorization", fmt.Sprintf("Bearer %s", config.Apikey))
	ret.headers.Add("Content-Type", "application/json")
	// TODO: Add metadata
	// TODO: Add gzip compression
	return ret
}

// Start begins execution
func (i *ImpressionsEvictionerImpl) Start() {

	for idx := 0; idx < i.postConcurrency; idx++ {
		go i.post()
	}

	for idx := 0; idx < i.processConcurrency; idx++ {
		go i.process()
	}

	go i.fill()
}

// IsRunning does nothing
func (i *ImpressionsEvictionerImpl) IsRunning() bool {
	return true
}

// Stop does nothing (for now)
func (i *ImpressionsEvictionerImpl) Stop(blocking bool) error {
	// TODO
	return nil
}

func (i *ImpressionsEvictionerImpl) fill() {
	timer := time.NewTimer(1 * time.Second)
	for i.status.Filling() {
		timer.Reset(1 * time.Second)
		imps, err := i.storage.PopNRaw(i.redisFetchSize)
		if len(imps) == 0 {
			select {
			case <-timer.C:
				continue
			case <-i.status.stopFilling:
				return
			}
		}
		if err != nil {
			i.logger.Error(fmt.Sprintf("error fetching impressions from redis: %s", err.Error()))
		}

		i.inputBuffer <- imps
	}
}

func (i *ImpressionsEvictionerImpl) process() {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for i.status.Processing() {
		func() {
			batch := i.processBatchSlicePool.Get().([][]byte)
			defer i.processBatchSlicePool.Put(batch) // recycle the buffer
			batch = batch[:0]                        // clear slice items without freeing memory

			timer.Reset(60 * time.Second)
			ready := false
			for !ready {
				select {
				case imps := <-i.inputBuffer:
					for idx := range imps {
						batch = append(batch, []byte(imps[idx]))
					}
					// batch = append(batch, imp)
					if len(batch) == i.processBatchSize {
						ready = true
					}
				case <-timer.C:
					ready = true
				}
			}

			if len(batch) == 0 {
				return
			}

			formatted := i.format(batch)
			for idx := range formatted {
				i.preSubmitBuffer <- formatted[idx]
			}
		}()
	}
}

func (i *ImpressionsEvictionerImpl) post() {
	for i.status.Posting() {
		select {
		case bulk := <-i.preSubmitBuffer:
			fmt.Println("posting impressions for ", len(bulk.imps), " features")
			serialized, err := json.Marshal(bulk.imps)

			if err != nil {
				i.logger.Error("error deserializing: ", err.Error())
				continue
			}

			req, err := http.NewRequest("POST", i.url, bytes.NewReader(serialized))
			if err != nil {
				i.logger.Error("error making request: ", err.Error())
				continue
			}
			req.Header = i.headers
			common.WithAttempts(3, func() error {
				resp, err := i.httpClient.Do(req)
				if err != nil {
					i.logger.Error("error posting: ", err.Error())
					return err
				}

				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					i.logger.Error("bad status code when posting impressions: ", resp.StatusCode)
					return errHTTP
				}

				if resp.Body != nil {
					resp.Body.Close()
				}
				return nil
			})
		case <-i.status.stopPosting:
			return
		}
	}
}

func (i *ImpressionsEvictionerImpl) format(raws [][]byte) []impsWithMetadata {
	batches := newImpBatches(10)
	for _, raw := range raws {
		var queueObj dtos.ImpressionQueueObject
		err := json.Unmarshal(raw, &queueObj)
		toLog, _ := i.impManager.ProcessSingle(&queueObj.Impression)
		if !toLog {
			continue
		}

		if err != nil {
			i.logger.Error("error deserializing fetched impression: ", err.Error())
			continue
		}

		batches.add(&queueObj)
	}

	// we remove the name-index mapping of each impression group prior to returning since we no longer need it
	// this way, memory can be collected by the GC
	for retIndex := range batches.groups {
		batches.groups[retIndex].clearTestNameMapping()
	}
	return batches.groups
}

// This struct is used to maintain a slice of ready-to-post impression bulks, grouped by metadata,
// and partitioned by bulk size. The index is used to access the latest bulk being built for a specific metadata.
// This indirection helps avoid fetching the item from the map, updating it and storing it again which can be more expensive
// Because we're splitting the bulks by size as well, there can be more than one bulk for the same metadata.
// When the max size is reached, a new one is added to the list and the index is updated accordingly
type impBatches struct {
	groups []impsWithMetadata
	index  map[dtos.Metadata]int
}

func newImpBatches(metadataCount int) *impBatches {
	return &impBatches{
		groups: make([]impsWithMetadata, 0, metadataCount),
		index:  make(map[dtos.Metadata]int, metadataCount),
	}
}

// add an impression to a bulk
// after identifying the correct bulk (or creating one if necessary), the call is forwarded
// to such structure. (see impsWithMetadata.add)
func (i *impBatches) add(queueObj *dtos.ImpressionQueueObject) {
	idx, ok := i.index[queueObj.Metadata]
	if !ok || i.groups[idx].count > defaultBulkSize {
		i.groups = append(i.groups, newMakeImpsWithMeta(10, &queueObj.Metadata)) // again, take a guess on the size
		idx = len(i.groups) - 1
		i.index[queueObj.Metadata] = idx
	}
	i.groups[idx].add(&queueObj.Impression)
}

type impsWithMetadata struct {
	metadata dtos.Metadata
	imps     []dtos.ImpressionsDTO
	count    int
	nindex   map[string]int
}

func newMakeImpsWithMeta(size int, meta *dtos.Metadata) impsWithMetadata {
	return impsWithMetadata{
		metadata: *meta,
		imps:     make([]dtos.ImpressionsDTO, 0, size),
		nindex:   make(map[string]int),
	}
}

func (s *impsWithMetadata) add(i *dtos.Impression) {
	if i == nil {
		// TODO: Log? (this should not happen)
		return
	}
	idx, ok := s.nindex[i.FeatureName]
	if !ok {
		s.imps = append(s.imps, dtos.ImpressionsDTO{
			TestName:       i.FeatureName,
			KeyImpressions: make([]dtos.ImpressionDTO, 0, 50), // yet another guess
		})
		idx = len(s.imps) - 1
		s.nindex[i.FeatureName] = idx
	}
	s.imps[idx].KeyImpressions = append(s.imps[idx].KeyImpressions, dtos.ImpressionDTO{
		KeyName:      i.KeyName,
		Treatment:    i.Treatment,
		Time:         i.Time,
		ChangeNumber: i.ChangeNumber,
		Label:        i.Label,
		BucketingKey: i.BucketingKey,
		Pt:           i.Pt,
	})
	s.count++
}

func (s *impsWithMetadata) clearTestNameMapping() {
	s.nindex = nil
}

type status struct {
	filling        atomic.Value
	processing     atomic.Value
	posting        atomic.Value
	stopFilling    chan struct{}
	stopProcessing chan struct{}
	stopPosting    chan struct{}
}

func (s *status) Filling() bool {
	return s.filling.Load().(bool)
}

func (s *status) Processing() bool {
	return s.processing.Load().(bool)
}

func (s *status) Posting() bool {
	return s.posting.Load().(bool)
}

func (s *status) StopFilling() {
	s.filling.Store(false)
	s.stopFilling <- struct{}{}
}

func (s *status) StopProcessing() {
	s.filling.Store(false)
	s.stopProcessing <- struct{}{}
}

func (s *status) StopPosting() {
	s.filling.Store(false)
	s.stopFilling <- struct{}{}
}

func newStatus() *status {
	ret := &status{
		stopFilling:    make(chan struct{}, 1),
		stopProcessing: make(chan struct{}, 1),
		stopPosting:    make(chan struct{}, 1),
	}
	ret.filling.Store(true)
	ret.processing.Store(true)
	ret.posting.Store(true)
	return ret
}

var _ ImpressionsEvictioner = (*ImpressionsEvictionerImpl)(nil)
