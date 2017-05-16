package boltdb

import (
	"os"
	"strconv"
	"time"

	"github.com/boltdb/bolt"
	"github.com/splitio/go-agent/log"
)

// InMemoryMode used to store ramdom db into temporal folder
const InMemoryMode = ":memory:"

const inMemoryDBName = "splitio_"

// NewInstance creates a new instance of BoltDB wrapper
func NewInstance(path string, options *bolt.Options) (*bolt.DB, error) {
	var dbpath string
	if path == InMemoryMode {
		dbpath = os.TempDir() + "/" + inMemoryDBName + strconv.Itoa(int(time.Now().Unix())) + ".db"
		log.Debug.Println("Temporary database will be created at", dbpath)
	} else {
		dbpath = path
	}

	dbb, err := bolt.Open(dbpath, 0644, options)
	if err != nil {
		log.Error.Println(err)
		return nil, err
	}
	return dbb, nil
}
