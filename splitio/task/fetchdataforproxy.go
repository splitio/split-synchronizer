package task

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/stats/counter"
	"github.com/splitio/split-synchronizer/splitio/stats/latency"
	"github.com/splitio/split-synchronizer/splitio/storage/boltdb"
	"github.com/splitio/split-synchronizer/splitio/storage/boltdb/collections"
)

// Stats
var latencyRegister = latency.NewLatencyBucket()
var counterRegister = counter.NewLocalCounter()

var proxyFetchSegmentBlocker chan bool
var proxyInProgressSegments map[string]struct{}
var proxySegmentsTill map[string]int64
var proxySegmentToProcess chan string
var mutexInProgress = &sync.Mutex{}
var mutexSegmentsTill = &sync.Mutex{}

// worker to fetch segments
func proxyFetchSegmentsWorker() {
	for {
		segmentName := <-proxySegmentToProcess
		mutexInProgress.Lock()
		if _, ok := proxyInProgressSegments[segmentName]; !ok {
			proxyInProgressSegments[segmentName] = struct{}{}
			go fetchSegment(segmentName)
		}
		mutexInProgress.Unlock()
	}
}

func saveSegmentData(segmentChangesDTO *api.SegmentChangesDTO) error {
	segmentCollection := collections.NewSegmentChangesCollection(boltdb.DBB)
	segmentItem, _ := segmentCollection.Fetch(segmentChangesDTO.Name)

	if segmentItem == nil {
		segmentItem = &collections.SegmentChangesItem{}
		segmentItem.Name = segmentChangesDTO.Name
		segmentItem.Keys = make(map[string]collections.SegmentKey)
	}

	for _, removedSegment := range segmentChangesDTO.Removed {
		log.Debug.Println("Removing", removedSegment, "from", segmentChangesDTO.Name)
		if _, exists := segmentItem.Keys[removedSegment]; exists {
			itemAux := segmentItem.Keys[removedSegment]
			itemAux.Removed = true
			itemAux.ChangeNumber = segmentChangesDTO.Till
			segmentItem.Keys[removedSegment] = itemAux
		} else {
			segmentItem.Keys[removedSegment] = collections.SegmentKey{Name: removedSegment,
				Removed: true, ChangeNumber: segmentChangesDTO.Till}
		}

	}

	for _, addedSegment := range segmentChangesDTO.Added {
		log.Debug.Println("Adding", addedSegment, "in", segmentChangesDTO.Name)
		if _, exists := segmentItem.Keys[addedSegment]; exists {
			itemAux := segmentItem.Keys[addedSegment]
			itemAux.Removed = false
			itemAux.ChangeNumber = segmentChangesDTO.Till
			segmentItem.Keys[addedSegment] = itemAux
		} else {
			segmentItem.Keys[addedSegment] = collections.SegmentKey{Name: addedSegment,
				Removed: false, ChangeNumber: segmentChangesDTO.Till}
		}
	}
	err := segmentCollection.Add(segmentItem)
	if err != nil {
		log.Error.Println(err)
		return err
	}

	return nil
}

func fetchSegment(segment string) {
	log.Debug.Println("Fetching segment:", segment)
	mutexSegmentsTill.Lock()
	var since = proxySegmentsTill[segment]
	mutexSegmentsTill.Unlock()
	for {
		start := latencyRegister.StartMeasuringLatency()
		rawData, err := api.SegmentChangesFetchRaw(segment, since)
		if err != nil {
			log.Error.Println("Error fetching split changes ", err)
			counterRegister.Increment("backend::request.error")
			break
		}
		latencyRegister.RegisterLatency("backend::/api/segmentChanges", start)
		counterRegister.Increment("backend::request.ok")
		log.Verbose.Println(string(rawData))

		segmentChangesDTO := &api.SegmentChangesDTO{}
		err = json.Unmarshal(rawData, segmentChangesDTO)
		if err != nil {
			log.Error.Println("Error parsing segment changes JSON ", err)
			break
		}

		if segmentChangesDTO.Till != -1 {
			// Old data shouldn't be processed
			if since >= segmentChangesDTO.Till {
				break
			}
		}

		err = saveSegmentData(segmentChangesDTO)
		if err != nil {
			log.Error.Println(err)
		}

		if segmentChangesDTO.Since >= segmentChangesDTO.Till {
			break
		} else {
			since = segmentChangesDTO.Till
		}
	}

	log.Verbose.Println("Saving last since value for", segment, "--->", since)
	mutexSegmentsTill.Lock()
	proxySegmentsTill[segment] = since
	mutexSegmentsTill.Unlock()

	// Release the in-progress segment
	mutexInProgress.Lock()
	delete(proxyInProgressSegments, segment)
	mutexInProgress.Unlock()
}

