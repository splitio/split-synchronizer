package persistent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

// errorRecorder records the messages logged at error level.
type errorRecorder struct {
	logging.LoggerInterface
	mu   sync.Mutex
	errs []string
}

func (r *errorRecorder) Error(msg ...interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errs = append(r.errs, fmt.Sprint(msg...))
}

func (r *errorRecorder) logged(substr string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.errs {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

// closedDB returns a wrapper whose database is closed, so every write fails.
func closedDB(t *testing.T) *BoltDBWrapper {
	t.Helper()
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	require.NoError(t, err)
	require.NoError(t, dbw.wrapped.Close())
	return dbw
}

// TestSplitChangesWriteFailureIsReported checks that a failed Bolt write for feature flags
// is returned by UpdateWithError and logged by Update.
func TestSplitChangesWriteFailureIsReported(t *testing.T) {
	toAdd := []dtos.SplitDTO{{Name: "s1", ChangeNumber: 1, Status: "ACTIVE"}}

	t.Run("UpdateWithError returns the error", func(t *testing.T) {
		splitC := NewSplitChangesCollection(closedDB(t), logging.NewLogger(nil))
		assert.Error(t, splitC.UpdateWithError(toAdd, nil, 1))
	})
	t.Run("Update logs it", func(t *testing.T) {
		rec := &errorRecorder{}
		splitC := NewSplitChangesCollection(closedDB(t), rec)
		splitC.Update(toAdd, nil, 1)
		assert.True(t, rec.logged("persisting feature flags"), "errors: %v", rec.errs)
	})
	t.Run("a healthy write returns nil", func(t *testing.T) {
		dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = dbw.Discard() })
		splitC := NewSplitChangesCollection(dbw, logging.NewLogger(nil))
		require.NoError(t, splitC.UpdateWithError(toAdd, nil, 7))
		assert.Equal(t, int64(7), splitC.ChangeNumber())
	})
}

// TestSplitRestoreWithoutItemsAnchorsChangeNumber checks that a flags restore with no items still
// anchors the change number.
func TestSplitRestoreWithoutItemsAnchorsChangeNumber(t *testing.T) {
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dbw.Discard() })
	splitC := NewSplitChangesCollection(dbw, logging.NewLogger(nil))

	require.NoError(t, splitC.UpdateWithError(nil, nil, 42))
	assert.Equal(t, int64(42), splitC.ChangeNumber())
}

// unencodable has no exported field, so gob refuses to encode it.
type unencodable struct{ id uint64 }

func (u *unencodable) SetID(id uint64) { u.id = id }
func (u *unencodable) ID() uint64      { return u.id }

// TestCollectionWritesReturnEncodeErrors checks that gob encode errors are returned, not stored as an empty value.
func TestCollectionWritesReturnEncodeErrors(t *testing.T) {
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dbw.Discard() })
	coll := &BoltDBCollectionWrapper{db: dbw, name: "encode_errors", logger: logging.NewLogger(nil)}

	t.Run("SaveAs", func(t *testing.T) {
		assert.Error(t, coll.SaveAs([]byte("k"), &unencodable{}))
		_, err := coll.FetchBy([]byte("k"))
		assert.Error(t, err, "nothing may be stored when the encode failed")
	})
	t.Run("Save", func(t *testing.T) {
		_, err := coll.Save(&unencodable{})
		assert.Error(t, err)
	})
	t.Run("Update", func(t *testing.T) {
		item := &unencodable{}
		item.SetID(1)
		assert.Error(t, coll.Update(item))
	})
}

func TestSplitPersistentStorage(t *testing.T) {
	dbw, err := NewBoltWrapper(BoltInMemoryMode, nil)
	if err != nil {
		t.Error("error creating bolt wrapper: ", err)
	}

	logger := logging.NewLogger(nil)
	splitC := NewSplitChangesCollection(dbw, logger)

	splitC.Update([]dtos.SplitDTO{
		{Name: "s1", ChangeNumber: 1, Status: "ACTIVE"},
		{Name: "s2", ChangeNumber: 1, Status: "ACTIVE"},
	}, nil, 1)

	all, err := splitC.FetchAll()
	if err != nil {
		t.Error("FetchAll should not return an error. Got: ", err)
	}

	if len(all) != 2 {
		t.Error("invalid number of items fetched.")
		return
	}

	if all[0].Name != "s1" || all[1].Name != "s2" {
		t.Error("Invalid payload in fetched changes.")
	}

	if splitC.ChangeNumber() != 1 {
		t.Error("CN should be 1.")
	}

	splitC.Update([]dtos.SplitDTO{{Name: "s1", ChangeNumber: 2, Status: "ARCHIVED"}}, nil, 2)
	all, err = splitC.FetchAll()
	if err != nil {
		t.Error("FetchAll should not return an error. Got: ", err)
	}

	if len(all) != 2 {
		t.Error("invalid number of items fetched.")
		return
	}

	if all[0].Name != "s1" || all[0].Status != "ARCHIVED" {
		t.Error("s1 should be archived.")
	}

	if splitC.ChangeNumber() != 2 {
		t.Error("CN should be 2.")
	}
}

