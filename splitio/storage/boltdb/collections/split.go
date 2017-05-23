package collections

import (
	"bytes"
	"encoding/gob"
	"strconv"

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
	var sCollection = SplitChangesCollection{
		Collection:        baseCollection,
		NameIndex:         boltdb.Index{Name: nameIndexName, Collection: &baseCollection},
		ChangeNumberIndex: boltdb.Index{Name: changeNumberIndexName, Collection: &baseCollection},
		StatusIndex:       boltdb.Index{Name: statusIndexName, Collection: &baseCollection},
	}
	return sCollection
}

// SplitChangesItem represents an SplitChanges service response
type SplitChangesItem struct {
	id           uint64
	ChangeNumber int64  `json:"changeNumber"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	JSON         []byte
}

// SetID returns identifier
func (f *SplitChangesItem) SetID(id uint64) {
	f.id = id
}

// ID returns identifier
func (f *SplitChangesItem) ID() uint64 {
	return f.id
}

// SplitChangesCollection represents a collection of SplitChangesItem
type SplitChangesCollection struct {
	boltdb.Collection
	NameIndex         boltdb.Index
	ChangeNumberIndex boltdb.Index
	StatusIndex       boltdb.Index
}

// Add an item
func (c SplitChangesCollection) Add(item *SplitChangesItem) error {
	//Checking if the item already exists in Collection
	ids, errNameIdx := c.NameIndex.Retrieve([]byte(item.Name))
	if errNameIdx != nil {
		return errNameIdx
	} //Ends check
	if len(ids) == 0 { // item doesn't exist. So, add it!
		id, err := c.Collection.Save(item)
		if err != nil {
			return err
		}
		//Adding item to indexes
		c.NameIndex.Add([]byte(item.Name), id)
		c.ChangeNumberIndex.Add([]byte(strconv.Itoa(int(item.ChangeNumber))), id)
		c.StatusIndex.Add([]byte(item.Status), id)
		return nil
	}

	// item already exist. Update it!
	id := ids[0] //must be only 1 item with the same name.
	item.SetID(id)
	errUpdate := c.Collection.Update(item)
	if errUpdate != nil {
		return errUpdate
	}

	return nil
}

// Add an item
/*func (c SplitChangesCollection) Add(item *SplitChangesItem) error {
	key := []byte(strconv.Itoa(int(item.Since)))
	err := c.Collection.SaveAs(key, item)
	return err
}*/

// Fetch return a SplitChangesItem
func (c SplitChangesCollection) Fetch(since int64) (*SplitChangesItem, error) {
	key := []byte(strconv.Itoa(int(since)))
	item, err := c.Collection.FetchBy(key)
	if err != nil {
		return nil, err
	}

	var decodeBuffer bytes.Buffer
	decodeBuffer.Write(item)
	dec := gob.NewDecoder(&decodeBuffer)

	var q SplitChangesItem
	errq := dec.Decode(&q)
	if errq != nil {
		log.Error.Println("decode error:", errq)
	}
	return &q, nil
}
