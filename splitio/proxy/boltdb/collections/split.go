package collections

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"sort"
	"sync"

	"github.com/boltdb/bolt"
	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/split-synchronizer/v4/log"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb"
)

const splitChangesCollectionName = "SPLIT_CHANGES_COLLECTION"

// NewSplitChangesCollection returns an instance of SplitChangesCollection
func NewSplitChangesCollection(dbb *bolt.DB) SplitChangesCollection {
	baseCollection := boltdb.Collection{DB: dbb, Name: splitChangesCollectionName}
	sCollection := SplitChangesCollection{
		Collection:   baseCollection,
		changeNumber: 0,
		mutexTill:    &sync.RWMutex{},
	}
	return sCollection
}

// SplitChangesItem represents an SplitChanges service response
type SplitChangesItem struct {
	ChangeNumber int64  `json:"changeNumber"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	JSON         string
}

// SplitsChangesItems Sortable list
type SplitsChangesItems []SplitChangesItem

func (slice SplitsChangesItems) Len() int {
	return len(slice)
}

func (slice SplitsChangesItems) Less(i, j int) bool {
	return slice[i].ChangeNumber > slice[j].ChangeNumber
}

func (slice SplitsChangesItems) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

//----------------------------------------------------

// SplitChangesCollection represents a collection of SplitChangesItem
type SplitChangesCollection struct {
	boltdb.Collection
	mutexTill    *sync.RWMutex
	changeNumber int64
}

// Delete an item
func (c *SplitChangesCollection) Delete(item *SplitChangesItem) error {
	key := []byte(item.Name)
	err := c.Collection.Delete(key)
	return err
}

// Add an item
func (c *SplitChangesCollection) Add(item *SplitChangesItem) error {
	key := []byte(item.Name)

	err := c.Collection.SaveAs(key, item)
	return err
}

// FetchAll return a SplitChangesItem
func (c *SplitChangesCollection) FetchAll() (SplitsChangesItems, error) {

	items, err := c.Collection.FetchAll()
	if err != nil {
		return nil, err
	}

	toReturn := make(SplitsChangesItems, 0)

	var decodeBuffer bytes.Buffer
	for _, v := range items {
		var q SplitChangesItem
		// resets buffer data
		decodeBuffer.Reset()
		decodeBuffer.Write(v)
		dec := gob.NewDecoder(&decodeBuffer)

		errq := dec.Decode(&q)
		if errq != nil {
			log.Instance.Error("decode error:", errq, "|", string(v))
			continue
		}
		toReturn = append(toReturn, q)
	}

	sort.Sort(toReturn)

	return toReturn, nil
}

// ChangeNumber returns changeNumber
func (c *SplitChangesCollection) ChangeNumber() int64 {
	c.mutexTill.RLock()
	defer c.mutexTill.RUnlock()
	return c.changeNumber
}

// SetChangeNumber sets changeNumber
func (c *SplitChangesCollection) SetChangeNumber(since int64) {
	c.mutexTill.Lock()
	defer c.mutexTill.Unlock()
	c.changeNumber = since
}

// SegmentNames returns segments
func (c *SplitChangesCollection) SegmentNames() *set.ThreadUnsafeSet {
	segments := set.NewSet()
	rawSplits, _ := c.FetchAll()

	for _, rawSplit := range rawSplits {
		var split *dtos.SplitDTO
		err := json.Unmarshal([]byte(rawSplit.JSON), &split)
		if err != nil {
			continue
		}
		for _, condition := range split.Conditions {
			for _, matcher := range condition.MatcherGroup.Matchers {
				if matcher.UserDefinedSegment != nil {
					segments.Add(matcher.UserDefinedSegment.SegmentName)
				}

			}
		}
	}
	return segments
}
