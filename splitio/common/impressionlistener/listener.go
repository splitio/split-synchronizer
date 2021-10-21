package impressionlistener

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/splitio/go-split-commons/v4/dtos"

	"github.com/splitio/go-toolkit/v5/struct/traits/lifecycle"
)

// ErrInvalidQueueSize is returned when attemptingn to construct a listener with an invalid queue size
var ErrInvalidQueueSize = errors.New("queue size must be at least 1")

// ErrQueueFull is returned when attempting to push an impression bulk in a full queue
var ErrQueueFull = errors.New("queue is full, cannot add impression bulk")

// ErrAlreadyRunning is returned when attempting to start an already running listener
var ErrAlreadyRunning = errors.New("listener is already running")

// ErrNotRunning is returned when attempting to stop a non-running listener
var ErrNotRunning = errors.New("listener is not running")

// ImpressionBulkListener speciefies the interface of a secondary impression listener
type ImpressionBulkListener interface {
	Submit(imps json.RawMessage, metadata *dtos.Metadata) error
	Start() error
	Stop(bool) error
}

// impressionListenerPostBody bundles all the data posted by the impression's listener
type impressionListenerPostBody struct {
	Impressions json.RawMessage `json:"impressions"`
	SdkVersion  string          `json:"sdkVersion"`
	MachineIP   string          `json:"machineIP"`
	MachineName string          `json:"machineName"`
}

// ImpressionBulkListenerImpl is an implementation of the ImpressionBulkListener interface
type ImpressionBulkListenerImpl struct {
	lifecycle  lifecycle.Manager
	endpoint   string
	httpClient *http.Client
	queue      chan impressionListenerPostBody
}

// NewImpressionBulkListener constructs a new impression listner
func NewImpressionBulkListener(endpoint string, queueSize int, httpClient *http.Client) (*ImpressionBulkListenerImpl, error) {
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	if queueSize < 1 {
		return nil, ErrInvalidQueueSize
	}

	return &ImpressionBulkListenerImpl{
		endpoint:   endpoint,
		httpClient: httpClient,
		queue:      make(chan impressionListenerPostBody, queueSize),
	}, nil
}

// Submit attempts to push an impression bulk into the queue
// Will fail if the queue is full
func (l *ImpressionBulkListenerImpl) Submit(imps json.RawMessage, metadata *dtos.Metadata) error {
	select {
	case l.queue <- impressionListenerPostBody{
		Impressions: imps,
		SdkVersion:  metadata.SDKVersion,
		MachineIP:   metadata.MachineIP,
		MachineName: metadata.MachineName,
	}:
		return nil
	default:
		return ErrQueueFull
	}
}

// Start the bg task that will take bulks from the queue and post them
func (l *ImpressionBulkListenerImpl) Start() error {
	if !l.lifecycle.BeginInitialization() {
		return ErrAlreadyRunning
	}

	go func() {
		defer l.lifecycle.ShutdownComplete()
		if !l.lifecycle.InitializationComplete() {
			return
		}

		for {
			select {
			case <-l.lifecycle.ShutdownRequested():
				return
			case imps := <-l.queue:
				l.post(imps)
			}
		}
	}()

	return nil
}

// Stop the bg task
func (l *ImpressionBulkListenerImpl) Stop(blocking bool) error {
	if !l.lifecycle.BeginShutdown() {
		return ErrNotRunning
	}

	if blocking {
		l.lifecycle.AwaitShutdownComplete()
	}

	return nil
}

func (l *ImpressionBulkListenerImpl) post(imps impressionListenerPostBody) error {
	data, err := json.Marshal(imps)
	if err != nil {
		return fmt.Errorf("error serializing impressions: %w", err)
	}

	request, _ := http.NewRequest("POST", l.endpoint, bytes.NewBuffer(data))
	response, err := l.httpClient.Do(request)
	if err != nil {
		return err
	}
	response.Body.Close()
	return nil
}

var _ ImpressionBulkListener = (*ImpressionBulkListenerImpl)(nil)
