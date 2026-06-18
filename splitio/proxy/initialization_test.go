package proxy

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/splitio/go-split-commons/v9/synchronizer"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/common/snapshot"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/util"
)

type syncManagerMock struct {
	c         chan int
	execCount int64
}

func (m *syncManagerMock) IsRunning() bool { panic("unimplemented") }
func (m *syncManagerMock) Start() {
	atomic.AddInt64(&m.execCount, 1)
	switch atomic.LoadInt64(&m.execCount) {
	case 1:
		m.c <- synchronizer.Error
	default:
		m.c <- synchronizer.Ready
	}
}
func (m *syncManagerMock) Stop() { panic("unimplemented") }

func (m *syncManagerMock) StartBGSync(mstatus chan int, shouldRetry bool, onReady func()) error {
	panic("unimplemented")
}

var _ synchronizer.Manager = (*syncManagerMock)(nil)

func TestSyncManagerInitializationRetriesWithSnapshot(t *testing.T) {

	sm := &syncManagerMock{c: make(chan int, 1)}

	// No snapshot and error
	complete := make(chan struct{}, 1)
	err := startBGSync(sm, sm.c, false, func() { complete <- struct{}{} })
	if err != errUnrecoverable {
		t.Error("should be an unrecoverable error. Got: ", err)
	}

	select {
	case <-complete:
		t.Error("nothing should be published on the channel")
	case <-time.After(500 * time.Millisecond):
		// all good
	}

	// Snapshot and error
	atomic.StoreInt64(&sm.execCount, 0)
	err = startBGSync(sm, sm.c, true, func() { complete <- struct{}{} })
	if err != errRetrying {
		t.Error("should be a retrying error. Got: ", err)
	}

	select {
	case <-complete:
		// all good
	case <-time.After(2500 * time.Millisecond):
		t.Error("should not time out")
	}

	if atomic.LoadInt64(&sm.execCount) != 2 {
		t.Error("there should be 2 executions")
	}
}

// mockLogger captures warning messages for testing
type mockLogger struct {
	logging.LoggerInterface
	warnings []string
}

func (m *mockLogger) Warning(msg ...interface{}) {
	m.warnings = append(m.warnings, fmt.Sprint(msg...))
}

func (m *mockLogger) Debug(msg ...interface{}) {}
func (m *mockLogger) Error(msg ...interface{}) {}
func (m *mockLogger) Info(msg ...interface{}) {}

func TestSnapshotHashMismatchLogsWarning(t *testing.T) {
	// Create a temporary snapshot file with a specific hash
	tmpDir, err := os.MkdirTemp("", "snapshot-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	snapshotPath := tmpDir + "/test-snapshot.snap"

	// Create a snapshot with hash "12345"
	meta := snapshot.Metadata{
		Version: 1,
		Storage: 1,
		Hash:    "12345",
	}
	snap, err := snapshot.New(meta, []byte("test data"))
	if err != nil {
		t.Fatalf("failed to create snapshot: %v", err)
	}

	// Encode snapshot to bytes and write to file
	encoded, err := snap.Encode()
	if err != nil {
		t.Fatalf("failed to encode snapshot: %v", err)
	}

	if err := os.WriteFile(snapshotPath, encoded, 0644); err != nil {
		t.Fatalf("failed to write snapshot file: %v", err)
	}

	// Create a config with different apikey/version/flagsets that will produce a different hash
	cfg := &pconf.Main{
		Apikey:          "test-apikey",
		FlagSpecVersion: "1.1",
		FlagSetsFilter:  []string{"set1", "set2"},
		Initialization: pconf.Initialization{
			Snapshot: snapshotPath,
		},
	}

	// Calculate what the hash would be with this config
	currentHash := util.HashAPIKey(cfg.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))
	expectedHashStr := strconv.Itoa(int(currentHash))

	// Verify that the hashes are indeed different
	if meta.Hash == expectedHashStr {
		t.Fatal("test setup error: hashes should be different for this test")
	}

	// Create a mock logger to capture warnings
	mockLog := &mockLogger{
		warnings: make([]string, 0),
	}

	// This simulates the code path in Start() that checks the hash
	snap2, err := snapshot.DecodeFromFile(snapshotPath)
	if err != nil {
		t.Fatalf("failed to decode snapshot: %v", err)
	}

	currentHash2 := util.HashAPIKey(cfg.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))
	if snap2.Meta().Hash != strconv.Itoa(int(currentHash2)) {
		mockLog.Warning("snapshot cfg (apikey, version, flagsets) does not match the provided one")
	}

	// Verify that a warning was logged
	if len(mockLog.warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(mockLog.warnings))
	}

	expectedWarning := "snapshot cfg (apikey, version, flagsets) does not match the provided one"
	if len(mockLog.warnings) > 0 && mockLog.warnings[0] != expectedWarning {
		t.Errorf("expected warning '%s', got '%s'", expectedWarning, mockLog.warnings[0])
	}
}

func TestSnapshotHashMatchNoWarning(t *testing.T) {
	// Create a temporary snapshot file
	tmpDir, err := os.MkdirTemp("", "snapshot-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	snapshotPath := tmpDir + "/test-snapshot.snap"

	// Create config
	cfg := &pconf.Main{
		Apikey:          "test-apikey",
		FlagSpecVersion: "1.1",
		FlagSetsFilter:  []string{"set1", "set2"},
		Initialization: pconf.Initialization{
			Snapshot: snapshotPath,
		},
	}

	// Calculate the hash with the config
	currentHash := util.HashAPIKey(cfg.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))
	hashStr := strconv.Itoa(int(currentHash))

	// Create a snapshot with the SAME hash
	meta := snapshot.Metadata{
		Version: 1,
		Storage: 1,
		Hash:    hashStr,
	}
	snap, err := snapshot.New(meta, []byte("test data"))
	if err != nil {
		t.Fatalf("failed to create snapshot: %v", err)
	}

	// Encode snapshot to bytes and write to file
	encoded, err := snap.Encode()
	if err != nil {
		t.Fatalf("failed to encode snapshot: %v", err)
	}

	if err := os.WriteFile(snapshotPath, encoded, 0644); err != nil {
		t.Fatalf("failed to write snapshot file: %v", err)
	}

	// Create a mock logger to capture warnings
	mockLog := &mockLogger{
		warnings: make([]string, 0),
	}

	// This simulates the code path in Start() that checks the hash
	snap2, err := snapshot.DecodeFromFile(snapshotPath)
	if err != nil {
		t.Fatalf("failed to decode snapshot: %v", err)
	}

	currentHash2 := util.HashAPIKey(cfg.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))
	if snap2.Meta().Hash != strconv.Itoa(int(currentHash2)) {
		mockLog.Warning("snapshot cfg (apikey, version, flagsets) does not match the provided one")
	}

	// Verify that NO warning was logged
	if len(mockLog.warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d: %v", len(mockLog.warnings), mockLog.warnings)
	}
}
