// Package api contains all functions and dtos Split APIs
package api

import (
	"bytes"
	"encoding/json"
	"strconv"

	"github.com/splitio/go-agent/conf"
	"github.com/splitio/go-agent/errors"
	"github.com/splitio/go-agent/log"
)

const sdkName = "sdk"
const eventsName = "events"
const sdkURL = "https://sdk.splitio.io/api"
const eventsURL = "https://events.splitio.io/api"

var sdkClient *Client
var eventsClient *Client

// Initialize API fetchers
func Initialize() {
	for i := 0; i < len(conf.Data.APIServers); i++ {
		switch conf.Data.APIServers[i].Name {
		case sdkName:
			sdkClient = NewClient(conf.Data.APIServers[i].URL)
			log.Debug.Println("SDK API Client created with endpoint ", conf.Data.APIServers[i].URL)
			break
		case eventsName:
			eventsClient = NewClient(conf.Data.APIServers[i].URL)
			log.Debug.Println("EVENTS API Client created with endpoint ", conf.Data.APIServers[i].URL)
			break
		}
	}
}

func sdkFetch(url string) ([]byte, error) {
	data, err := sdkClient.Get(url)
	if errors.IsError(err) {
		return nil, err
	}
	return data, nil
}

// SplitChangesFetch GET request to fetch splits from server
func SplitChangesFetch(since int64) (*SplitChangesDTO, error) {

	var bufferQuery bytes.Buffer
	bufferQuery.WriteString("/splitChanges")

	if since >= -1 {
		bufferQuery.WriteString("?since=")
		bufferQuery.WriteString(strconv.FormatInt(since, 10))
	}

	data, err := sdkFetch(bufferQuery.String())
	if errors.IsError(err) {
		log.Error.Println("Error fetching split changes ", err)
		return nil, err
	}

	var splitChangesDto SplitChangesDTO
	err = json.Unmarshal(data, &splitChangesDto)
	if errors.IsError(err) {
		log.Error.Println("Error parsing split changes JSON ", err)
		return nil, err
	}

	return &splitChangesDto, nil
}

// SegmentChangesFetch GET request to fetch segments from server
func SegmentChangesFetch(name string, since int64) (*SegmentChangesDTO, error) {
	var bufferQuery bytes.Buffer
	bufferQuery.WriteString("/segmentChanges/")
	bufferQuery.WriteString(name)

	if since >= -1 {
		bufferQuery.WriteString("?since=")
		bufferQuery.WriteString(strconv.FormatInt(since, 10))
	}

	data, err := sdkFetch(bufferQuery.String())
	if errors.IsError(err) {
		log.Error.Println("Error fetching segment changes ", err)
		return nil, err
	}

	var segmentChangesDto SegmentChangesDTO
	err = json.Unmarshal(data, &segmentChangesDto)
	if errors.IsError(err) {
		log.Error.Println("Error parsing segment changes JSON for segment ", name, err)
		return nil, err
	}

	return &segmentChangesDto, nil
}
