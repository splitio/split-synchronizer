package persistent

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-toolkit/v5/logging"
)

const splitChangesCollectionName = "SPLIT_CHANGES_COLLECTION"

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

// Update persists each item in its own transaction and advances the changeNumber only when every write succeeded.
// Errors are logged, not returned; use UpdateWithError when the caller must stop on a failed write.
func (c *SplitChangesCollection) Update(toAdd []dtos.SplitDTO, toRemove []dtos.SplitDTO, cn int64) {
	if err := c.UpdateWithError(toAdd, toRemove, cn); err != nil {
		c.collection.Logger().Error("error persisting feature flags: ", err)
	}
}

// UpdateWithError is Update returning the joined write errors instead of logging them.
// The changeNumber advances only when no write failed.
func (c *SplitChangesCollection) UpdateWithError(toAdd []dtos.SplitDTO, toRemove []dtos.SplitDTO, cn int64) error {
	items := NewChangesItems(len(toAdd) + len(toRemove))
	var errs []error
	process := func(split *dtos.SplitDTO) {
		asJSON, err := json.Marshal(split)
		if err != nil {
			errs = append(errs, fmt.Errorf("encoding feature flag %q: %w", split.Name, err))
			return
		}
		items.Append(ChangesItem{
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
	for idx := range items.items {
		if err := c.collection.SaveAs([]byte(items.items[idx].Name), items.items[idx]); err != nil {
			errs = append(errs, fmt.Errorf("saving feature flag %q: %w", items.items[idx].Name, err))
		}
	}
	// The cursor only advances when every item was stored, so it never gets ahead of the stored data
	if len(errs) == 0 {
		c.changeNumber = cn
	}
	return errors.Join(errs...)
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
		var q ChangesItem
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

// SetChangeNumber seeds the cursor after stored data was loaded at startup
func (c *SplitChangesCollection) SetChangeNumber(cn int64) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.changeNumber = cn
}
