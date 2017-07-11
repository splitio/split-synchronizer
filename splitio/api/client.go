package api

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/splitio/go-agent/conf"
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
	headers    map[string]string
}

// NewClient instance of Client
func NewClient(endpoint string) *Client {
	client := &http.Client{Timeout: time.Duration(conf.Data.HTTPTimeout) * time.Second}
	return &Client{url: endpoint, httpClient: client, headers: make(map[string]string)}
}

// Get method is a get call to an url
func (c *Client) Get(service string) ([]byte, error) {

	serviceURL := c.url + service
	log.Debug.Println("[GET] ", serviceURL)
	req, _ := http.NewRequest("GET", serviceURL, nil)

	authorization := conf.Data.APIKey
	log.Debug.Println("Authorization [ApiKey]: ", log.ObfuscateAPIKey(authorization))

	req.Header.Add("Authorization", "Bearer "+authorization)
	req.Header.Add("SplitSDKVersion", "go-0.0.1")
	req.Header.Add("User-Agent", "SplitIO-GO-AGENT/0.1")
	req.Header.Add("Accept-Encoding", "gzip")
	req.Header.Add("Content-Type", "application/json")

	//logging headers
	if conf.Data.Logger.DebugOn {
		log.Debug.Println("[REQUEST_HEADERS]", log.ObfuscateHTTPHeader(req.Header), "[END_REQUEST_HEADERS]")
	}

	startTimeInMillis := time.Now().UnixNano() / 1000000
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error.Println("Error requesting data to API: ", req.URL.String(), err.Error())
		return nil, err
	}
	defer resp.Body.Close()
	tookTimeInMillis := time.Now().UnixNano()/1000000 - startTimeInMillis
	log.Debug.Println("REQUEST TIME TOOK:", tookTimeInMillis, "millis")

	//logging headers
	if conf.Data.Logger.DebugOn {
		log.Debug.Println("[RESPONSE_HEADERS]", log.ObfuscateHTTPHeader(resp.Header), "[END_RESPONSE_HEADERS]")
	}
	log.Verbose.Println("[RESPONSE_STATUS]", resp.Status, " - ", resp.StatusCode, "[END_RESPONSE_STATUS]")

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
	if err != nil {
		log.Error.Println(err.Error())
		return nil, err
	}

	log.Verbose.Println("[RESPONSE_BODY]", string(body), "[END_RESPONSE_BODY]")

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return body, nil
	}

	return nil, fmt.Errorf("GET method: Status Code: %d - %s", resp.StatusCode, resp.Status)
}

// Post performs a HTTP POST request
func (c *Client) Post(service string, body []byte) error {

	serviceURL := c.url + service
	log.Debug.Println("[POST] ", serviceURL)
	req, _ := http.NewRequest("POST", serviceURL, bytes.NewBuffer(body))
	//****************
	req.Close = true // To prevent EOF error when connection is closed
	//****************
	authorization := conf.Data.APIKey
	log.Debug.Println("Authorization [ApiKey]: ", log.ObfuscateAPIKey(authorization))

	req.Header.Add("Authorization", "Bearer "+authorization)
	//SplitSDKVersion added by poster tasks
	req.Header.Add("User-Agent", "SplitIO-GO-AGENT/0.1")
	req.Header.Add("Accept-Encoding", "gzip")
	req.Header.Add("Content-Type", "application/json")

	for headerName, headerValue := range c.headers {
		req.Header.Add(headerName, headerValue)
	}

	//logging headers
	if conf.Data.Logger.DebugOn {
		log.Debug.Println("[REQUEST_HEADERS]", log.ObfuscateHTTPHeader(req.Header), "[END_REQUEST_HEADERS]")
	}

	startTimeInMillis := time.Now().UnixNano() / 1000000
	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error.Println("Error requesting data to API: ", req.URL.String(), err.Error())
		return err
	}
	defer resp.Body.Close()
	tookTimeInMillis := time.Now().UnixNano()/1000000 - startTimeInMillis
	log.Debug.Println("REQUEST TIME TOOK:", tookTimeInMillis, "millis")

	//logging headers
	if conf.Data.Logger.DebugOn {
		log.Debug.Println("[RESPONSE_HEADERS]", log.ObfuscateHTTPHeader(resp.Header), "[END_RESPONSE_HEADERS]")
	}
	log.Verbose.Println("[RESPONSE_STATUS]", resp.Status, " - ", resp.StatusCode, "[END_RESPONSE_STATUS]")

	respBody, _ := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error.Println(err.Error())
		return err
	}

	log.Verbose.Println("[RESPONSE_BODY]", string(respBody), "[END_RESPONSE_BODY]")

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return nil
	}

	return fmt.Errorf("POST method: Status Code: %d - %s", resp.StatusCode, resp.Status)
}

// AddHeader adds header value to HTTP client
func (c *Client) AddHeader(name string, value string) {
	c.headers[name] = value
}

// ResetHeaders resets custom headers
func (c *Client) ResetHeaders() {
	c.headers = make(map[string]string)
}
