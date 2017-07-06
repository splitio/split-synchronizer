package collections

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"testing"

	"github.com/boltdb/bolt"
	uuid "github.com/satori/go.uuid"
	"github.com/splitio/go-agent/conf"
	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/storage/boltdb"
)

func before() {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)
	//Initialize by default
	conf.Initialize()

	conf.Data.Logger.DebugOn = true
}

func TestSegmentKeys(t *testing.T) {
	before()

	numberOfKeys := 10
	chunkSize := 2
	list := make([]SegmentKey, 0)

	for j := 1; j <= numberOfKeys; j++ {
		segmentItem := SegmentKey{
			Name:         fmt.Sprintf("4d37521c-06d5-43cf-849f-4727bfdeaa0c_%d", j),
			Removed:      false,
			ChangeNumber: 1498262190861}

		list = append(list, segmentItem)
	}

	//fmt.Println(len(list))

	var divided [][]SegmentKey

	for i := 0; i < len(list); i += chunkSize {
		end := i + chunkSize

		if end > len(list) {
			end = len(list)
		}

		divided = append(divided, list[i:end])
	}
	//fmt.Println(divided)
	db, err := boltdb.NewInstance("/Users/sarrubia/segments.db", nil)
	if err != nil {
		t.Error(err)
		return
	}

	for n, chunk := range divided {
		segmentName := fmt.Sprintf("Segment_Name_POC_%d", n)
		err = db.Update(func(tx *bolt.Tx) error {

			root, _ := tx.CreateBucketIfNotExists([]byte("SEGMENTS"))

			segmentBucket, errt := root.CreateBucketIfNotExists([]byte(segmentName))
			if errt != nil {
				fmt.Println(errt)
				return errt
			}

			for _, item := range chunk {
				var encodeBuffer bytes.Buffer
				erre := gob.NewEncoder(&encodeBuffer).Encode(item)
				if erre != nil {
					fmt.Println(erre)
					return erre
				}

				data := encodeBuffer.Bytes()
				errp := segmentBucket.Put([]byte(item.Name), data)
				if errp != nil {
					fmt.Println(errp)
					return errp
				}
			}
			return nil
		})

		if err != nil {
			t.Error(err)
			return
		}
	}

	err = db.Update(func(tx *bolt.Tx) error {

		bkt := tx.Bucket([]byte("SEGMENTS"))
		fmt.Println(bkt.Stats().BucketN)
		var counter = 0
		bkt.ForEach(func(k []byte, v []byte) error {
			if v == nil {
				counter++
				//fmt.Printf("key=%s, value=%s\n", k, v)
				//fmt.Println(bkt.Bucket(k).Stats().KeyN)
			}
			return nil
		})
		fmt.Println("BUCKETS:", counter)
		/*return tx.ForEach(func(name []byte, b *bolt.Bucket) error {
			fmt.Printf("key=%s, value=%d\n", name, b.Stats().KeyN)
			return nil
		})*/
		return nil
	})
}

func testSegmentAllocation(t *testing.T) {
	before()
	//Test variables
	numberOfKeys := 100000000
	numberOfSegments := 1
	var dbb *bolt.DB
	var err error
	//dbb, err = boltdb.NewInstance(boltdb.InMemoryMode, nil)
	dbb, err = boltdb.NewInstance("/Users/sarrubia/segments.db", nil)
	if err != nil {
		t.Error(err)
		return
	}

	segmentCollection := NewSegmentChangesCollection(dbb)

	for i := 1; i <= numberOfSegments; i++ {
		segmentItem := &SegmentChangesItem{}
		segmentItem.Name = fmt.Sprintf("segment_name_%d", i)
		segmentItem.Keys = make(map[string]SegmentKey)
		keyCounter := 0
		for j := 1; j <= numberOfKeys; j++ {
			keyName := fmt.Sprintf(uuid.NewV4().String()+"_%d", j)
			segmentItem.Keys[keyName] = SegmentKey{
				Name:         keyName,
				Removed:      false,
				ChangeNumber: 1498262190861}
			keyCounter++
		}

		err = segmentCollection.Add(segmentItem)
		if err != nil {
			t.Error(err)
			return
		}
	}

}
