package persistent

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/splitio/go-toolkit/v5/logging"

	bolt "go.etcd.io/bbolt" // new fork maintained by etcd
)

// BoltInMemoryMode used to store ramdom db into temporal folder
const BoltInMemoryMode = ":memory:"
const inMemoryDBName = "splitio_"

// ErrorBucketNotFound error type for bucket not found
var ErrorBucketNotFound = errors.New("Bucket not found")

// ErrorKeyNotFound error type for key not found within a bucket
var ErrorKeyNotFound = errors.New("key not found")

// DBWrapper defines the interface for a Persistant storage wrapper
type DBWrapper interface {
	Update(func(*bolt.Tx) error) error
	View(func(*bolt.Tx) error) error
	Lock()
	Unlock()
	GetRawSnapshot() ([]byte, error)
}

// BoltDBWrapper is a boltdb-based implmentation of a persistent storage wrapper
type BoltDBWrapper struct {
	wrapped *bolt.DB
	path    string
	temp    bool // true only for the throwaway file created by BoltInMemoryMode
	mutex   sync.Mutex
}

// Discard closes the db and, when it is a BoltInMemoryMode throwaway, removes its file.
// A db opened on a real path is only closed, so a snapshot or user file is never deleted.
func (b *BoltDBWrapper) Discard() error {
	closeErr := b.wrapped.Close()
	if !b.temp {
		return closeErr
	}
	if err := os.Remove(b.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(closeErr, fmt.Errorf("error removing db file: %w", err))
	}
	return closeErr
}

// Update executes a RW function within a transaction
func (b *BoltDBWrapper) Update(f func(tx *bolt.Tx) error) error {
	return b.wrapped.Update(f)
}

// View executes a RO function wihtin a transaction
func (b *BoltDBWrapper) View(f func(tx *bolt.Tx) error) error {
	return b.wrapped.View(f)
}

// Lock grants exclusive access to the referenced db
func (b *BoltDBWrapper) Lock() {
	b.mutex.Lock()
}

// Unlock reliquishes exclusive access to the referenced db
func (b *BoltDBWrapper) Unlock() {
	b.mutex.Unlock()
}

// GetRawSnapshot dumps all the contents of the db into a raw byte buffer
func (b *BoltDBWrapper) GetRawSnapshot() ([]byte, error) {
	var buffer bytes.Buffer
	err := b.View(func(tx *bolt.Tx) error {
		_, err := tx.WriteTo(&buffer)
		return err
	})

	if err != nil {
		return nil, fmt.Errorf("error reading whole db: %w", err)
	}

	return buffer.Bytes(), nil
}

// GetRawSnapshotWithContentID dumps all the contents of the db into a raw byte buffer and computes its
// content identifier. Both are produced from the same read transaction.
func (b *BoltDBWrapper) GetRawSnapshotWithContentID(flagSpecVersion string, flagSets []string) ([]byte, string, error) {
	var buffer bytes.Buffer
	var contentID string
	err := b.View(func(tx *bolt.Tx) error {
		if _, err := tx.WriteTo(&buffer); err != nil {
			return fmt.Errorf("error reading whole db: %w", err)
		}

		var err error
		contentID, err = computeContentID(tx, flagSpecVersion, flagSets)
		return err
	})

	if err != nil {
		return nil, "", err
	}

	return buffer.Bytes(), contentID, nil
}

// CollectionItem is the item into a collection
type CollectionItem interface {
	SetID(id uint64)
	ID() uint64
}

// CollectionWrapper defines the set of methods that should be implemented by a collection
type CollectionWrapper interface {
	Delete(key []byte) error
	SaveAs(key []byte, item interface{}) error
	Save(item CollectionItem) (uint64, error)
	Update(item CollectionItem) error
	Fetch(id uint64) ([]byte, error)
	FetchBy(key []byte) ([]byte, error)
	FetchAll() ([][]byte, error)
	Logger() logging.LoggerInterface
}

// BoltDBCollectionWrapper wraps a boltdb collection (aka bucket)
type BoltDBCollectionWrapper struct {
	db     DBWrapper
	name   string
	logger logging.LoggerInterface
}

// Delete removess an item into collection under key parameter
func (c *BoltDBCollectionWrapper) Delete(key []byte) error {
	c.db.Lock()
	defer c.db.Unlock()

	// Insert value in DB
	return c.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(c.name))
		if err != nil {
			return err
		}

		err = bucket.Delete(key)
		if err != nil {
			return err
		}

		return nil
	})
}

// encodeItem gob-encodes item. An error rolls back the enclosing transaction instead of storing an empty value.
func encodeItem(item interface{}) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(item); err != nil {
		return nil, fmt.Errorf("encoding item: %w", err)
	}
	return buf.Bytes(), nil
}

