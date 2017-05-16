package boltdb

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/boltdb/bolt"
)

// Collection sets
type Collection struct {
	//DB is a pointer to bolt DB
	DB *bolt.DB
	//Name is the collection name used for bucket name
	Name string
}

// Save an item into collection
func (c Collection) Save(item CollectionItem) (uint64, error) {
	var id uint64
	// Insert value in DB
	updateError := c.DB.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(c.Name))
		if err != nil {
			return err
		}

		id, _ = bucket.NextSequence()
		item.SetID(id)

		var encodeBuffer bytes.Buffer
		enc := gob.NewEncoder(&encodeBuffer)
		enc.Encode(item)

		err = bucket.Put(itob(id), encodeBuffer.Bytes())
		if err != nil {
			return err
		}
		return nil
	})

	if updateError != nil {
		// TODO Log the error
		return 0, updateError
	}

	return id, nil
}

// Fetch returns an item from collection
func (c Collection) Fetch(id uint64) ([]byte, error) {
	var item []byte
	err := c.DB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(c.Name))
		if bucket == nil {
			// TODO add log error
			return fmt.Errorf("Bucket not found! %s", c.Name)
		}

		item = bucket.Get(itob(id))

		return nil
	})

	if err != nil {
		return nil, err
	}

	return item, nil
}
