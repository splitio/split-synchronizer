package collections

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/boltdb/bolt"
	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/storage/boltdb"
)

const segmentChangesCollectionName = "SEGMENT_CHANGES_COLLECTION"

// NewSegmentChangesCollection returns an instance of SegmentChangesCollection
func NewSegmentChangesCollection(dbb *bolt.DB) SegmentChangesCollection {
	baseCollection := boltdb.Collection{DB: dbb, Name: segmentChangesCollectionName}
	var sCollection = SegmentChangesCollection{Collection: baseCollection}
	return sCollection
}

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
}

// Add an item
func (c SegmentChangesCollection) Add(item *SegmentChangesItem) error {
	key := []byte(item.Name)
	err := c.Collection.SaveAs(key, item)
	return err
}

func (c SegmentChangesCollection) FetchAll() ([]*SegmentChangesItem, error) {
	items, err := c.Collection.FetchAll()
	if err != nil {
		return nil, err
	}
	fmt.Println(items)
	toReturn := make([]*SegmentChangesItem, 0)
	for _, v := range items {
		var decodeBuffer bytes.Buffer
		var q SegmentChangesItem

		decodeBuffer.Write(v)
		dec := gob.NewDecoder(&decodeBuffer)

		errq := dec.Decode(&q)
		if errq != nil {
			log.Error.Println("decode error:", errq)
			continue
		}
		toReturn = append(toReturn, &q)
	}

	//sort.Sort(toReturn)

	return toReturn, nil
}

// Fetch return a SegmentChangesItem
func (c SegmentChangesCollection) Fetch(name string) (*SegmentChangesItem, error) {
	key := []byte(name)
	item, err := c.Collection.FetchBy(key)
	if err != nil {
		return nil, err
	}

	if item == nil {
		return nil, nil
	}

	var decodeBuffer bytes.Buffer
	decodeBuffer.Write(item)
	dec := gob.NewDecoder(&decodeBuffer)

	var q SegmentChangesItem
	errq := dec.Decode(&q)
	if errq != nil {
		log.Error.Println("decode error:", errq)
	}
	return &q, nil
}
