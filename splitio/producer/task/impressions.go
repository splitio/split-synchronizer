package task

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/splitio/split-synchronizer/v5/splitio/common/impressionlistener"
	"github.com/splitio/split-synchronizer/v5/splitio/producer/evcalc"

	"github.com/splitio/go-split-commons/v9/dtos"
	"github.com/splitio/go-split-commons/v9/provisional"
	"github.com/splitio/go-split-commons/v9/storage"
	"github.com/splitio/go-toolkit/v5/logging"
)

const (
	defaultImpFetchSize   = 20000
	defaultMetasPerBulk   = 10
	defaultFeatureCount   = 200
	defaultImpsPerFeature = 25
	defaultBulkSize       = defaultFeatureCount * defaultImpsPerFeature
)

// ImpressionWorkerConfig bundles options
type ImpressionWorkerConfig struct {
	Logger              logging.LoggerInterface
	Storage             storage.ImpressionMultiSdkConsumer
	ImpressionsListener impressionlistener.ImpressionBulkListener
	EvictionMonitor     evcalc.Monitor
	URL                 string
	Apikey              string
	FetchSize           int
	ImpressionManager   provisional.ImpressionManager
}

func (c *ImpressionWorkerConfig) normalize() {
	if c.FetchSize == 0 {
		c.FetchSize = defaultImpFetchSize
	}
}

// ImpressionsPipelineWorker implements all the required  methods to work with a pipelined task
type ImpressionsPipelineWorker struct {
	logger          logging.LoggerInterface
	storage         storage.ImpressionMultiSdkConsumer
	impManager      provisional.ImpressionManager
	impListener     impressionlistener.ImpressionBulkListener
	evictionMonitor evcalc.Monitor

	url       string
	apikey    string
	fetchSize int64
	pool      impressionsMemoryPool
}

// NewImpressionWorker builds a pipeline-suited impressions worker
func NewImpressionWorker(cfg *ImpressionWorkerConfig) (*ImpressionsPipelineWorker, error) {
	cfg.normalize()

	return &ImpressionsPipelineWorker{
		logger:          cfg.Logger,
		storage:         cfg.Storage,
		impListener:     cfg.ImpressionsListener,
		impManager:      cfg.ImpressionManager,
		url:             cfg.URL + "/testImpressions/bulk",
		apikey:          cfg.Apikey,
		fetchSize:       int64(cfg.FetchSize),
		evictionMonitor: cfg.EvictionMonitor,
		pool:            newImpWorkerMemoryPool(cfg.FetchSize, defaultMetasPerBulk, defaultFeatureCount, defaultImpsPerFeature),
	}, nil
}

// Fetch fetches raw impressions
// This interface is kinda inconsistent, since we really want byte streams to be deserialized,
// but because redis returns strings, we end up using that to avoid making copies.
// We should eventually revisit the redis client interface and see how feasible it is
// to return bytes directly.
func (i *ImpressionsPipelineWorker) Fetch() ([]string, error) {
	raw, sizeAfterPop, err := i.storage.PopNRaw(i.fetchSize)
	if err != nil {
		return nil, fmt.Errorf("error fetching raw impressions: %w", err)
	}
	i.evictionMonitor.StoreDataFlushed(time.Now(), len(raw), sizeAfterPop)
	return raw, nil
}

// Process parses the raw data and packages the impressions
func (i *ImpressionsPipelineWorker) Process(raws [][]byte, sink chan<- interface{}) error {
	batches := newImpBatches(i.pool)
	// After processing of these impressions is done, we release temporary structures but NOT the final data
	// which will be released after imrpessions have been successfully posted
	defer batches.recycleContainer()

	deduped := 0
	for _, raw := range raws {
		var queueObj dtos.ImpressionQueueObject
		err := json.Unmarshal(raw, &queueObj)
		if err != nil {
			i.logger.Error("error deserializing fetched impression: ", err.Error())
			continue
		}

		toLog := i.impManager.ProcessSingle(&queueObj.Impression)
		if !toLog {
			deduped++
			continue
		}

		batches.add(&queueObj)
	}

	i.logger.Debug(fmt.Sprintf("[pipelined imp worker] total impressions Processed: %d, deduped %d", len(raws), deduped))

	if i.impListener != nil {
		i.sendImpressionsToListener(batches)
	}

	for retIndex := range batches.groups {
		sink <- batches.groups[retIndex]
	}

	return nil
}