// SaveAs saves an item into collection under key parameter
func (c *BoltDBCollectionWrapper) SaveAs(key []byte, item interface{}) error {

	c.db.Lock()
	defer c.db.Unlock()

	// Insert value in DB
	return c.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(c.name))
		if err != nil {
			return err
		}

		encoded, err := encodeItem(item)
		if err != nil {
			return err
		}

		return bucket.Put(key, encoded)
	})
}

// Save an item into collection setting autoincrement ID
func (c *BoltDBCollectionWrapper) Save(item CollectionItem) (uint64, error) {
	c.db.Lock()
	defer c.db.Unlock()

	var id uint64
	// Insert value in DB
	updateError := c.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(c.name))
		if err != nil {
			return err
		}

		id, _ = bucket.NextSequence()
		item.SetID(id)

		encoded, err := encodeItem(item)
		if err != nil {
			return err
		}

		return bucket.Put(itob(id), encoded)
	})

	if updateError != nil {
		c.logger.Error(updateError)
		return 0, updateError
	}

	return id, nil
}

// Update an item into collection with current item ID
func (c *BoltDBCollectionWrapper) Update(item CollectionItem) error {
	if !(item.ID() > 0) {
		c.logger.Error("Trying to update an item with ID 0")
		return errors.New("Invalid ID, it must be grater than zero")
	}

	c.db.Lock()
	defer c.db.Unlock()

	// Insert value in DB
	updateError := c.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(c.name))
		if err != nil {
			return err
		}

		id := item.ID()

		encoded, err := encodeItem(item)
		if err != nil {
			return err
		}

		return bucket.Put(itob(id), encoded)
	})

	if updateError != nil {
		c.logger.Error(updateError)
		return updateError
	}

	return nil
}

// Fetch returns an item from collection
func (c *BoltDBCollectionWrapper) Fetch(id uint64) ([]byte, error) {

	c.db.Lock()
	defer c.db.Unlock()

	var item []byte
	err := c.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(c.name))
		if bucket == nil {
			return ErrorBucketNotFound
		}

		itemRef := bucket.Get(itob(id))
		if itemRef == nil {
			return ErrorKeyNotFound
		}
		item = make([]byte, len(itemRef))
		copy(item, itemRef)

		return nil
	})

	if err != nil {
		return nil, err
	}

	return item, nil
}

// FetchBy returns an item from collection given a key
func (c *BoltDBCollectionWrapper) FetchBy(key []byte) ([]byte, error) {

	c.db.Lock()
	defer c.db.Unlock()

	var item []byte
	err := c.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(c.name))
		if bucket == nil {
			return ErrorBucketNotFound
		}

		itemRef := bucket.Get(key)
		if itemRef == nil {
			return ErrorKeyNotFound
		}

		item = make([]byte, len(itemRef))
		copy(item, itemRef)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return item, nil
}

// FetchAll fetch all saved items
func (c *BoltDBCollectionWrapper) FetchAll() ([][]byte, error) {

	c.db.Lock()
	defer c.db.Unlock()

	toReturn := make([][]byte, 0)
	err := c.db.View(func(tx *bolt.Tx) error {
		// Assume bucket exists and has keys
		bucket := tx.Bucket([]byte(c.name))
		if bucket == nil {
			return ErrorBucketNotFound
		}

		cursor := bucket.Cursor()

		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			it := make([]byte, len(v))
			copy(it, v)
			toReturn = append(toReturn, it)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return toReturn, err
}

// Logger returns a reference to a logger
func (c *BoltDBCollectionWrapper) Logger() logging.LoggerInterface {
	return c.logger
}

// NewBoltWrapper creates a new instance of BoltDB wrapper
func NewBoltWrapper(path string, options *bolt.Options) (*BoltDBWrapper, error) {
	dbpath := path
	if path == BoltInMemoryMode {
		dbpath = filepath.Join(os.TempDir(), fmt.Sprintf("%s_%d.db", inMemoryDBName, time.Now().UnixNano()))
		if options == nil {
			// The temp file is a throwaway, never reused after a restart, so fsync is skipped.
			options = &bolt.Options{NoSync: true}
		}
	}

	var err error
	wrapper := &BoltDBWrapper{path: dbpath, temp: path == BoltInMemoryMode}
	wrapper.wrapped, err = bolt.Open(dbpath, 0644, options)
	if err != nil {
		if wrapper.temp {
			// bolt may have created the throwaway file before failing; a missing file is fine
			if rmErr := os.Remove(dbpath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
				err = fmt.Errorf("%w (also failed to remove %s: %v)", err, dbpath, rmErr)
			}
		}
		return nil, fmt.Errorf("error opening db: %w", err)
	}
	return wrapper, nil
}
