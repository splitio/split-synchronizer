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

const ruleBasedSegmentsChangesCollectionName = "RULE_BASED_SEGMENTS_CHANGES_COLLECTION"

// RBChangesCollection represents a collection of ChangesItem for rule-based segments
type RBChangesCollection struct {
	collection   CollectionWrapper
	changeNumber int64
	mutex        sync.RWMutex
}

// NewRBChangesCollection returns an instance of RBChangesCollection
func NewRBChangesCollection(db DBWrapper, logger logging.LoggerInterface) *RBChangesCollection {
	return &RBChangesCollection{
		collection:   &BoltDBCollectionWrapper{db: db, name: ruleBasedSegmentsChangesCollectionName, logger: logger},
		changeNumber: 0,
	}
}

// Update persists each item in its own transaction and advances the changeNumber only when every write succeeded.
// Errors are logged, not returned; use UpdateWithError when the caller must stop on a failed write.
func (c *RBChangesCollection) Update(toAdd []dtos.RuleBasedSegmentDTO, toRemove []dtos.RuleBasedSegmentDTO, cn int64) {
	if err := c.UpdateWithError(toAdd, toRemove, cn); err != nil {
		c.collection.Logger().Error("error persisting rule-based segments: ", err)
	}
}

// UpdateWithError is Update returning the joined write errors instead of logging them.
// The changeNumber advances only when no write failed.
func (c *RBChangesCollection) UpdateWithError(
	toAdd []dtos.RuleBasedSegmentDTO,
	toRemove []dtos.RuleBasedSegmentDTO,
	cn int64,
) error {
	items := NewChangesItems(len(toAdd) + len(toRemove))
	var errs []error
	process := func(rb *dtos.RuleBasedSegmentDTO) {
		asJSON, err := json.Marshal(rb)
		if err != nil {
			errs = append(errs, fmt.Errorf("encoding rule-based segment %q: %w", rb.Name, err))
			return
		}
		items.Append(ChangesItem{
			ChangeNumber: rb.ChangeNumber,
			Name:         rb.Name,
			Status:       rb.Status,
			JSON:         string(asJSON),
		})
	}

	for _, rb := range toAdd {
		process(&rb)
	}

	for _, rb := range toRemove {
		process(&rb)
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()
	for idx := range items.items {
		if err := c.collection.SaveAs([]byte(items.items[idx].Name), items.items[idx]); err != nil {
			errs = append(errs, fmt.Errorf("saving rule-based segment %q: %w", items.items[idx].Name, err))
		}
	}
	// The cursor only advances when every item was stored, so it never gets ahead of the stored data
	if len(errs) == 0 {
		c.changeNumber = cn
	}
	return errors.Join(errs...)
}

// FetchAll return a ChangesItem
func (c *RBChangesCollection) FetchAll() ([]dtos.RuleBasedSegmentDTO, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	items, err := c.collection.FetchAll()
	if err != nil {
		return nil, err
	}

	toReturn := make([]dtos.RuleBasedSegmentDTO, 0)

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

		var parsed dtos.RuleBasedSegmentDTO
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
func (c *RBChangesCollection) ChangeNumber() int64 {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.changeNumber
}

// SetChangeNumber seeds the cursor after stored data was loaded at startup
func (c *RBChangesCollection) SetChangeNumber(cn int64) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.changeNumber = cn
}
