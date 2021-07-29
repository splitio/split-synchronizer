package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v4/log"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/boltdb/collections"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/controllers"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/interfaces"
)

func TestSplitController(t *testing.T) {

	db, err := boltdb.NewInstance(fmt.Sprintf("/tmp/test_controller_splits_%d.db", time.Now().UnixNano()), nil)
	if err != nil {
		t.Error(err)
	}

	boltdb.DBB = db

	var split1 = &collections.SplitChangesItem{Name: "SPLIT_1", ChangeNumber: 333333, Status: "ACTIVE", JSON: "some_json_split1"}
	var split2 = &collections.SplitChangesItem{Name: "SPLIT_2", ChangeNumber: 222222, Status: "ARCHIVED", JSON: "some_json_split2"}
	var split3 = &collections.SplitChangesItem{Name: "SPLIT_3", ChangeNumber: 111111, Status: "ACTIVE", JSON: "some_json_split3"}

	splitCollection := collections.NewSplitChangesCollection(db)

	erra := splitCollection.Add(split1)
	if erra != nil {
		t.Error(erra)
	}

	erra = splitCollection.Add(split2)
	if erra != nil {
		t.Error(erra)
	}

	erra = splitCollection.Add(split3)
	if erra != nil {
		t.Error(erra)
	}

	// Since = -1
	splits, till, errf := fetchSplitsFromDB(-1)
	if errf != nil {
		t.Error(errf)
	}

	if len(splits) != 3 {
		t.Error("Invalid len result")
	}

	if till != 333333 {
		t.Error("Invalid TILL value")
	}

	//Since = 222222
	splits, till, errf = fetchSplitsFromDB(222222)
	if errf != nil {
		t.Error(errf)
	}

	if len(splits) != 1 {
		t.Error("Invalid len result")
	}

	if till != 333333 {
		t.Error("Invalid TILL value")
	}
}

func TestSegmentController(t *testing.T) {
	db, err := boltdb.NewInstance(fmt.Sprintf("/tmp/test_controller_segments_%d.db", time.Now().UnixNano()), nil)
	if err != nil {
		t.Error(err)
	}

	boltdb.DBB = db
	segmentName := "SEGMENT_TEST"

	var segment = &collections.SegmentChangesItem{Name: segmentName}
	segment.Keys = make(map[string]collections.SegmentKey)

	key1 := collections.SegmentKey{Name: "Key_1", Removed: false, ChangeNumber: 1}
	key2 := collections.SegmentKey{Name: "Key_2", Removed: false, ChangeNumber: 2}
	key3 := collections.SegmentKey{Name: "Key_3", Removed: false, ChangeNumber: 3}
	key4 := collections.SegmentKey{Name: "Key_4", Removed: true, ChangeNumber: 4}

	segment.Keys[key1.Name] = key1
	segment.Keys[key2.Name] = key2
	segment.Keys[key3.Name] = key3
	segment.Keys[key4.Name] = key4

	col := collections.NewSegmentChangesCollection(boltdb.DBB)

	// test Add
	errs := col.Add(segment)
	if errs != nil {
		t.Error(errs)
	}

	added, removed, till, errf := fetchSegmentsFromDB(-1, segmentName)
	if errf != nil {
		t.Error(errf)
	}

	if till != 3 {
		t.Error("Incorrect TILL value")
	}

	if len(added) != 3 {
		t.Error("Wrong number of keys in ADDED")
	}

	if len(removed) != 0 {
		t.Error("Wrong number of keys in REMOVED")
	}
	// test keys
	if !inSegmentArray(added, key1.Name) || !inSegmentArray(added, key2.Name) || !inSegmentArray(added, key3.Name) {
		t.Error("Missing key")
	}

	if inSegmentArray(added, key4.Name) {
		t.Error("Removed keys musn't be added")
	}

	added, removed, till, errf = fetchSegmentsFromDB(3, segmentName)
	if errf != nil {
		t.Error(errf)
	}

	if till != 4 {
		t.Error("Incorrect TILL value")
	}

	if len(added) != 0 {
		t.Error("Wrong number of keys in ADDED")
	}

	if len(removed) != 1 {
		t.Error("Wrong number of keys in REMOVED")
	}
	// testing keys
	if !inSegmentArray(removed, key4.Name) {
		t.Error("Invalid key added in REMOVED array")
	}
}

func inSegmentArray(keys []string, key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}

func TestAPIKeyValidator(t *testing.T) {
	if validateAPIKey(make([]string, 0), "something") {
		t.Error("It should be invalid")
	}

	if validateAPIKey([]string{"something"}, "some") {
		t.Error("It should be invalid")
	}

	if !validateAPIKey([]string{"something"}, "something") {
		t.Error("It should be valid")
	}

	if !validateAPIKey([]string{"some", "something"}, "something") {
		t.Error("It should be valid")
	}
}

