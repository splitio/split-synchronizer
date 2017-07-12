package stats

import (
	"encoding/binary"

	"github.com/boltdb/bolt"
	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/storage/boltdb"
)

// SDB boltdb instance pointer
var SDB *bolt.DB

const counterBucket = "COUNTERS"

// Initialize stats db
func Initialize() {
	var err error
	SDB, err = boltdb.NewInstance(boltdb.InMemoryMode, nil)
	if err != nil {
		log.Error.Println("The stats db could not be created")
	}
}

// COLLECTIONS

// SaveCounter saves counter value
func SaveCounter(name string, value int64) error {
	return SDB.Update(func(tx *bolt.Tx) error {
		bkt, err := tx.CreateBucketIfNotExists([]byte(counterBucket))
		if err != nil {
			log.Error.Println(err)
			return err
		}

		val := bkt.Get([]byte(name))
		if val != nil {
			i := int64(binary.LittleEndian.Uint64(val))
			value += i
		}

		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, uint64(value))
		return bkt.Put([]byte(name), b)
	})
}

// GetCounters
func GetCounters() map[string]int64 {
	var counters = make(map[string]int64)
	return counters
}
