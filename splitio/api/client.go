// Package api contains all functions and dtos Split APIs
package api

import (
	"compress/gzip"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/splitio/go-agent/conf"
	"github.com/splitio/go-agent/errors"
	"github.com/splitio/go-agent/log"
)

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
	defer resp.Body.Close()
	if errors.IsError(err) {
		log.Debug.Println("Status code: ", resp.StatusCode)
		log.Error.Println("Error requesting data to API: ", req.URL.String(), err.Error())
	}

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
	//var f SplitChangesDTO
	//var f SegmentChangesDTO
	//err = json.Unmarshal(body, &f)
	//log.Debug.Println(f.Till)

	return body, nil
}
