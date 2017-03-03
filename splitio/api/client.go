// Package api contains all functions and dtos Split APIs
package api

import (
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/splitio/go-agent/conf"
	"github.com/splitio/go-agent/errors"
	"github.com/splitio/go-agent/log"
)

const sdkName = "sdk"
const eventsName = "events"
const sdkURL = "https://sdk.split.io/api"
const eventsURL = "https://events.split.io/api"

const envSdkURLNamespace = "SPLITIO_SDK_URL"
const envEventsURLNamespace = "SPLITIO_EVENTS_URL"

var sdkClient *Client
var eventsClient *Client

// Initialize API client
func Initialize() {
	envSdkURL := os.Getenv(envSdkURLNamespace)
	if envSdkURL != "" {
		sdkClient = NewClient(envSdkURL)
		log.Debug.Println("SDK API Client created with endpoint ", envSdkURL)
	} else {
		sdkClient = NewClient(sdkURL)
		log.Debug.Println("SDK API Client created with endpoint ", sdkURL)
	}

	envEventsURL := os.Getenv(envEventsURLNamespace)
	if envEventsURL != "" {
		eventsClient = NewClient(envEventsURL)
		log.Debug.Println("EVENTS API Client created with endpoint ", envEventsURL)
	} else {
		eventsClient = NewClient(eventsURL)
		log.Debug.Println("EVENTS API Client created with endpoint ", eventsURL)
	}
}

// Client structure to wrap up the net/http.Client
type Client struct {
	url        string
	httpClient *http.Client
}

// NewClient instance of Client
func NewClient(endpoint string) *Client {
	client := &http.Client{}
	return &Client{url: endpoint, httpClient: client}
}

// Get method is a get call to an url
func (c *Client) Get(service string) ([]byte, error) {

	serviceURL := c.url + service
	log.Debug.Println("[GET] ", serviceURL)
	req, _ := http.NewRequest("GET", serviceURL, nil)

	authorization := conf.Data.APIKey
	log.Debug.Println("Authorization [ApiKey]: " + authorization)

	req.Header.Add("Authorization", "Bearer "+authorization)
	req.Header.Add("SplitSDKVersion", "go-0.0.1")
	req.Header.Add("User-Agent", "SplitIO-GO-AGENT/0.1")
	req.Header.Add("Accept-Encoding", "gzip")
	req.Header.Add("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if errors.IsError(err) {
		log.Error.Println("Error requesting data to API: ", req.URL.String(), err.Error())
		return nil, err
	}
	defer resp.Body.Close()

	// Check that the server actually sent compressed data
	var reader io.ReadCloser
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, _ = gzip.NewReader(resp.Body)
		defer reader.Close()
	default:
		reader = resp.Body
	}

	body, err := ioutil.ReadAll(reader)
	if errors.IsError(err) {
		log.Error.Println(err.Error())
		return nil, err
	}

	log.Verbose.Println("[RESPONSE_BODY]", string(body), "[END_RESPONSE_BODY]")

	return body, nil
}
