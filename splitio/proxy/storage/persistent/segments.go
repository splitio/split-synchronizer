package persistent

import (
	"bytes"
	"encoding/gob"
	"errors"
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

func (c *SegmentChangesCollectionImpl) initialize() error {
	all, err := c.FetchAll()
	if err != nil {
		// TODO
		return err
	}

	tills := make(map[string]int64)
	for _, segment := range all {
		var cn int64 // 0
		for _, key := range segment.Keys {
			if key.ChangeNumber > cn {
				cn = key.ChangeNumber
			}
		}
		tills[segment.Name] = cn
	}

	c.segmentsTill = tills
	return nil
}

// NewSegmentChangesCollection returns an instance of SegmentChangesCollection
func NewSegmentChangesCollection(db DBWrapper, logger logging.LoggerInterface) *SegmentChangesCollectionImpl {
	collection := &SegmentChangesCollectionImpl{
		collection:   &BoltDBCollectionWrapper{db: db, name: segmentChangesCollectionName, logger: logger},
		segmentsTill: make(map[string]int64, 0),
		logger:       logger,
	}
	collection.initialize()
	return collection
}

// Update persists a segmentChanges update
func (c *SegmentChangesCollectionImpl) Update(name string, toAdd *set.ThreadUnsafeSet, toRemove *set.ThreadUnsafeSet, cn int64) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	segmentItem, err := c.fetch(name)
	if err != nil && !errors.Is(err, ErrorKeyNotFound) && !errors.Is(err, ErrorBucketNotFound) {
		// The record cannot be read and applying a delta on top of it is not possible. Drop it and forget the change
		// number so the next fetch starts from -1 and rebuilds the segment from a full snapshot. This update is not applied.
		c.logger.Error(fmt.Sprintf("segment %s is unreadable, dropping it so it is rebuilt from a full snapshot: %s", name, err))
		if delErr := c.collection.Delete([]byte(name)); delErr != nil {
			return fmt.Errorf("error dropping unreadable segment %s: %w (read error: %v)", name, delErr, err)
		}
		delete(c.segmentsTill, name)
		return fmt.Errorf("error reading segment %s before update, it will be rebuilt: %w", name, err)
	}
	if segmentItem == nil {
		segmentItem = &SegmentChangesItem{}
		segmentItem.Name = name
		segmentItem.Keys = make(map[string]SegmentKey, toAdd.Size()+toRemove.Size())
	}

	for _, removedKey := range toRemove.List() {
		strKey, ok := removedKey.(string)
		if !ok {
			c.logger.Error(fmt.Sprintf("skipping non-string key when updating segment %s: %T::%+v", name, removedKey, removedKey))
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
			c.logger.Error(fmt.Sprintf("skipping non-string key when updating segment %s: %T::%+v", name, addedKey, addedKey))
			continue
		}
		c.logger.Debug("Adding", strKey, "in", name)
		segmentItem.Keys[strKey] = SegmentKey{
			Name:         strKey,
			Removed:      false,
			ChangeNumber: cn,
		}
	}

	err = c.collection.SaveAs([]byte(name), segmentItem)
	if err != nil {
		return fmt.Errorf("error saving segment changes to bolt: %w", err)
	}
	c.segmentsTill[name] = cn
	return nil
}

// Fetch return a SegmentChangesItem
func (c *SegmentChangesCollectionImpl) Fetch(name string) (*SegmentChangesItem, error) {
	c.mutex.RLock()
	item, err := c.collection.FetchBy([]byte(name))
	c.mutex.RUnlock()
	if err != nil {
		return nil, err
	}
	return decodeSegmentItem(name, item)
}

// fetch must be called with c.mutex held (read or write)
func (c *SegmentChangesCollectionImpl) fetch(name string) (*SegmentChangesItem, error) {
	item, err := c.collection.FetchBy([]byte(name))
	if err != nil {
		return nil, err
	}
	return decodeSegmentItem(name, item)
}

// decodeSegmentItem decodes gob bytes returned by FetchBy. It does not touch shared state.
func decodeSegmentItem(name string, item []byte) (*SegmentChangesItem, error) {
	var decodeBuffer bytes.Buffer
	decodeBuffer.Write(item)
	dec := gob.NewDecoder(&decodeBuffer)

	var q SegmentChangesItem
	errq := dec.Decode(&q)
	if errq != nil {
		return nil, fmt.Errorf("decoding segment %s: %w", name, errq)
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

// Names returns a snapshot copy of the segment names currently stored
func (c *SegmentChangesCollectionImpl) Names() []string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	names := make([]string, 0, len(c.segmentsTill))
	for name := range c.segmentsTill {
		names = append(names, name)
	}
	return names
}

var _ SegmentChangesCollection = (*SegmentChangesCollectionImpl)(nil)
