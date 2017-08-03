package collections

import (
	"bytes"
	"encoding/gob"
	"sort"

	"github.com/boltdb/bolt"
	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/storage/boltdb"
)

const splitChangesCollectionName = "SPLIT_CHANGES_COLLECTION"
const changeNumberIndexName = "SPLIT_CHANGES_NUMBER_IDX"
const statusIndexName = "SPLIT_STATUS_IDX"
const nameIndexName = "SPLIT_NAME_IDX"

// NewSplitChangesCollection returns an instance of SplitChangesCollection
func NewSplitChangesCollection(dbb *bolt.DB) SplitChangesCollection {
	baseCollection := boltdb.Collection{DB: dbb, Name: splitChangesCollectionName}
	sCollection := SplitChangesCollection{Collection: baseCollection}
	return sCollection
}

// SplitChangesItem represents an SplitChanges service response
type SplitChangesItem struct {
	id           uint64
	ChangeNumber int64  `json:"changeNumber"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	JSON         string
}

// SetID returns identifier
func (f *SplitChangesItem) SetID(id uint64) {
	f.id = id
}

// ID returns identifier
func (f *SplitChangesItem) ID() uint64 {
	return f.id
}

//----------------------------------------------------

// SplitsChangesItems Sortable list
type SplitsChangesItems []*SplitChangesItem

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
}

// Add an item
func (c SplitChangesCollection) Add(item *SplitChangesItem) error {
	key := []byte(item.Name)
	err := c.Collection.SaveAs(key, item)
	return err
}

// FetchAll return a SplitChangesItem
func (c SplitChangesCollection) FetchAll() (SplitsChangesItems, error) {

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
			log.Error.Println("decode error:", errq, "|", string(v))
			continue
		}
		toReturn = append(toReturn, &q)
	}

	sort.Sort(toReturn)

	return toReturn, nil
}
