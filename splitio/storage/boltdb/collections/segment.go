package collections

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/boltdb/bolt"
	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/storage/boltdb"
)

const segmentChangesCollectionName = "SEGMENT_CHANGES_COLLECTION"

// NewSegmentChangesCollection returns an instance of SegmentChangesCollection
func NewSegmentChangesCollection(dbb *bolt.DB) SegmentChangesCollection {
	baseCollection := boltdb.Collection{DB: dbb, Name: segmentChangesCollectionName}
	var sCollection = SegmentChangesCollection{Collection: baseCollection}
	return sCollection
}

type SegmentKey struct {
	Name         string
	ChangeNumber int64
	Removed      bool
}

//------------ Chunked Collection ------------------
const sccBucketName = "SEGMENT_%s"
const sccChunkBucketName = "CHUNK_%d"
const sccChunkSize = 100000

func chunkSlice(list []SegmentKey) [][]SegmentKey {
	var divided [][]SegmentKey

	for i := 0; i < len(list); i += sccChunkSize {
		end := i + sccChunkSize

		if end > len(list) {
			end = len(list)
		}

		divided = append(divided, list[i:end])
	}

	return divided
}

func NewSegmentChunkedCollection(dbb *bolt.DB, name string) SegmentChunkedCollection {
	scc := SegmentChunkedCollection{DB: dbb, Name: name}
	return scc
}

// SegmentCollection segment buncket
type SegmentChunkedCollection struct {
	DB   *bolt.DB
	Name string
}

func putChunkedKeysIntoNewBuckets(start int, root *bolt.Bucket, chunkedList [][]SegmentKey) error {

	for i, chunk := range chunkedList {
		bucketName := []byte(fmt.Sprintf(sccChunkBucketName, start+i))
		bucket, errc := root.CreateBucketIfNotExists(bucketName)
		if errc != nil {
			log.Error.Println(errc)
			return errc
		}

		errp := putKeysIntoBucket(root, bucket, chunk)
		if errp != nil {
			log.Error.Println(errp)
			return errp
		}
	}
	return nil
}

func putKeysIntoBucket(root *bolt.Bucket, b *bolt.Bucket, items []SegmentKey) error {
	for _, k := range items {
		var err error
		var encodeBuffer bytes.Buffer
		gob.NewEncoder(&encodeBuffer).Encode(k)

		bktName := keyExistsIn(root, []byte(k.Name))
		if bktName != nil { //Update existing key
			err = root.Bucket(bktName).Put([]byte(k.Name), encodeBuffer.Bytes())
		} else { //Add new key
			err = b.Put([]byte(k.Name), encodeBuffer.Bytes())
		}

		if err != nil {
			return err
		}
	}
	return nil
}

func keyExistsIn(root *bolt.Bucket, key []byte) []byte {
	var container []byte = nil
	root.ForEach(func(k []byte, v []byte) error {
		if k != nil && v == nil { // it is bucket
			if root.Bucket(k).Get(key) != nil {
				container = k
			}
		}
		return nil
	})

	return container
}

// Add an item
func (c *SegmentChunkedCollection) Add(items []SegmentKey) error {

	err := c.DB.Update(func(tx *bolt.Tx) error {

		// fetching/creating parent bucket
		root, errb := tx.CreateBucketIfNotExists([]byte(fmt.Sprintf(sccBucketName, c.Name)))
		if errb != nil {
			log.Error.Println("Error getting segment bucket", errb)
			return errb
		}

		//fetching chunk bucket to add impressions
		var lastChunkBucket = root.Stats().BucketN - 1
		var lastChunkBucketName []byte
		var chunkBucket *bolt.Bucket

		if lastChunkBucket == 0 { //First add, creates bucket named CHUNK_1

			toAddInNewBuckets := chunkSlice(items)
			putChunkedKeysIntoNewBuckets(lastChunkBucket+1, root, toAddInNewBuckets)

		} else {
			lastChunkBucketName = []byte(fmt.Sprintf(sccChunkBucketName, lastChunkBucket))
			chunkBucket = root.Bucket(lastChunkBucketName)
			var remainingKeys = sccChunkSize - chunkBucket.Stats().KeyN
			if remainingKeys > 0 { //there are free slots to add items
				if remainingKeys >= len(items) {
					putKeysIntoBucket(root, chunkBucket, items)
				} else {
					toAddFirst := items[:remainingKeys]
					putKeysIntoBucket(root, chunkBucket, toAddFirst)

					toAddInNewBuckets := chunkSlice(items[remainingKeys:])
					putChunkedKeysIntoNewBuckets(lastChunkBucket+1, root, toAddInNewBuckets)
				}
			} else {
				toAddInNewBuckets := chunkSlice(items)
				putChunkedKeysIntoNewBuckets(lastChunkBucket+1, root, toAddInNewBuckets)
			}
		}

		/*root.ForEach(func(k []byte, v []byte) error {
			if k != nil && v == nil { // it is bucket
				if root.Bucket(k).Stats().KeyN < 100000 {
					lastChunkBucket = k //saving chunk bucket name
				}
			}
			return nil
		})*/

		return nil
	})

	return err
}

//--------------------------------------------------
//--------------------------------------------------

// SegmentChangesItem represents an SplitChanges service response
type SegmentChangesItem struct {
	Name string
	Keys map[string]SegmentKey
}

// SegmentChangesCollection represents a collection of SplitChangesItem
type SegmentChangesCollection struct {
	boltdb.Collection
}

// Add an item
func (c SegmentChangesCollection) Add(item *SegmentChangesItem) error {
	key := []byte(item.Name)
	err := c.Collection.SaveAs(key, item)
	return err
}

func (c SegmentChangesCollection) FetchAll() ([]*SegmentChangesItem, error) {
	items, err := c.Collection.FetchAll()
	if err != nil {
		return nil, err
	}
	fmt.Println(items)
	toReturn := make([]*SegmentChangesItem, 0)
	for _, v := range items {
		var decodeBuffer bytes.Buffer
		var q SegmentChangesItem

		decodeBuffer.Write(v)
		dec := gob.NewDecoder(&decodeBuffer)

		errq := dec.Decode(&q)
		if errq != nil {
			log.Error.Println("decode error:", errq)
			continue
		}
		toReturn = append(toReturn, &q)
	}

	//sort.Sort(toReturn)

	return toReturn, nil
}

// Fetch return a SegmentChangesItem
func (c SegmentChangesCollection) Fetch(name string) (*SegmentChangesItem, error) {
	key := []byte(name)
	item, err := c.Collection.FetchBy(key)
	if err != nil {
		return nil, err
	}

	if item == nil {
		return nil, nil
	}

	var decodeBuffer bytes.Buffer
	decodeBuffer.Write(item)
	dec := gob.NewDecoder(&decodeBuffer)

	var q SegmentChangesItem
	errq := dec.Decode(&q)
	if errq != nil {
		log.Error.Println("decode error:", errq)
	}
	return &q, nil
}