type saveFailCollection struct {
	CollectionWrapper
	fail   bool
	logger logging.LoggerInterface
}

func (f *saveFailCollection) SaveAs(key []byte, item interface{}) error {
	if f.fail {
		return errors.New("save failed")
	}
	return nil
}

func (f *saveFailCollection) Logger() logging.LoggerInterface { return f.logger }

func TestSplitChangesCollectionCursorAdvancesOnlyWhenSaved(t *testing.T) {
	logger := logging.NewLogger(nil)
	failing := &SplitChangesCollection{collection: &saveFailCollection{fail: true, logger: logger}}
	failing.Update([]dtos.SplitDTO{{Name: "f", ChangeNumber: 5, Status: "ACTIVE"}}, nil, 5)
	if cn := failing.ChangeNumber(); cn != 0 {
		t.Errorf("cursor must not advance when a save fails, got %d", cn)
	}

	ok := &SplitChangesCollection{collection: &saveFailCollection{logger: logger}}
	ok.Update([]dtos.SplitDTO{{Name: "f", ChangeNumber: 5, Status: "ACTIVE"}}, nil, 5)
	if cn := ok.ChangeNumber(); cn != 5 {
		t.Errorf("cursor should advance after successful saves, got %d", cn)
	}
}
func TestBoltInMemoryNoSyncRoundTrip(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	db, err := NewBoltWrapper(BoltInMemoryMode, nil)
	require.Nil(t, err)
	t.Cleanup(func() { _ = db.Discard() })
	assert.True(t, db.wrapped.NoSync)

	require.Nil(t, db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("roundtrip"))
		if err != nil {
			return err
		}
		return b.Put([]byte("k"), []byte("v"))
	}))

	var got string
	require.Nil(t, db.View(func(tx *bolt.Tx) error {
		got = string(tx.Bucket([]byte("roundtrip")).Get([]byte("k")))
		return nil
	}))
	assert.Equal(t, "v", got)
}

func TestBoltDBWrapperDiscardRemovesFile(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	db, err := NewBoltWrapper(BoltInMemoryMode, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := os.Stat(db.path); err != nil {
		t.Fatalf("db file should exist: %v", err)
	}
	if err := db.Discard(); err != nil {
		t.Fatalf("discard: %v", err)
	}
	if _, err := os.Stat(db.path); !os.IsNotExist(err) {
		t.Errorf("db file should be removed, stat err: %v", err)
	}
}

func TestDiscardKeepsFileOfPersistentDB(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keep.db")
	db, err := NewBoltWrapper(path, nil)
	require.Nil(t, err)
	assert.Nil(t, db.Discard())

	_, err = os.Stat(path)
	assert.Nil(t, err, "file of a persistent db must survive Discard")
}

func TestDiscardRemovesInMemoryDBFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	db, err := NewBoltWrapper(BoltInMemoryMode, nil)
	require.Nil(t, err)
	assert.Nil(t, db.Discard())

	entries, err := os.ReadDir(dir)
	require.Nil(t, err)
	assert.Empty(t, entries, "in-memory db file should be removed by Discard")
}

func TestDiscardTwiceIsSafe(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	db, err := NewBoltWrapper(BoltInMemoryMode, nil)
	require.Nil(t, err)

	assert.NotPanics(t, func() {
		assert.Nil(t, db.Discard())
		_ = db.Discard()
	})

	_, statErr := os.Stat(db.path)
	assert.True(t, os.IsNotExist(statErr), "db file should be gone after Discard")
}

type failOnKeyCollection struct {
	CollectionWrapper
	failKey string
	logger  logging.LoggerInterface
}

func (f *failOnKeyCollection) SaveAs(key []byte, item interface{}) error {
	if string(key) == f.failKey {
		return errors.New("save failed")
	}
	return nil
}

func (f *failOnKeyCollection) Logger() logging.LoggerInterface { return f.logger }

func TestSplitChangesCollectionCursorHoldsOnPartialFailure(t *testing.T) {
	c := &SplitChangesCollection{collection: &failOnKeyCollection{failKey: "bad", logger: logging.NewLogger(nil)}}
	err := c.UpdateWithError([]dtos.SplitDTO{
		{Name: "good", ChangeNumber: 5, Status: "ACTIVE"},
		{Name: "bad", ChangeNumber: 5, Status: "ACTIVE"},
	}, nil, 5)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad")
	assert.Equal(t, int64(0), c.ChangeNumber(), "cursor must not advance when any item failed")
}