// BuildRequest takes an intermediate object and generates an http request to post impressions
func (i *ImpressionsPipelineWorker) BuildRequest(data interface{}) (*http.Request, error) {
	iwm, ok := data.(impsWithMetadata)
	if !ok {
		return nil, fmt.Errorf("expected `impsWithMeta`. Got: %T", data)
	}

	serialized, err := json.Marshal(iwm.imps)
	req, err := http.NewRequest("POST", i.url, bytes.NewReader(serialized))
	if err != nil {
		return nil, fmt.Errorf("error building impressions post request: %w", err)
	}

	req.Header = http.Header{}
	req.Header.Add("Authorization", "Bearer "+i.apikey)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("SplitSDKVersion", iwm.metadata.SDKVersion)
	req.Header.Add("SplitSDKMachineIp", iwm.metadata.MachineIP)
	req.Header.Add("SplitSDKMachineName", iwm.metadata.MachineName)
	req.Header.Add("SplitSDKImpressionsMode", "optimized") // TODO(mredolatti): populate this correctly
	return req, nil
}

func (i *ImpressionsPipelineWorker) sendImpressionsToListener(b *impBatches) {
	for _, group := range b.groups {
		payload := make([]impressionlistener.ImpressionsForListener, 0, len(group.imps))
		for _, ti := range group.imps {
			var forTest impressionlistener.ImpressionsForListener
			forTest.TestName = ti.TestName
			forTest.KeyImpressions = make([]impressionlistener.ImpressionForListener, 0, len(ti.KeyImpressions))
			for _, ki := range ti.KeyImpressions {
				forTest.KeyImpressions = append(forTest.KeyImpressions, impressionlistener.ImpressionForListener{
					KeyName:      ki.KeyName,
					Treatment:    ki.Treatment,
					Time:         ki.Time,
					ChangeNumber: ki.ChangeNumber,
					Label:        ki.Label,
					BucketingKey: ki.BucketingKey,
					Pt:           ki.Pt,
					Properties:   ki.Properties,
				})
			}
			payload = append(payload, forTest)
		}

		metaCopy := group.metadata // we need a copy, since the current one will be reused, as soon as imps are posted to the BE
		if err := i.impListener.Submit(payload, &metaCopy); err != nil {
			i.logger.Error("error pushing impressions to listener: ", err.Error())
		}
	}
}

// This struct is used to maintain a slice of ready-to-post impression bulks, grouped by metadata,
// and partitioned by bulk size. The index is used to access the latest bulk being built for a specific metadata.
// This indirection helps avoid fetching the item from the map, updating it and storing it again which can be more expensive
// Because we're splitting the bulks by size as well, there can be more than one bulk for the same metadata.
// When the max size is reached, a new one is added to the list and the index is updated accordingly
type impBatches struct {
	groups impsWithMetaSlice
	index  metadataMap
	pool   impressionsMemoryPool
}

func newImpBatches(pool impressionsMemoryPool) *impBatches {
	toRet := &impBatches{
		groups: pool.acquireImpsWithMeta(),
		index:  pool.acquireMetadataMap(),
		pool:   pool,
	}
	return toRet
}

// This method releases only the remporary containers, the data is released after being consumed
func (i *impBatches) recycleContainer() {
	i.pool.releaseMetadataMap(i.index)
	i.pool.releaseImpsWithMeta(i.groups)
}

// add an impression to a bulk
// after identifying the correct bulk (or creating one if necessary), the call is forwarded
// to such structure. (see impsWithMetadata.add)
func (i *impBatches) add(queueObj *dtos.ImpressionQueueObject) {
	idx, ok := i.index[queueObj.Metadata]
	if !ok || i.groups[idx].count > defaultBulkSize {
		i.groups = append(i.groups, newImpsWithMetadata(i.pool, &queueObj.Metadata))
		idx = len(i.groups) - 1
		i.index[queueObj.Metadata] = idx
	}
	i.groups[idx].add(&queueObj.Impression)
}

type impsWithMetadata struct {
	pool     impressionsMemoryPool
	metadata dtos.Metadata
	imps     testImpressionsSlice
	count    int
	nindex   featureNameMap
}

func newImpsWithMetadata(pool impressionsMemoryPool, metadata *dtos.Metadata) impsWithMetadata {
	toRet := impsWithMetadata{
		pool:     pool,
		metadata: *metadata,
		imps:     pool.acquireTestImpressions(),
		nindex:   pool.acquireFeatureNameMap(),
	}
	for jdx := range toRet.imps {
		toRet.imps[jdx].KeyImpressions = pool.acquireKeyImpressions()
	}

	return toRet
}