func performRequest(r http.Handler, method, path string, body string) *httptest.ResponseRecorder {
	var data io.ReadCloser
	if body != "" {
		data = ioutil.NopCloser(bytes.NewReader([]byte(body)))
	}
	req, _ := http.NewRequest(method, path, data)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func checkHeaders(t *testing.T, r *http.Request) {
	sdkVersion := r.Header.Get("SplitSDKVersion")
	sdkMachineName := r.Header.Get("SplitSDKMachineName")
	sdkMachine := r.Header.Get("SplitSDKMachineIP")

	if sdkVersion != "something" {
		t.Error("SDK Version HEADER not match")
	}

	if sdkMachine != "" {
		t.Error("SDK Machine HEADER not match")
	}

	if sdkMachineName != "" {
		t.Error("SDK Machine Name HEADER not match", sdkMachineName)
	}
}

func TestPostImpressionsBeacon(t *testing.T) {
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}
	interfaces.Initialize()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkHeaders(t, r)

		rBody, _ := ioutil.ReadAll(r.Body)

		var impressionsInPost []dtos.ImpressionsDTO
		err := json.Unmarshal(rBody, &impressionsInPost)
		if err != nil {
			t.Error(err)
			return
		}

		if impressionsInPost[0].TestName != "some_test" || len(impressionsInPost) != 2 {
			t.Error("Impressions malformed")
		}

		fmt.Fprintln(w, "ok!!")
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	wg := &sync.WaitGroup{}
	controllers.InitializeImpressionWorkers(200, 3, wg)

	gin.SetMode(gin.TestMode)
	router := gin.Default()
	router.POST("/noBody", func(c *gin.Context) {
		postImpressionBeacon([]string{"something"}, false)(c)
	})

	res := performRequest(router, "POST", "/noBody", "")
	if res.Code != http.StatusBadRequest {
		t.Error("Should returned 400")
	}

	router.POST("/badApiKey", func(c *gin.Context) {
		postImpressionBeacon([]string{"something"}, false)(c)
	})
	res = performRequest(router, "POST", "/badApiKey", "{\"entries\":[],\"token\":\"some\",\"sdk\":\"test\"}")
	if res.Code != http.StatusUnauthorized {
		t.Error("Should returned 401")
	}

	router.POST("/ok", func(c *gin.Context) {
		postImpressionBeacon([]string{"something"}, false)(c)
	})
	res = performRequest(router, "POST", "/ok", "{\"entries\":[],\"token\":\"something\",\"sdk\":\"something\"}")
	if res.Code != http.StatusNoContent {
		t.Error("Should returned 204")
	}

	res = performRequest(
		router,
		"POST",
		"/ok",
		"{\"entries\": [{\"f\":\"some_test\",\"i\": [{\"k\": \"some_key_1\",\"t\": \"off\",\"m\": 1572026609000,\"c\": 1567008715937,\"r\": \"default rule\"}]}],\"token\":\"something\",\"sdk\":\"something\"}",
	)
	if res.Code != http.StatusNoContent {
		t.Error("Should returned 204")
	}

	res = performRequest(
		router,
		"POST",
		"/ok",
		"{\"entries\": [{\"f\":\"some_test\",\"i\": [{\"k\": \"some_key_2\",\"t\": \"off\",\"m\": 1572026609000,\"c\": 1567008715937,\"r\": \"default rule\"}]}],\"token\":\"something\",\"sdk\":\"something\"}",
	)
	if res.Code != http.StatusNoContent {
		t.Error("Should returned 204")
	}

	// Lets async function post impressions
	time.Sleep(time.Duration(5) * time.Second)
}

