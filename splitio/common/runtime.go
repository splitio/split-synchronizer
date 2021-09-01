package common

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/splitio/go-split-commons/v4/synchronizer"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/go-toolkit/v5/sync"

	"github.com/splitio/split-synchronizer/v4/splitio/common/impressionlistener"
	"github.com/splitio/split-synchronizer/v4/splitio/log"
)

// ErrShutdownAlreadyRegistered is returned when trying to register the shutdown handler more than once
var ErrShutdownAlreadyRegistered = errors.New("shutdown handler already scheduled")

// Runtime defines the interface
type Runtime interface {
	Uptime() time.Duration
	Shutdown()
	ShutdownWithMessage(message string)
}

// RuntimeImpl provides an implementation for the Runtime interface
type RuntimeImpl struct {
	proxy              bool
	startup            time.Time
	shutdownRegistered *sync.AtomicBool
	logger             logging.LoggerInterface
	slackWriter        *log.SlackWriter
	syncManager        synchronizer.Manager
	impListener        impressionlistener.ImpressionBulkListener
	blocker            chan struct{}
	osSignals          chan<- os.Signal
}

// NewRuntime constructs a RuntimeImpl object
func NewRuntime(
	proxy bool,
	syncManager synchronizer.Manager,
	logger logging.LoggerInterface,
	listener impressionlistener.ImpressionBulkListener,
	slackWriter *log.SlackWriter,
) *RuntimeImpl {
	return &RuntimeImpl{
		proxy:              proxy,
		startup:            time.Now(),
		logger:             logger,
		slackWriter:        slackWriter,
		syncManager:        syncManager,
		impListener:        listener,
		blocker:            make(chan struct{}),
		shutdownRegistered: sync.NewAtomicBool(false),
	}
}

// RegisterShutdownHandler installs a shutdown handler that will be triggered when a SIGTERM/SIGQUIT/SIGINT is received
func (r *RuntimeImpl) RegisterShutdownHandler() error {
	if !r.shutdownRegistered.TestAndSet() {
		return ErrShutdownAlreadyRegistered
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-signals
		r.Shutdown()

	}()

	return nil
}

// Uptime returns how long the sync has been running
func (r *RuntimeImpl) Uptime() time.Duration {
	return time.Now().Sub(r.startup)
}

// Shutdown stops sends a SIGTERM to the current process
func (r *RuntimeImpl) Shutdown() {
	r.logger.Info("\n\n * Starting graceful shutdown")
	r.logger.Info(" * Waiting goroutines stop")
	r.syncManager.Stop()
	if r.impListener != nil {
		r.impListener.Stop(true)
	}
	r.logger.Info(" * Shutdown complete - see you soon!")
	r.blocker <- struct{}{}
}

// Block puts the current goroutine on hold until Shutdown is complete
func (r *RuntimeImpl) Block() {
	<-r.blocker
}

// ShutdownWithMessage logs a message and then sends a SIGTERM to the current process
func (r *RuntimeImpl) ShutdownWithMessage(message string) {
	// TODO(mredolatti): implement!
}