func retrieveJSONdata(rawData []byte) (int64, []collections.SplitChangesItem, error) {
	var till int64
	var splits = make([]collections.SplitChangesItem, 0)
	var err error

	var objmap map[string]*json.RawMessage
	if err = json.Unmarshal(rawData, &objmap); err != nil {
		log.Error.Println(err)
		return 0, nil, err
	}

	var tmpTill interface{}
	if err = json.Unmarshal(*objmap["till"], &tmpTill); err != nil {
		log.Error.Println(err)
		return 0, nil, err
	}

	till = int64(tmpTill.(float64))
	log.Verbose.Println("Fetched TILL (next since):", till)

	var tmpSplits []*json.RawMessage
	if err = json.Unmarshal(*objmap["splits"], &tmpSplits); err != nil {
		log.Error.Println(err)
		return 0, nil, err
	}
	log.Verbose.Println("Fetched splits:", len(tmpSplits))

	for i := 0; i < len(tmpSplits); i++ {

		splitChangesItem := &collections.SplitChangesItem{}
		err = json.Unmarshal(*tmpSplits[i], splitChangesItem)
		if err != nil {
			log.Error.Println("Error parsing split changes JSON", err)
			return 0, nil, err
		}
		rdat, _ := tmpSplits[i].MarshalJSON()
		splitChangesItem.JSON = string(rdat)

		splits = append(splits, *splitChangesItem)
	}

	return till, splits, nil
}

// FetchRawSplits task to retrieve split changes from Split servers
func FetchRawSplits(splitsRefreshRate int, segmentsRefreshRate int) {
	// Initialize global variables
	proxyFetchSegmentBlocker = make(chan bool, 10)
	proxyInProgressSegments = make(map[string]struct{}, 0)
	proxySegmentToProcess = make(chan string)
	proxySegmentsTill = make(map[string]int64, 0)

	//Launch fetch segments worker
	go proxyFetchSegmentsWorker()

	//Fetch registered segments
	go retrieveSegments(segmentsRefreshRate)

	// Starting to fetch splits
	splitCollection := collections.NewSplitChangesCollection(boltdb.DBB)

	// starting from beggining
	var since int64 = -1
	var splits []collections.SplitChangesItem

	for {
		//Fetch raw JSON from Split servers
		start := latencyRegister.StartMeasuringLatency()
		rawData, err := api.SplitChangesFetchRaw(since)
		if err != nil {
			log.Error.Println("Error fetching split changes ", err)
			counterRegister.Increment("backend::request.error")
			time.Sleep(time.Duration(5) * time.Second)
			continue
		}
		latencyRegister.RegisterLatency("backend::/api/splitChanges", start)
		counterRegister.Increment("backend::request.ok")
		log.Verbose.Println(string(rawData))

		// Parsing JSON and update since for next call
		prevSince := since
		since, splits, err = retrieveJSONdata(rawData)
		if err != nil {
			log.Error.Println("Error parsing splits ", err)
			time.Sleep(time.Duration(5) * time.Second)
			since = prevSince
			continue
		}

		//Saving in memory db
		for i := 0; i < len(splits); i++ {
			err = splitCollection.Add(&splits[i])
			if err != nil {
				log.Error.Println(err)
				continue
			}
			log.Verbose.Println(splits[i])
		}

		//Registering segments
		registerSegments(rawData)
		time.Sleep(time.Duration(splitsRefreshRate) * time.Second)
	}
}

func registerSegments(rawData []byte) {
	var splitChangesDto api.SplitChangesDTO
	err := json.Unmarshal(rawData, &splitChangesDto)
	if err != nil {
		log.Error.Println("Error parsing split changes JSON ", err)
	}

	splits := splitChangesDto.Splits
	totalItems := len(splits)

	for i := 0; i < totalItems; i++ {
		totalConditions := len(splits[i].Conditions)
		for j := 0; j < totalConditions; j++ {
			totalMatchers := len(splits[i].Conditions[j].MatcherGroup.Matchers)
			for k := 0; k < totalMatchers; k++ {
				if splits[i].Conditions[j].MatcherGroup.Matchers[k].MatcherType == "IN_SEGMENT" {
					segmentName := splits[i].Conditions[j].MatcherGroup.Matchers[k].UserDefinedSegment.SegmentName
					mutexSegmentsTill.Lock()
					if _, exists := proxySegmentsTill[segmentName]; !exists {
						proxySegmentsTill[segmentName] = -1
					}
					log.Debug.Println("Segments to be fetched:", proxySegmentsTill)
					mutexSegmentsTill.Unlock()
				}
			}
		}
	}
}

func retrieveSegments(segmentsRefreshRate int) {
	for {
		time.Sleep(time.Duration(segmentsRefreshRate) * time.Second)
		mutexSegmentsTill.Lock()
		for segmentName := range proxySegmentsTill {
			// Adding segment to channel to be processed by worker
			proxySegmentToProcess <- segmentName
		}
		mutexSegmentsTill.Unlock()
	}
}