func (s impsWithMetadata) recycle() {
	for idx := range s.imps {
		s.pool.releaseKeyImpressions(s.imps[idx].KeyImpressions)
	}
	s.pool.releaseFeatureNameMap(s.nindex)
	s.pool.releaseTestImpressions(s.imps)
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
			KeyImpressions: s.pool.acquireKeyImpressions(),
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
		Properties:   i.Properties,
	})
	s.count++
}

// Memory pool implementation to minimize allocations & gargabe collector pressure on the cpu

type keyImpressionsSlice = []dtos.ImpressionDTO
type metadataMap = map[dtos.Metadata]int
type featureNameMap = map[string]int
type testImpressionsSlice = []dtos.ImpressionsDTO
type impsWithMetaSlice = []impsWithMetadata

type impressionsMemoryPool interface {
	acquireKeyImpressions() keyImpressionsSlice
	releaseKeyImpressions(k keyImpressionsSlice)
	acquireMetadataMap() metadataMap
	releaseMetadataMap(m metadataMap)
	acquireFeatureNameMap() featureNameMap
	releaseFeatureNameMap(m featureNameMap)
	acquireTestImpressions() testImpressionsSlice
	releaseTestImpressions(t testImpressionsSlice)
	acquireImpsWithMeta() impsWithMetaSlice
	releaseImpsWithMeta(t impsWithMetaSlice)
}

type impressionsMemoryPoolImpl struct {
	keyImpressions  sync.Pool
	metadataMaps    sync.Pool
	featureNameMaps sync.Pool
	testImpressions sync.Pool
	impsWithMetas   sync.Pool
}

func newImpWorkerMemoryPool(fetchSize int, metadatasPerBulk int, featuresPerBulk int, impsPerFeature int) *impressionsMemoryPoolImpl {
	return &impressionsMemoryPoolImpl{
		keyImpressions:  sync.Pool{New: func() interface{} { return make(keyImpressionsSlice, 0, impsPerFeature) }},
		metadataMaps:    sync.Pool{New: func() interface{} { return make(metadataMap, metadatasPerBulk) }},
		featureNameMaps: sync.Pool{New: func() interface{} { return make(featureNameMap, featuresPerBulk) }},
		testImpressions: sync.Pool{New: func() interface{} { return make(testImpressionsSlice, 0, impsPerFeature) }},
		impsWithMetas:   sync.Pool{New: func() interface{} { return make(impsWithMetaSlice, 0, metadatasPerBulk) }},
	}
}

func (p *impressionsMemoryPoolImpl) acquireKeyImpressions() keyImpressionsSlice {
	return p.keyImpressions.Get().(keyImpressionsSlice)[:0]
}

func (p *impressionsMemoryPoolImpl) releaseKeyImpressions(k keyImpressionsSlice) {
	p.keyImpressions.Put(k)
}

func (p *impressionsMemoryPoolImpl) acquireMetadataMap() metadataMap {
	t := p.metadataMaps.Get().(metadataMap)
	// Clear the map
	// this is optimized by the go compiler by clearing all the map buckets.
	// no actual iteration is performed.
	for k := range t {
		delete(t, k)
	}
	return t
}

func (p *impressionsMemoryPoolImpl) releaseMetadataMap(m metadataMap) {
	p.metadataMaps.Put(m)
}

func (p *impressionsMemoryPoolImpl) acquireFeatureNameMap() featureNameMap {
	t := p.featureNameMaps.Get().(featureNameMap)
	// Clear the map
	// this is optimized by the go compiler by clearing all the map buckets.
	// no actual iteration is performed.
	for k := range t {
		delete(t, k)
	}
	return t
}

func (p *impressionsMemoryPoolImpl) releaseFeatureNameMap(m featureNameMap) {
	p.featureNameMaps.Put(m)
}

func (p *impressionsMemoryPoolImpl) acquireTestImpressions() testImpressionsSlice {
	return p.testImpressions.Get().(testImpressionsSlice)[:0]
}

func (p *impressionsMemoryPoolImpl) releaseTestImpressions(t testImpressionsSlice) {
	p.testImpressions.Put(t)
}

func (p *impressionsMemoryPoolImpl) acquireImpsWithMeta() impsWithMetaSlice {
	return p.impsWithMetas.Get().(impsWithMetaSlice)[:0]
}

func (p *impressionsMemoryPoolImpl) releaseImpsWithMeta(t impsWithMetaSlice) {
	p.impsWithMetas.Put(t)
}

var _ impressionsMemoryPool = (*impressionsMemoryPoolImpl)(nil)
var _ Worker = (*ImpressionsPipelineWorker)(nil)
