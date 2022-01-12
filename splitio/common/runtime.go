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

	"github.com/splitio/split-synchronizer/v5/splitio/common/impressionlistener"
	"github.com/splitio/split-synchronizer/v5/splitio/log"
	"github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/application"
	"github.com/splitio/split-synchronizer/v5/splitio/provisional/healthcheck/services"
)

// ErrShutdownAlreadyRegistered is returned when trying to register the shutdown handler more than once
var ErrShutdownAlreadyRegistered = errors.New("shutdown handler already scheduled")

// Runtime defines the interface
type Runtime interface {
	Uptime() time.Duration
	Shutdown()
	Kill()
}

// RuntimeImpl provides an implementation for the Runtime interface
type RuntimeImpl struct {
	proxy              bool
	startup            time.Time
	shutdownRegistered *sync.AtomicBool
	logger             logging.LoggerInterface
	dashboardTitle     string
	slackWriter        *log.SlackWriter
	syncManager        synchronizer.Manager
	impListener        impressionlistener.ImpressionBulkListener
	blocker            chan struct{}
	osSignals          chan os.Signal
	appMonitor         application.MonitorIterface
	servicesMonitor    services.MonitorIterface
}

// NewRuntime constructs a RuntimeImpl object
func NewRuntime(
	proxy bool,
	syncManager synchronizer.Manager,
	logger logging.LoggerInterface,
	dashboardTitle string,
	listener impressionlistener.ImpressionBulkListener,
	slackWriter *log.SlackWriter,
	appMonitor application.MonitorIterface,
	servicesMonitor services.MonitorIterface,
) *RuntimeImpl {
	return &RuntimeImpl{
		proxy:              proxy,
		startup:            time.Now(),
		logger:             logger,
		dashboardTitle:     dashboardTitle,
		slackWriter:        slackWriter,
		syncManager:        syncManager,
		impListener:        listener,
		blocker:            make(chan struct{}),
		shutdownRegistered: sync.NewAtomicBool(false),
		osSignals:          make(chan os.Signal, 1),
		appMonitor:         appMonitor,
		servicesMonitor:    servicesMonitor,
	}
}

// RegisterShutdownHandler installs a shutdown handler that will be triggered when a SIGTERM/SIGQUIT/SIGINT is received
func (r *RuntimeImpl) RegisterShutdownHandler() error {
	if !r.shutdownRegistered.TestAndSet() {
		return ErrShutdownAlreadyRegistered
	}

	signal.Notify(r.osSignals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		s := <-r.osSignals
		if s == syscall.SIGKILL {
			os.Exit(0)
		}
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
	if r.slackWriter != nil {
		message, attachments := buildSlackShutdownMessage(r.dashboardTitle, false)
		r.slackWriter.PostNow(message, attachments)
	}
	r.syncManager.Stop()
	if r.impListener != nil {
		r.impListener.Stop(true)
	}
	r.appMonitor.Stop()
	r.servicesMonitor.Stop()

	r.logger.Info(" * Shutdown complete - see you soon!")
	r.blocker <- struct{}{}
}

// Block puts the current goroutine on hold until Shutdown is complete
func (r *RuntimeImpl) Block() {
	<-r.blocker
}

// Kill sends a SIGKILL and aborts the app immediately
func (r *RuntimeImpl) Kill() {
	r.osSignals <- syscall.SIGKILL
}

func buildSlackShutdownMessage(title string, kill bool) ([]byte, []log.SlackMessageAttachment) {
	var color string
	var message string
	if kill {
		color = "danger"
		message = "*[KILL]* Force shutdown signal sent - see you soon!"
	} else {
		color = "good"
		message = "*[Important]* Shutting down split-sync - see you soon!"
	}

	var attach []log.SlackMessageAttachment
	if title != "" {
		fields := make([]log.SlackMessageAttachmentFields, 0)
		fields = append(fields)
		attach = []log.SlackMessageAttachment{log.SlackMessageAttachment{
			Fallback: "Shutting Split-Sync down",
			Color:    color,
			Fields: []log.SlackMessageAttachmentFields{{
				Title: title,
				Value: "Shutting it down, see you soon!",
				Short: false,
			}},
		}}
	}

	return []byte(message), attach
}
