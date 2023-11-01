package persistent

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sync"

	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"
)

const segmentChangesCollectionName = "SEGMENT_CHANGES_COLLECTION"

// SegmentKey represents a segment key data
type SegmentKey struct {
	Name         string
	ChangeNumber int64
	Removed      bool
}

// SegmentChangesItem represents an SplitChanges service response
type SegmentChangesItem struct {
	Name string
	Keys map[string]SegmentKey
}

type SegmentChangesCollection interface {
	Update(name string, toAdd *set.ThreadUnsafeSet, toRemove *set.ThreadUnsafeSet, cn int64) error
	Fetch(name string) (*SegmentChangesItem, error)
	ChangeNumber(segment string) int64
	SetChangeNumber(segment string, cn int64)
}

// SegmentChangesCollectionImpl represents a collection of SplitChangesItem
type SegmentChangesCollectionImpl struct {
	collection   CollectionWrapper
	segmentsTill map[string]int64
	logger       logging.LoggerInterface
	mutex        sync.RWMutex
}

// NewSegmentChangesCollection returns an instance of SegmentChangesCollection
func NewSegmentChangesCollection(db DBWrapper, logger logging.LoggerInterface) *SegmentChangesCollectionImpl {
	return &SegmentChangesCollectionImpl{
		collection:   &BoltDBCollectionWrapper{db: db, name: segmentChangesCollectionName, logger: logger},
		segmentsTill: make(map[string]int64, 0),
		logger:       logger,
	}
}

// Update persists a segmentChanges update
func (c *SegmentChangesCollectionImpl) Update(name string, toAdd *set.ThreadUnsafeSet, toRemove *set.ThreadUnsafeSet, cn int64) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Error is most likely that the segment isn't yet cached.
	// In the worst case, the update will fail later in the method
	segmentItem, _ := c.fetch(name)
	if segmentItem == nil {
		segmentItem = &SegmentChangesItem{}
		segmentItem.Name = name
		segmentItem.Keys = make(map[string]SegmentKey, toAdd.Size()+toRemove.Size())
	}

	for _, removedKey := range toRemove.List() {
		strKey, ok := removedKey.(string)
		if !ok {
			c.logger.Error(fmt.Sprintf("skipping non-string key when updating segment %s: %+v", name, strKey))
			continue
		}
		c.logger.Debug("Removing", strKey, "from", name)
		segmentItem.Keys[strKey] = SegmentKey{
			Name:         strKey,
			Removed:      true,
			ChangeNumber: cn,
		}

	}

	for _, addedKey := range toAdd.List() {
		strKey, ok := addedKey.(string)
		if !ok {
			c.logger.Error(fmt.Sprintf("skipping non-string key when updating segment %s: %+v", name, strKey))
			continue
		}
		c.logger.Debug("Adding", strKey, "in", name)
		segmentItem.Keys[strKey] = SegmentKey{
			Name:         strKey,
			Removed:      false,
			ChangeNumber: cn,
		}
	}

	err := c.collection.SaveAs([]byte(name), segmentItem)
	if err != nil {
		return fmt.Errorf("error saving segment changes to bolt: %w", err)
	}
	c.segmentsTill[name] = cn
	return nil
}

// Fetch return a SegmentChangesItem
func (c *SegmentChangesCollectionImpl) Fetch(name string) (*SegmentChangesItem, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.fetch(name)
}

func (c *SegmentChangesCollectionImpl) fetch(name string) (*SegmentChangesItem, error) {
	item, err := c.collection.FetchBy([]byte(name))
	if err != nil {
		return nil, err
	}

	var decodeBuffer bytes.Buffer
	decodeBuffer.Write(item)
	dec := gob.NewDecoder(&decodeBuffer)

	var q SegmentChangesItem
	errq := dec.Decode(&q)
	if errq != nil {
		c.collection.Logger().Error("decode error:", errq)
	}
	return &q, nil
}

// FetchAll return a list of SegmentChangesItem
func (c *SegmentChangesCollectionImpl) FetchAll() ([]SegmentChangesItem, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	items, err := c.collection.FetchAll()
	if err != nil {
		return nil, err
	}

	var toReturn = make([]SegmentChangesItem, 0)
	for _, item := range items {
		if item == nil {
			continue
		}

		var q SegmentChangesItem
		var decodeBuffer bytes.Buffer
		decodeBuffer.Write(item)
		errq := gob.NewDecoder(&decodeBuffer).Decode(&q)
		if errq != nil {
			c.collection.Logger().Error("decode error:", errq)
			continue
		}

		toReturn = append(toReturn, q)
	}

	return toReturn, nil
}

// ChangeNumber returns changeNumber
func (c *SegmentChangesCollectionImpl) ChangeNumber(segment string) int64 {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	value, exists := c.segmentsTill[segment]
	if exists {
		return value
	}
	return -1
}

// SetChangeNumber returns changeNumber
func (c *SegmentChangesCollectionImpl) SetChangeNumber(segment string, cn int64) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.segmentsTill[segment] = cn
}

var _ SegmentChangesCollection = (*SegmentChangesCollectionImpl)(nil)
