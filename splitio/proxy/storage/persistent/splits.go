package persistent

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"sync"

	"github.com/splitio/go-split-commons/v6/dtos"
	"github.com/splitio/go-toolkit/v5/logging"
)

const splitChangesCollectionName = "SPLIT_CHANGES_COLLECTION"

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
	collection   CollectionWrapper
	changeNumber int64
	mutex        sync.RWMutex
}

// NewSplitChangesCollection returns an instance of SplitChangesCollection
func NewSplitChangesCollection(db DBWrapper, logger logging.LoggerInterface) *SplitChangesCollection {
	return &SplitChangesCollection{
		collection:   &BoltDBCollectionWrapper{db: db, name: splitChangesCollectionName, logger: logger},
		changeNumber: 0,
	}
}

// Update processes a set of feature flag changes items + a changeNumber bump atomically
func (c *SplitChangesCollection) Update(toAdd []dtos.SplitDTO, toRemove []dtos.SplitDTO, cn int64) {

	items := make(SplitsChangesItems, 0, len(toAdd)+len(toRemove))
	process := func(split *dtos.SplitDTO) {
		asJSON, err := json.Marshal(split)
		if err != nil {
			// This should not happen unless the DTO class is broken
			return
		}
		items = append(items, SplitChangesItem{
			ChangeNumber: split.ChangeNumber,
			Name:         split.Name,
			Status:       split.Status,
			JSON:         string(asJSON),
		})
	}

	for _, split := range toAdd {
		process(&split)
	}

	for _, split := range toRemove {
		process(&split)
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()
	for idx := range items {
		err := c.collection.SaveAs([]byte(items[idx].Name), items[idx])
		if err != nil {
			// TODO(mredolatti): log
		}
	}
	c.changeNumber = cn
}

// FetchAll return a SplitChangesItem
func (c *SplitChangesCollection) FetchAll() ([]dtos.SplitDTO, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	items, err := c.collection.FetchAll()
	if err != nil {
		return nil, err
	}

	toReturn := make([]dtos.SplitDTO, 0)

	var decodeBuffer bytes.Buffer
	for _, v := range items {
		var q SplitChangesItem
		// resets buffer data
		decodeBuffer.Reset()
		decodeBuffer.Write(v)
		dec := gob.NewDecoder(&decodeBuffer)

		errq := dec.Decode(&q)
		if errq != nil {
			c.collection.Logger().Error("decode error:", errq, "|", string(v))
			continue
		}

		var parsed dtos.SplitDTO
		err := json.Unmarshal([]byte(q.JSON), &parsed)
		if err != nil {
			c.collection.Logger().Error("error decoding feature flag fetched from db: ", err, "|", q.JSON)
			continue
		}
		toReturn = append(toReturn, parsed)
	}

	return toReturn, nil
}

// ChangeNumber returns changeNumber
func (c *SplitChangesCollection) ChangeNumber() int64 {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.changeNumber
}