func TestPostEventsBeacon(t *testing.T) {
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}
	interfaces.Initialize()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		checkHeaders(t, r)

		rBody, _ := ioutil.ReadAll(r.Body)

		var eventsInPost []dtos.EventDTO
		err := json.Unmarshal(rBody, &eventsInPost)
		if err != nil {
			t.Error(err)
			return
		}

		if eventsInPost[0].Key != "some_key" ||
			eventsInPost[0].EventTypeID != "some_event" ||
			eventsInPost[0].TrafficTypeName != "some_traffic_type" {
			t.Error("Posted events arrived mal-formed")
		}

		fmt.Fprintln(w, "ok!!")
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	wg := &sync.WaitGroup{}

	controllers.InitializeEventWorkers(200, 3, wg)

	gin.SetMode(gin.TestMode)
	router := gin.Default()
	router.POST("/noBody", func(c *gin.Context) {
		postEventsBeacon([]string{"something"})(c)
	})

	res := performRequest(router, "POST", "/noBody", "")
	if res.Code != http.StatusBadRequest {
		t.Error("Should returned 400")
	}

	router.POST("/badApiKey", func(c *gin.Context) {
		postEventsBeacon([]string{"something"})(c)
	})

	res = performRequest(router, "POST", "/badApiKey", "{\"entries\":[],\"token\":\"some\",\"sdk\":\"test\"}")
	if res.Code != http.StatusUnauthorized {
		t.Error("Should returned 401")
	}

	router.POST("/ok", func(c *gin.Context) {
		postEventsBeacon([]string{"something"})(c)
	})
	res = performRequest(router, "POST", "/ok", "{\"entries\":[],\"token\":\"something\",\"sdk\":\"something\"}")
	if res.Code != http.StatusNoContent {
		t.Error("Should returned 204")
	}

	res = performRequest(
		router,
		"POST",
		"/ok",
		"{\"entries\":[{\"eventTypeId\":\"some_event\",\"trafficTypeName\":\"some_traffic_type\",\"value\":null,\"timestamp\":1572017717747,\"key\":\"some_key\",\"properties\":null}],\"token\":\"something\",\"sdk\":\"something\"}",
	)
	if res.Code != http.StatusNoContent {
		t.Error("Should returned 204")
	}

	res = performRequest(
		router,
		"POST",
		"/ok",
		"{\"entries\":[{\"eventTypeId\":\"some_event\",\"trafficTypeName\":\"some_traffic_type\",\"value\":null,\"timestamp\":1572017717747,\"key\":\"some_key\",\"properties\":null}],\"token\":\"something\",\"sdk\":\"something\"}",
	)
	if res.Code != http.StatusNoContent {
		t.Error("Should returned 204")
	}

	// Lets async function post impressions
	time.Sleep(time.Duration(5) * time.Second)
}

func TestPostImpressionsCountBeacon(t *testing.T) {
	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkHeaders(t, r)

		rBody, _ := ioutil.ReadAll(r.Body)

		var impressionsCountInPost dtos.ImpressionsCountDTO
		err := json.Unmarshal(rBody, &impressionsCountInPost)
		if err != nil {
			t.Error(err)
			return
		}

		if impressionsCountInPost.PerFeature[0].FeatureName != "some" || impressionsCountInPost.PerFeature[0].RawCount != 100 {
			t.Error("Wrong payload")
		}

		fmt.Fprintln(w, "ok!!")
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	controllers.InitializeImpressionsCountRecorder()

	gin.SetMode(gin.TestMode)
	router := gin.Default()

	router.POST("/noBody", func(c *gin.Context) {
		postImpressionsCountBeacon([]string{"something"})(c)
	})
	res := performRequest(router, "POST", "/noBody", "")
	if res.Code != http.StatusBadRequest {
		t.Error("Should returned 400")
	}

	router.POST("/badApiKey", func(c *gin.Context) {
		postImpressionsCountBeacon([]string{})(c)
	})
	res = performRequest(router, "POST", "/badApiKey", "{\"entries\": {\"pf\":[]},\"token\":\"some\",\"sdk\":\"test\"}")
	if res.Code != http.StatusUnauthorized {
		t.Error("Should returned 401")
	}

	router.POST("/ok", func(c *gin.Context) {
		postImpressionsCountBeacon([]string{"something"})(c)
	})
	res = performRequest(router, "POST", "/ok", "{\"entries\":{\"pf\":[]},\"token\":\"something\",\"sdk\":\"something\"}")
	if res.Code != http.StatusNoContent {
		t.Error("Should returned 204")
	}

	res = performRequest(
		router,
		"POST",
		"/ok",
		"{\"entries\": {\"pf\":[{\"f\":\"some\",\"rc\":100,\"m\":12345678}]},\"token\":\"something\",\"sdk\":\"something\"}",
	)
	if res.Code != http.StatusNoContent {
		t.Error("Should returned 204")
	}

	// Lets async function post impressions
	time.Sleep(time.Duration(500) * time.Millisecond)
}

func TestAuth(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok!!")
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	if log.Instance == nil {
		stdoutWriter := ioutil.Discard //os.Stdout
		log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, logging.LevelNone)
	}

	gin.SetMode(gin.TestMode)
	router := gin.Default()
	router.GET("/auth", func(c *gin.Context) {
		auth(c)
	})

	res := performRequest(router, "GET", "/auth", "")
	if res.Code != http.StatusOK {
		t.Error("Should returned 200")
	}
	type response struct {
		pushEnabled bool
		token       string
	}

	var body *response
	_ = json.Unmarshal(res.Body.Bytes(), &body)
	if body.pushEnabled {
		t.Error("It should be false")
	}
	if body.token != "" {
		t.Error("Wrong token")
	}
}
