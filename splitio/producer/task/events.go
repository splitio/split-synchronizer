package task

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/splitio/split-synchronizer/v5/splitio/producer/evcalc"

	"github.com/splitio/go-split-commons/v9/dtos"
	"github.com/splitio/go-split-commons/v9/storage"
	"github.com/splitio/go-toolkit/v5/logging"
)

const (
	defaultEventsPerBulk  = 10000
	defaultEventFetchSize = 10000
)

// EventWorkerConfig bundles options
type EventWorkerConfig struct {
	Logger          logging.LoggerInterface
	Storage         storage.EventMultiSdkConsumer
	EvictionMonitor evcalc.Monitor
	URL             string
	Apikey          string
	FetchSize       int
}

func (c *EventWorkerConfig) normalize() {
	if c.FetchSize == 0 {
		c.FetchSize = defaultImpFetchSize
	}
}

// EventsPipelineWorker implements all the required  methods to work with a pipelined task
type EventsPipelineWorker struct {
	logger          logging.LoggerInterface
	storage         storage.EventMultiSdkConsumer
	evictionMonitor evcalc.Monitor

	url       string
	apikey    string
	fetchSize int64
	pool      eventsMemoryPool
}

// NewEventsWorker builds a pipeline-suited events worker
func NewEventsWorker(cfg *EventWorkerConfig) (*EventsPipelineWorker, error) {
	cfg.normalize()
	return &EventsPipelineWorker{
		logger:          cfg.Logger,
		evictionMonitor: cfg.EvictionMonitor,
		storage:         cfg.Storage,
		url:             cfg.URL + "/events/bulk",
		apikey:          cfg.Apikey,
		fetchSize:       int64(cfg.FetchSize),
		pool:            newEventWorkerMemoryPool(cfg.FetchSize, defaultMetasPerBulk, defaultEventsPerBulk),
	}, nil
}

// Fetch fetches raw events
// This interface is kinda inconsistent, since we really want byte streams to be deserialized,
// but because redis returns strings, we end up using that to avoid making copies.
// We should eventually revisit the redis client interface and see how feasible it is
// to return bytes directly.
func (i *EventsPipelineWorker) Fetch() ([]string, error) {
	raw, sizeAfterPop, err := i.storage.PopNRaw(i.fetchSize)
	if err != nil {
		return nil, fmt.Errorf("error fetching raw events: %w", err)
	}
	i.evictionMonitor.StoreDataFlushed(time.Now(), len(raw), sizeAfterPop)
	return raw, nil
}

// Process parses the raw data and packages the events
func (i *EventsPipelineWorker) Process(raws [][]byte, sink chan<- interface{}) error {
	batches := newEventBatches(i.pool)
	// After processing of these events is done, we release temporary structures but NOT the final data
	// which will be released after imrpessions have been successfully posted
	defer batches.recycleContainer()

	for _, raw := range raws {
		var queueObj dtos.QueueStoredEventDTO
		err := json.Unmarshal(raw, &queueObj)
		if err != nil {
			i.logger.Error("error deserializing fetched events: ", err.Error())
			continue
		}
		batches.add(&queueObj)
	}

	for retIndex := range batches.groups {
		sink <- batches.groups[retIndex]
	}
	return nil
}

// BuildRequest takes an intermediate object and generates an http request to post events
func (i *EventsPipelineWorker) BuildRequest(data interface{}) (*http.Request, error) {
	ewm, ok := data.(eventsWithMetadata)
	if !ok {
		return nil, fmt.Errorf("expected `eventsWithMeta`. Got: %T", data)
	}

	serialized, err := json.Marshal(ewm.events)
	req, err := http.NewRequest("POST", i.url, bytes.NewReader(serialized))
	if err != nil {
		return nil, fmt.Errorf("error building events post request: %w", err)
	}

	req.Header = http.Header{}
	req.Header.Add("Authorization", "Bearer "+i.apikey)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("SplitSDKVersion", ewm.metadata.SDKVersion)
	req.Header.Add("SplitSDKMachineIp", ewm.metadata.MachineIP)
	req.Header.Add("SplitSDKMachineName", ewm.metadata.MachineName)
	return req, nil
}

type eventBatches struct {
	groups eventsWithMetaSlice
	index  metadataMap
	pool   eventsMemoryPool
}

