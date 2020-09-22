package collections

import (
	"bytes"
	"encoding/gob"
	"sync"

	"github.com/boltdb/bolt"
	"github.com/splitio/split-synchronizer/v4/log"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb"
)

const segmentChangesCollectionName = "SEGMENT_CHANGES_COLLECTION"

// NewSegmentChangesCollection returns an instance of SegmentChangesCollection
func NewSegmentChangesCollection(dbb *bolt.DB) SegmentChangesCollection {
	baseCollection := boltdb.Collection{DB: dbb, Name: segmentChangesCollectionName}
	var sCollection = SegmentChangesCollection{
		Collection:        baseCollection,
		mutexSegmentsTill: &sync.RWMutex{},
		segmentsTill:      make(map[string]int64, 0),
	}
	return sCollection
}

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

// SegmentChangesCollection represents a collection of SplitChangesItem
type SegmentChangesCollection struct {
	boltdb.Collection
	mutexSegmentsTill *sync.RWMutex
	segmentsTill      map[string]int64
}

// Add an item
func (c *SegmentChangesCollection) Add(item *SegmentChangesItem) error {
	key := []byte(item.Name)
	err := c.Collection.SaveAs(key, item)
	return err
}

// Fetch return a SegmentChangesItem
func (c *SegmentChangesCollection) Fetch(name string) (*SegmentChangesItem, error) {
	key := []byte(name)
	item, err := c.Collection.FetchBy(key)
	if err != nil {
		return nil, err
	}

	if item == nil || len(item) <= 0 {
		return nil, nil
	}

	var decodeBuffer bytes.Buffer
	decodeBuffer.Write(item)
	dec := gob.NewDecoder(&decodeBuffer)

	var q SegmentChangesItem
	errq := dec.Decode(&q)
	if errq != nil {
		log.Instance.Error("decode error:", errq)
	}
	return &q, nil
}

// FetchAll return a list of SegmentChangesItem
func (c *SegmentChangesCollection) FetchAll() ([]SegmentChangesItem, error) {
	items, err := c.Collection.FetchAll()
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
			log.Instance.Error("decode error:", errq)
			continue
		}

		toReturn = append(toReturn, q)
	}

	return toReturn, nil
}

// ChangeNumber returns changeNumber
func (c *SegmentChangesCollection) ChangeNumber(segment string) int64 {
	c.mutexSegmentsTill.RLock()
	defer c.mutexSegmentsTill.RUnlock()
	value, exists := c.segmentsTill[segment]
	if exists {
		return value
	}
	return -1
}

// SetChangeNumber sets changeNumber
func (c *SegmentChangesCollection) SetChangeNumber(segment string, since int64) {
	c.mutexSegmentsTill.Lock()
	defer c.mutexSegmentsTill.Unlock()
	c.segmentsTill[segment] = since
}
