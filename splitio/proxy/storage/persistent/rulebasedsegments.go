package persistent

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"sync"

	"github.com/splitio/go-split-commons/v8/dtos"
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

// Update processes a set of rule based changes items + a changeNumber bump atomically
func (c *RBChangesCollection) Update(toAdd []dtos.RuleBasedSegmentDTO, toRemove []dtos.RuleBasedSegmentDTO, cn int64) {
	items := NewChangesItems(len(toAdd) + len(toRemove))
	process := func(rb *dtos.RuleBasedSegmentDTO) {
		asJSON, err := json.Marshal(rb)
		if err != nil {
			// This should not happen unless the DTO class is broken
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
		err := c.collection.SaveAs([]byte(items.items[idx].Name), items.items[idx])
		if err != nil {
			// TODO(mredolatti): log
		}
	}
	c.changeNumber = cn
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
