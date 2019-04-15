package api

import (
	"bytes"
	"encoding/json"
	"strconv"

	"github.com/splitio/split-synchronizer/log"
)

func sdkFetch(url string) ([]byte, error) {
	data, err := SdkClient.Get(url)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// SplitChangesFetchRaw GET request to fetch splits from server
func SplitChangesFetchRaw(since int64) ([]byte, error) {

	var bufferQuery bytes.Buffer
	bufferQuery.WriteString("/splitChanges")

	if since >= -1 {
		bufferQuery.WriteString("?since=")
		bufferQuery.WriteString(strconv.FormatInt(since, 10))
	}

	data, err := sdkFetch(bufferQuery.String())
	if err != nil {
		log.Error.Println("Error fetching split changes ", err)
		return nil, err
	}

	return data, nil
}

// SplitChangesFetch GET request to fetch splits from server
func SplitChangesFetch(since int64) (*SplitChangesDTO, error) {

	data, err := SplitChangesFetchRaw(since)
	if err != nil {
		log.Error.Println("Error fetching split changes ", err)
		return nil, err
	}

	var splitChangesDto SplitChangesDTO
	err = json.Unmarshal(data, &splitChangesDto)
	if err != nil {
		log.Error.Println("Error parsing split changes JSON ", err)
		return nil, err
	}

	// RAW DATA --------------
	var objmap map[string]*json.RawMessage
	if err = json.Unmarshal(data, &objmap); err != nil {
		log.Error.Println(err)
		return nil, err
	}

	if err = json.Unmarshal(*objmap["splits"], &splitChangesDto.RawSplits); err != nil {
		log.Error.Println(err)
		return nil, err
	}
	//-------------------------
	return &splitChangesDto, nil
}

// SegmentChangesFetchRaw GET request to fetch segments from server
func SegmentChangesFetchRaw(name string, since int64) ([]byte, error) {
	var bufferQuery bytes.Buffer
	bufferQuery.WriteString("/segmentChanges/")
	bufferQuery.WriteString(name)

	if since >= -1 {
		bufferQuery.WriteString("?since=")
		bufferQuery.WriteString(strconv.FormatInt(since, 10))
	}

	data, err := sdkFetch(bufferQuery.String())
	if err != nil {
		log.Error.Println("Error fetching segment changes ", err)
		return nil, err
	}

	return data, nil
}

// SegmentChangesFetch GET request to fetch segments from server
func SegmentChangesFetch(name string, since int64) (*SegmentChangesDTO, error) {
	data, err := SegmentChangesFetchRaw(name, since)
	if err != nil {
		log.Error.Println("Error fetching segment changes ", err)
		return nil, err
	}

	var segmentChangesDto SegmentChangesDTO
	err = json.Unmarshal(data, &segmentChangesDto)
	if err != nil {
		log.Error.Println("Error parsing segment changes JSON for segment ", name, err)
		return nil, err
	}

	return &segmentChangesDto, nil
}
