package boltdb

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"

	"github.com/boltdb/bolt"
	"github.com/splitio/go-agent/log"
)

// Collection sets
type Collection struct {
	//DB is a pointer to bolt DB
	DB *bolt.DB
	//Name is the collection name used for bucket name
	Name string
}

// SaveAs saves an item into collection under key parameter
func (c Collection) SaveAs(key []byte, item interface{}) error {
	// Insert value in DB
	return c.DB.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(c.Name))
		if err != nil {
			return err
		}

		var encodeBuffer bytes.Buffer
		enc := gob.NewEncoder(&encodeBuffer)
		enc.Encode(item)

		err = bucket.Put(key, encodeBuffer.Bytes())
		if err != nil {
			return err
		}
		return nil
	})
}

// Save an item into collection setting autoincrement ID
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
		log.Error.Println(updateError)
		return 0, updateError
	}

	return id, nil
}

// Update an item into collection with current item ID
func (c Collection) Update(item CollectionItem) error {
	if !(item.ID() > 0) {
		log.Error.Println("Trying to update an item with ID 0")
		return errors.New("Invalid ID, it must be grater than zero")
	}

	// Insert value in DB
	updateError := c.DB.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(c.Name))
		if err != nil {
			return err
		}

		id := item.ID()

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
		log.Error.Println(updateError)
		return updateError
	}

	return nil
}

// Fetch returns an item from collection
func (c Collection) Fetch(id uint64) ([]byte, error) {
	var item []byte
	err := c.DB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(c.Name))
		if bucket == nil {
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

// FetchBy returns an item from collection given a key
func (c Collection) FetchBy(key []byte) ([]byte, error) {
	var item []byte
	err := c.DB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(c.Name))
		if bucket == nil {
			return fmt.Errorf("Bucket not found! %s", c.Name)
		}

		item = bucket.Get(key)

		return nil
	})

	if err != nil {
		return nil, err
	}

	return item, nil
}
