package boltdb

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/boltdb/bolt"
	"github.com/splitio/go-agent/log"
)

// InMemoryMode used to store ramdom db into temporal folder
const InMemoryMode = ":memory:"

const inMemoryDBName = "splitio_"

// DBB boltdb instance pointer
var DBB *bolt.DB

// Initialize the DBB instance pointer to a valid boltdb
func Initialize(path string, options *bolt.Options) {
	var err error
	DBB, err = NewInstance(path, options)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// NewInstance creates a new instance of BoltDB wrapper
func NewInstance(path string, options *bolt.Options) (*bolt.DB, error) {
	var dbpath string
	if path == InMemoryMode {
		tmpDir := os.TempDir()
		if !strings.HasSuffix(tmpDir, "/") {
			tmpDir = tmpDir + "/"
		}
		dbpath = tmpDir + inMemoryDBName + strconv.Itoa(int(time.Now().UnixNano())) + ".db"
		log.Debug.Println("Temporary database will be created at", dbpath)
		fmt.Println("DB PATH:", dbpath)
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
