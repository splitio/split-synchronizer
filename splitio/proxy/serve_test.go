package proxy

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/splitio/split-synchronizer/v5/splitio/common"
	sconf "github.com/splitio/split-synchronizer/v5/splitio/common/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/common/snapshot"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"

	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/stretchr/testify/assert"
)

// neverReturns is a block function that never unblocks, like the runtime waiting for a signal.
func neverReturns() {
	select {}
}

// runServeWithTimeout runs fn and fails the test if it does not return promptly.
func runServeWithTimeout(t *testing.T, fn func() error) error {
	t.Helper()
	result := make(chan error, 1)
	go func() { result <- fn() }()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("serveUntilShutdown did not return")
		return nil
	}
}

// assertServeInitError checks that err is an InitializationError with the given exit code whose message names the server and address.
func assertServeInitError(t *testing.T, err error, exitCode int, name, addr string) {
	t.Helper()
	var initErr *common.InitializationError
	if !assert.True(t, errors.As(err, &initErr), "expected *common.InitializationError, got %v", err) {
		return
	}
	assert.Equal(t, exitCode, initErr.ExitCode())
	assert.Contains(t, err.Error(), name)
	assert.Contains(t, err.Error(), addr)
}

func TestServeReturnsNilWhenBlockReturns(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	spec := serverSpec{
		name:     "proxy API",
		addr:     "127.0.0.1:3000",
		start:    func() error { <-release; return http.ErrServerClosed },
		exitCode: common.ExitTaskInitialization,
	}
	block := func() { time.Sleep(50 * time.Millisecond) }

	err := runServeWithTimeout(t, func() error { return serveUntilShutdown(block, spec) })
	assert.Nil(t, err)
}

func TestServeReturnsInitErrorWhenServerFails(t *testing.T) {
	spec := serverSpec{
		name:     "proxy API",
		addr:     "127.0.0.1:3000",
		start:    func() error { return errors.New("listen tcp 127.0.0.1:3000: bind: address already in use") },
		exitCode: common.ExitTaskInitialization,
	}

	err := runServeWithTimeout(t, func() error { return serveUntilShutdown(neverReturns, spec) })
	assertServeInitError(t, err, common.ExitTaskInitialization, "proxy API", "127.0.0.1:3000")
}

func TestServeAdminFailureUsesAdminExitCode(t *testing.T) {
	spec := serverSpec{
		name:     "admin server",
		addr:     "127.0.0.1:3010",
		start:    func() error { return errors.New("listen tcp 127.0.0.1:3010: bind: address already in use") },
		exitCode: common.ExitAdminError,
	}

	err := runServeWithTimeout(t, func() error { return serveUntilShutdown(neverReturns, spec) })
	assertServeInitError(t, err, common.ExitAdminError, "admin server", "127.0.0.1:3010")
}

func TestServeIgnoresErrServerClosed(t *testing.T) {
	spec := serverSpec{
		name:     "admin server",
		addr:     "127.0.0.1:3010",
		start:    func() error { return http.ErrServerClosed },
		exitCode: common.ExitAdminError,
	}
	block := func() { time.Sleep(50 * time.Millisecond) }

	err := runServeWithTimeout(t, func() error { return serveUntilShutdown(block, spec) })
	assert.Nil(t, err)
}

func TestServeFirstFailureWins(t *testing.T) {
	admin := serverSpec{
		name:     "admin server",
		addr:     "127.0.0.1:3010",
		start:    func() error { return errors.New("bind failed") },
		exitCode: common.ExitAdminError,
	}
	api := serverSpec{
		name:     "proxy API",
		addr:     "127.0.0.1:3000",
		start:    func() error { return errors.New("bind failed") },
		exitCode: common.ExitTaskInitialization,
	}

	err := runServeWithTimeout(t, func() error { return serveUntilShutdown(neverReturns, admin, api) })

	var initErr *common.InitializationError
	if assert.True(t, errors.As(err, &initErr), "expected *common.InitializationError, got %v", err) {
		assert.Contains(t, []int{common.ExitAdminError, common.ExitTaskInitialization}, initErr.ExitCode())
	}
}

func TestServeDoesNotLeakBlockingOnFailure(t *testing.T) {
	// The server fails after block has already started waiting, and block never returns.
	spec := serverSpec{
		name: "proxy API",
		addr: "127.0.0.1:3000",
		start: func() error {
			time.Sleep(50 * time.Millisecond)
			return errors.New("bind failed")
		},
		exitCode: common.ExitTaskInitialization,
	}

	err := runServeWithTimeout(t, func() error { return serveUntilShutdown(neverReturns, spec) })
	assertServeInitError(t, err, common.ExitTaskInitialization, "proxy API", "127.0.0.1:3000")
}

// newOfflineTestConfig returns a valid offline configuration backed by a real (empty) Bolt snapshot.
func newOfflineTestConfig(t *testing.T) *pconf.Main {
	t.Helper()
	db, err := persistent.NewBoltWrapper(filepath.Join(t.TempDir(), "source.db"), nil)
	if !assert.Nil(t, err) {
		t.FailNow()
	}
	payload, err := db.GetRawSnapshot()
	if !assert.Nil(t, err) {
		t.FailNow()
	}

	meta := snapshot.Metadata{
		Version:         1,
		Storage:         snapshot.StorageBoltDB,
		Hash:            "unrelated-hash",
		Checksum:        snapshot.PayloadChecksum(payload),
		FlagSpecVersion: "1.3",
	}

	cfg := &pconf.Main{}
	if err := sconf.PopulateDefaults(cfg); err != nil {
		t.Fatalf("populating defaults: %v", err)
	}
	cfg.Offline = true
	cfg.Apikey = ""
	cfg.FlagSpecVersion = "1.3"
	cfg.Initialization.Snapshot = writeTestSnapshot(t, meta, payload)
	return cfg
}

// occupyTCPPortOn holds a listening socket on a random port of addr until the test ends.
func occupyTCPPortOn(t *testing.T, addr string) int {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("occupying port: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().(*net.TCPAddr).Port
}

// freeTCPPort returns a loopback port that was free a moment ago.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func TestOfflineStartFailsWhenPortInUse(t *testing.T) {
	logger := logging.NewLogger(nil)

	t.Run("proxy port", func(t *testing.T) {
		// The SDK API always binds 0.0.0.0, so the occupant must hold the wildcard address to conflict.
		port := occupyTCPPortOn(t, "0.0.0.0:0")
		cfg := newOfflineTestConfig(t)
		cfg.Server.Host = "0.0.0.0"
		cfg.Server.Port = int64(port)
		cfg.Admin.Host = "127.0.0.1"
		cfg.Admin.Port = int64(freeTCPPort(t))

		err := runServeWithTimeout(t, func() error { return Start(logger, cfg) })
		assertServeInitError(t, err, common.ExitTaskInitialization, "proxy API", fmt.Sprintf("0.0.0.0:%d", port))
	})

	t.Run("admin port", func(t *testing.T) {
		port := occupyTCPPortOn(t, "127.0.0.1:0")
		cfg := newOfflineTestConfig(t)
		cfg.Admin.Host = "127.0.0.1"
		cfg.Admin.Port = int64(port)
		cfg.Server.Host = "127.0.0.1"
		cfg.Server.Port = int64(freeTCPPort(t))

		err := runServeWithTimeout(t, func() error { return Start(logger, cfg) })
		assertServeInitError(t, err, common.ExitAdminError, "admin server", "127.0.0.1:"+strconv.Itoa(port))
	})
}
