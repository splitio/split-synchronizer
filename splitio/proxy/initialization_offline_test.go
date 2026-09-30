package proxy

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/common"
	"github.com/splitio/split-synchronizer/v5/splitio/common/snapshot"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"

	"github.com/splitio/go-split-commons/v10/dtos"
	commonsflagsets "github.com/splitio/go-split-commons/v10/flagsets"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/stretchr/testify/assert"
)

func writeTestSnapshot(t *testing.T, meta snapshot.Metadata, payload []byte) string {
	t.Helper()
	snap, err := snapshot.New(meta, payload)
	assert.Nil(t, err)
	raw, err := snap.Encode()
	assert.Nil(t, err)
	path := filepath.Join(t.TempDir(), "proxy.snapshot")
	assert.Nil(t, os.WriteFile(path, raw, 0644))
	return path
}

func assertInvalidConfiguration(t *testing.T, err error) {
	t.Helper()
	var initErr *common.InitializationError
	if assert.True(t, errors.As(err, &initErr), "expected an initialization error, got %v", err) {
		assert.Equal(t, common.ExitInvalidConfiguration, initErr.ExitCode())
	}
}

func TestLoadOfflineSnapshot(t *testing.T) {
	payload := []byte("bolt-bytes")
	validMeta := snapshot.Metadata{
		Version:         1,
		Storage:         snapshot.StorageBoltDB,
		Hash:            "unrelated-hash",
		Checksum:        snapshot.PayloadChecksum(payload),
		FlagSpecVersion: "1.3",
	}

	t.Run("valid snapshot loads regardless of hash", func(t *testing.T) {
		cfg := &pconf.Main{FlagSpecVersion: "1.2"}
		cfg.Initialization.Snapshot = writeTestSnapshot(t, validMeta, payload)
		snap, data, err := loadOfflineSnapshot(cfg)
		assert.Nil(t, err)
		assert.Equal(t, payload, data)
		assert.Equal(t, "1.3", snap.Meta().FlagSpecVersion)
	})

	t.Run("api key is rejected", func(t *testing.T) {
		cfg := &pconf.Main{Apikey: "someKey", FlagSpecVersion: "1.3"}
		cfg.Initialization.Snapshot = writeTestSnapshot(t, validMeta, payload)
		_, _, err := loadOfflineSnapshot(cfg)
		assertInvalidConfiguration(t, err)
	})

	t.Run("missing snapshot path is rejected", func(t *testing.T) {
		_, _, err := loadOfflineSnapshot(&pconf.Main{FlagSpecVersion: "1.3"})
		assertInvalidConfiguration(t, err)
	})

	t.Run("unreadable snapshot file is rejected", func(t *testing.T) {
		cfg := &pconf.Main{FlagSpecVersion: "1.3"}
		cfg.Initialization.Snapshot = filepath.Join(t.TempDir(), "missing.snapshot")
		_, _, err := loadOfflineSnapshot(cfg)
		assertInvalidConfiguration(t, err)
	})

	t.Run("snapshot without checksum is rejected", func(t *testing.T) {
		meta := validMeta
		meta.Checksum = ""
		cfg := &pconf.Main{FlagSpecVersion: "1.3"}
		cfg.Initialization.Snapshot = writeTestSnapshot(t, meta, payload)
		_, _, err := loadOfflineSnapshot(cfg)
		assertInvalidConfiguration(t, err)
	})

	t.Run("checksum mismatch is rejected", func(t *testing.T) {
		meta := validMeta
		meta.Checksum = snapshot.PayloadChecksum([]byte("other-bytes"))
		cfg := &pconf.Main{FlagSpecVersion: "1.3"}
		cfg.Initialization.Snapshot = writeTestSnapshot(t, meta, payload)
		_, _, err := loadOfflineSnapshot(cfg)
		assertInvalidConfiguration(t, err)
	})
}

// Treatment overrides (FME-19233) rely on offline loading leaving the Bolt data untouched,
// so a re-exported snapshot keeps the checksum of the file that was loaded.
func TestOfflineLoadPreservesExportChecksum(t *testing.T) {
	logger := logging.NewLogger(nil)
	src, err := persistent.NewBoltWrapper(filepath.Join(t.TempDir(), "src.db"), nil)
	assert.Nil(t, err)
	storage.NewProxySplitStorage(src, logger, commonsflagsets.NewFlagSetFilter(nil), false).Update(
		[]dtos.SplitDTO{{Name: "s1", Status: "ACTIVE", ChangeNumber: 10}, {Name: "s2", Status: "ACTIVE", ChangeNumber: 12}},
		nil,
		12,
	)
	exported, err := src.GetRawSnapshot()
	assert.Nil(t, err)

	path, err := snapshot.WritePayloadToTmpFile(exported)
	assert.Nil(t, err)
	defer os.Remove(path)
	db, err := persistent.NewBoltWrapper(path, nil)
	assert.Nil(t, err)
	storage.NewProxySplitStorage(db, logger, commonsflagsets.NewFlagSetFilter(nil), true)
	storage.NewProxyRuleBasedSegmentsStorage(db, logger, true)
	storage.NewProxySegmentStorage(db, logger, true)

	reexported, err := db.GetRawSnapshot()
	assert.Nil(t, err)
	assert.Equal(t, snapshot.PayloadChecksum(exported), snapshot.PayloadChecksum(reexported))
}
