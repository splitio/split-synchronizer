package boltdb

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"

	"github.com/boltdb/bolt"
)

// Index index based on date
type Index struct {
	Name string

	Collection *Collection
}

// Add value into index
func (idx *Index) Add(key []byte, id uint64) error {
	err := idx.Collection.DB.Update(func(tx *bolt.Tx) error {

		root := tx.Bucket([]byte(idx.Collection.Name))

		// Setup the index bucket.
		bkt, err := root.CreateBucketIfNotExists([]byte(idx.Name))
		if err != nil {
			return err
		}

		//Get
		currentItems := bkt.Get(key)
		var items map[uint64]struct{}
		if currentItems == nil {
			items = make(map[uint64]struct{})
		} else {
			gob.NewDecoder(bytes.NewReader(currentItems)).Decode(&items)
		}
		items[id] = struct{}{}
		var buff bytes.Buffer
		gob.NewEncoder(&buff).Encode(items)
		bkt.Put(key, buff.Bytes())

		return nil
	})

	return err
}

// Retrieve items in key
func (idx *Index) Retrieve(idxKey []byte) ([]uint64, error) {
	var toReturn = make([]uint64, 0)
	err := idx.Collection.DB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(idx.Collection.Name))
		if bucket == nil {
			return fmt.Errorf("Bucket %s not found!", idx.Collection.Name)
		}

		bktIdx := bucket.Bucket([]byte(idx.Name))
		currentItems := bktIdx.Get(idxKey)
		if currentItems == nil {
			return errors.New("Empty Index")
		}

		var items map[uint64]struct{}

		// items in index
		gob.NewDecoder(bytes.NewReader(currentItems)).Decode(&items)
		for key := range items {
			toReturn = append(toReturn, key)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return toReturn, nil
}

// Between into dates
func (idx *Index) Between(min []byte, max []byte) ([]uint64, error) {
	var toReturn = make([]uint64, 0)
	err := idx.Collection.DB.View(func(tx *bolt.Tx) error {

		root := tx.Bucket([]byte(idx.Collection.Name))
		bkt := root.Bucket([]byte(idx.Name))
		if bkt == nil {
			return nil
		}
		c := bkt.Cursor()

		for k, v := c.Seek(min); k != nil && bytes.Compare(k, max) <= 0; k, v = c.Next() {
			var items map[uint64]struct{}
			gob.NewDecoder(bytes.NewReader(v)).Decode(&items)
			for key := range items {
				toReturn = append(toReturn, key)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return toReturn, nil
}
