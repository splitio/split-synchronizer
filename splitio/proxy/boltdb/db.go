package boltdb

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/boltdb/bolt"
)

// InMemoryMode used to store ramdom db into temporal folder
const InMemoryMode = ":memory:"

const inMemoryDBName = "splitio_"

// DBB boltdb instance pointer
var DBB *bolt.DB

// Initialize the DBB instance pointer to a valid boltdb
// func Initialize(path string, options *bolt.Options) {
// 	var err error
// 	DBB, err = NewInstance(path, options)
// 	if err != nil {
// 		fmt.Println(err)
// 		os.Exit(splitio.ExitErrorDB)
// 	}
// }

// NewInstance creates a new instance of BoltDB wrapper
func NewInstance(path string, options *bolt.Options) (*bolt.DB, error) {
	var dbpath string
	if path == InMemoryMode {
		tmpDir := os.TempDir()
		if !strings.HasSuffix(tmpDir, "/") {
			tmpDir = tmpDir + "/"
		}
		dbpath = tmpDir + inMemoryDBName + strconv.Itoa(int(time.Now().UnixNano())) + ".db"
	} else {
		dbpath = path
	}

	dbb, err := bolt.Open(dbpath, 0644, options)
	if err != nil {
		return nil, fmt.Errorf("error opening db: ", err)
	}
	return dbb, nil
}