func newEventBatches(pool eventsMemoryPool) *eventBatches {
	toRet := &eventBatches{
		groups: pool.acquireEventsWithMeta(),
		index:  pool.acquireMetadataMap(),
		pool:   pool,
	}
	return toRet
}

// This method releases only the remporary containers, the data is released after being consumed
func (i *eventBatches) recycleContainer() {
	i.pool.releaseMetadataMap(i.index)
	i.pool.releaseEventsWithMeta(i.groups)
}

// add an event to a bulk
// after identifying the correct bulk (or creating one if necessary), the call is forwarded
// to such structure. (see eventsWithMetaSlice.add)
func (i *eventBatches) add(queueObj *dtos.QueueStoredEventDTO) {
	idx, ok := i.index[queueObj.Metadata]
	if !ok || i.groups[idx].count > defaultBulkSize {
		i.groups = append(i.groups, newEventsWithMetadata(i.pool, &queueObj.Metadata))
		idx = len(i.groups) - 1
		i.index[queueObj.Metadata] = idx
	}
	i.groups[idx].add(&queueObj.Event)
}

type eventsWithMetadata struct {
	pool     eventsMemoryPool
	metadata dtos.Metadata
	events   eventsSlice
	count    int
}

func newEventsWithMetadata(pool eventsMemoryPool, metadata *dtos.Metadata) eventsWithMetadata {
	return eventsWithMetadata{
		pool:     pool,
		metadata: *metadata,
		events:   pool.acquireEvents(),
	}
}

func (s eventsWithMetadata) recycle() {
	s.pool.releaseEvents(s.events)
}

func (s *eventsWithMetadata) add(e *dtos.EventDTO) {
	if e == nil {
		// TODO: Log? (this should not happen)
		return
	}
	s.events = append(s.events, *e)
	s.count++
}

// Memory pool implementation to minimize allocations & gargabe collector pressure on the cpu

type eventsSlice = []dtos.EventDTO
type eventsWithMetaSlice = []eventsWithMetadata

type eventsMemoryPool interface {
	acquireEvents() eventsSlice
	releaseEvents(e eventsSlice)
	acquireMetadataMap() metadataMap
	releaseMetadataMap(m metadataMap)
	acquireEventsWithMeta() eventsWithMetaSlice
	releaseEventsWithMeta(t eventsWithMetaSlice)
}

type eventsMemoryPoolImpl struct {
	metadataMaps   sync.Pool
	events         sync.Pool
	eventsWithMeta sync.Pool
}

func newEventWorkerMemoryPool(fetchSize int, metadatasPerBulk int, eventsPerBulk int) *eventsMemoryPoolImpl {
	return &eventsMemoryPoolImpl{
		metadataMaps:   sync.Pool{New: func() interface{} { return make(metadataMap, metadatasPerBulk) }},
		events:         sync.Pool{New: func() interface{} { return make(eventsSlice, 0, eventsPerBulk) }},
		eventsWithMeta: sync.Pool{New: func() interface{} { return make(eventsWithMetaSlice, 0, metadatasPerBulk) }},
	}
}

func (p *eventsMemoryPoolImpl) acquireEvents() eventsSlice {
	return p.events.Get().(eventsSlice)[:0]
}

func (p *eventsMemoryPoolImpl) releaseEvents(e eventsSlice) {
	p.events.Put(e)
}

func (p *eventsMemoryPoolImpl) acquireMetadataMap() metadataMap {
	t := p.metadataMaps.Get().(metadataMap)
	// Clear the map
	// this is optimized by the go compiler by clearing all the map buckets.
	// no actual iteration is performed.
	for k := range t {
		delete(t, k)
	}
	return t
}

func (p *eventsMemoryPoolImpl) releaseMetadataMap(m metadataMap) {
	p.metadataMaps.Put(m)
}

func (p *eventsMemoryPoolImpl) acquireEventsWithMeta() eventsWithMetaSlice {
	return p.eventsWithMeta.Get().(eventsWithMetaSlice)[:0]
}

func (p *eventsMemoryPoolImpl) releaseEventsWithMeta(t eventsWithMetaSlice) {
	p.eventsWithMeta.Put(t)
}

var _ eventsMemoryPool = (*eventsMemoryPoolImpl)(nil)
var _ Worker = (*EventsPipelineWorker)(nil)
