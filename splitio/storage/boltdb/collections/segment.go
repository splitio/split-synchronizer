package collections

import (
	"bytes"
	"encoding/gob"

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
/*
const sccBucketName = "SEGMENT_%s"
const sccChunkBucketName = "C_%d"
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

func createChunkedBucket(root *bolt.Bucket) []byte {
	var bucketName []byte

	lastChunkBucket := root.Stats().BucketN - 1
	bucketName = []byte(fmt.Sprintf(sccChunkBucketName, lastChunkBucket+1))
	_, errc := root.CreateBucketIfNotExists(bucketName)
	if errc != nil {
		log.Error.Println(errc)
		return nil
	}

	return bucketName
}

func (c *SegmentChunkedCollection) getAvailableBucket() []byte {

	var bktName []byte

	err := c.DB.Update(func(tx *bolt.Tx) error {
		// fetching/creating parent bucket
		root, errb := tx.CreateBucketIfNotExists([]byte(fmt.Sprintf(sccBucketName, c.Name)))
		if errb != nil {
			log.Error.Println("Error getting segment bucket", errb)
			return errb
		}

		var lastChunkBucket = root.Stats().BucketN - 1
		var lbkt *bolt.Bucket
		if lastChunkBucket == 0 {
			bktName = createChunkedBucket(root)
		} else {
			lastChunkBucketName := []byte(fmt.Sprintf(sccChunkBucketName, lastChunkBucket))
			lbkt = root.Bucket(lastChunkBucketName)
			var remainingKeys = sccChunkSize - lbkt.Stats().KeyN
			if remainingKeys > 0 { //there are free slots to add items
				bktName = lastChunkBucketName
			} else { //a new bucket must be created
				bktName = createChunkedBucket(root)
			}
		}

		if bktName == nil {
			return fmt.Errorf("Error fetching available bucket for segment %s", c.Name)
		}

		return nil
	})

	if err != nil {
		log.Error.Println(err)
	}

	return bktName
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

func (c *SegmentChunkedCollection) putKeysIntoBucket(bktName []byte, items []SegmentKey) ([]SegmentKey, error) {
	var remainingList = make([]SegmentKey, 0)

	err := c.DB.Update(func(tx *bolt.Tx) error {

		root := tx.Bucket([]byte(fmt.Sprintf(sccBucketName, c.Name)))
		bkt := root.Bucket(bktName)
		remaining := sccChunkSize - bkt.Stats().KeyN

		for _, k := range items {
			var err error
			var encodeBuffer bytes.Buffer

			existsInBktName := keyExistsIn(root, []byte(k.Name))
			if existsInBktName != nil { //Update existing key
				gob.NewEncoder(&encodeBuffer).Encode(k)
				err = root.Bucket(existsInBktName).Put([]byte(k.Name), encodeBuffer.Bytes())
			} else { //Add new key
				if remaining > 0 {
					gob.NewEncoder(&encodeBuffer).Encode(k)
					err = bkt.Put([]byte(k.Name), encodeBuffer.Bytes())
					if err == nil {
						remaining--
					}
				} else {
					remainingList = append(remainingList, k)
				}
			}

			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return remainingList, err
	}

	return remainingList, nil
}

// Add an item
func (c *SegmentChunkedCollection) Add(items []SegmentKey) error {

	chunks := chunkSlice(items)
	for _, chunk := range chunks {
		var bktName = c.getAvailableBucket()
		remainingList, err := c.putKeysIntoBucket(bktName, chunk)
		if err != nil {
			log.Error.Println(err)
			return err
		}

		if len(remainingList) > 0 { //items cannot be allocated on bucket
			bktName = c.getAvailableBucket()
			_, err := c.putKeysIntoBucket(bktName, remainingList)
			if err != nil {
				log.Error.Println(err)
				return err
			}
		}
	}

	return nil
}
*/
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

/*func (c SegmentChangesCollection) FetchAll() ([]*SegmentChangesItem, error) {
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
}*/

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

// FetchAll return a list of SegmentChangesItem
func (c SegmentChangesCollection) FetchAll() ([]SegmentChangesItem, error) {

	items, err := c.Collection.FetchAll()
	if err != nil {
		return nil, err
	}

	var toReturn = make([]SegmentChangesItem, 0)
	for _, item := range items {
		if item == nil {
			continue
		}

		var q SegmentChangesItem
		var decodeBuffer bytes.Buffer
		decodeBuffer.Write(item)
		errq := gob.NewDecoder(&decodeBuffer).Decode(&q)
		if errq != nil {
			log.Error.Println("decode error:", errq)
			continue
		}

		toReturn = append(toReturn, q)
	}

	return toReturn, nil
}
