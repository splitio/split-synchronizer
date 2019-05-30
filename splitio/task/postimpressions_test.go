// Package task contains all agent tasks
package task

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/recorder"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
)

/* ImpressionStorage for testing */
type testImpressionStorage struct{}

func (r testImpressionStorage) RetrieveImpressions(count int64, legacyDisabled bool) (map[api.SdkMetadata][]api.ImpressionsDTO, error) {
	imp1 := api.ImpressionDTO{KeyName: "some_key_1", Treatment: "on", Time: 1234567890, ChangeNumber: 9876543210, Label: "some_label_1", BucketingKey: "some_bucket_key_1"}
	imp2 := api.ImpressionDTO{KeyName: "some_key_2", Treatment: "off", Time: 1234567890, ChangeNumber: 9876543210, Label: "some_label_2", BucketingKey: "some_bucket_key_2"}

	keyImpressions := make([]api.ImpressionDTO, 0)
	keyImpressions = append(keyImpressions, imp1, imp2)
	impressionsTest := api.ImpressionsDTO{TestName: "some_test", KeyImpressions: keyImpressions}
	metadata := api.SdkMetadata{
		SdkVersion: "test-2.0",
		MachineIP:  "127.0.0.1",
	}

	return map[api.SdkMetadata][]api.ImpressionsDTO{metadata: {impressionsTest}}, nil
}

func (r testImpressionStorage) Size() int64 {
	return 0
}

/* ImpressionsRecorder for testing */
type testImpressionsRecorder struct{}

func (r testImpressionsRecorder) Post(impressions []api.ImpressionsDTO, metadata api.SdkMetadata) error {
	return nil
}

func TestTaskPostImpressions(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	//Initialize by default
	conf.Initialize()

	tid := 1
	impressionsRecorderAdapter := testImpressionsRecorder{}
	impressionStorageAdapter := testImpressionStorage{}
	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		taskPostImpressions(tid, impressionsRecorderAdapter, impressionStorageAdapter, conf.Data.ImpressionsPerPost, true, false)
	}()
}

func TestFlushImpressions(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	size := 4

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		rBody, _ := ioutil.ReadAll(r.Body)

		var impressionsInPost []redis.ImpressionDTO
		err := json.Unmarshal(rBody, &impressionsInPost)
		if err != nil {
			t.Error(err)
			return
		}

	}))

	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	// API initilization
	api.Initialize()

	//Initialize by default
	conf.Initialize()

	conf.Data.Redis.Prefix = "postimpressionsintest"

	//Redis storage by default
	redis.Initialize(conf.Data.Redis)

	//INSERT MOCK DATA
	//----------------
	itemsToAdd := 5
	impressionListName := conf.Data.Redis.Prefix + ".SPLITIO.impressions"

	impressionJSON := `{"m":{"s":"test-1.0.0","i":"127.0.0.1","n":"SOME_MACHINE_NAME"},"i":{"k":"6c4829ab-a0d8-4e72-8176-a334f596fb79","b":"bucketing","f":"feature","t":"ON","c":12345,"r":"rule","timestamp":1516310749882}}`

	//Deleting previous test data
	res := redis.Client.Del(impressionListName)
	if res.Err() != nil {
		t.Error(res.Err().Error())
		return
	}

	//Pushing 5 impressions
	for i := 0; i < itemsToAdd; i++ {
		redis.Client.RPush(impressionListName, impressionJSON)
	}
	//----------------

	impressionRecorderAdapter := recorder.ImpressionsHTTPRecorder{}
	impressionStorageAdapter := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		count := int64(size)
		ImpressionsFlush(impressionRecorderAdapter, impressionStorageAdapter, &count, conf.Data.Redis.DisableLegacyImpressions, true)
		total := impressionStorageAdapter.Size()
		if total != 1 {
			t.Error("It should kept 1 element, but there are:", total)
		}
	}()
}

func TestFlushImpressionsNilSize(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		rBody, _ := ioutil.ReadAll(r.Body)

		var impressionsInPost []redis.ImpressionDTO
		err := json.Unmarshal(rBody, &impressionsInPost)
		if err != nil {
			t.Error(err)
			return
		}

	}))

	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	// API initilization
	api.Initialize()

	//Initialize by default
	conf.Initialize()

	conf.Data.Redis.Prefix = "postimpressionsintest"

	//Redis storage by default
	redis.Initialize(conf.Data.Redis)

	//INSERT MOCK DATA
	//----------------
	impressionListName := conf.Data.Redis.Prefix + ".SPLITIO.impressions"

	//Deleting previous test data
	res := redis.Client.Del(impressionListName)
	if res.Err() != nil {
		t.Error(res.Err().Error())
		return
	}

	impressionJSON := `{"m":{"s":"test-1.0.0","i":"127.0.0.1","n":"SOME_MACHINE_NAME"},"i":{"k":"6c4829ab-a0d8-4e72-8176-a334f596fb79","b":"bucketing","f":"feature","t":"ON","c":12345,"r":"rule","timestamp":1516310749882}}`

	impressionsToStore := make([]interface{}, 50001)
	for index := range impressionsToStore {
		impressionsToStore[index] = impressionJSON
	}
	redis.Client.RPush(impressionListName, impressionsToStore...)

	impressionRecorderAdapter := recorder.ImpressionsHTTPRecorder{}
	impressionStorageAdapter := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		ImpressionsFlush(impressionRecorderAdapter, impressionStorageAdapter, nil, conf.Data.Redis.DisableLegacyImpressions, true)
		total := impressionStorageAdapter.Size()
		if total != 25001 {
			t.Error("It should flush 25000 elements, but there are:", total)
		}
	}()
}

func TestFlushImpressionsInBatches(t *testing.T) {
	stdoutWriter := ioutil.Discard //os.Stdout
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	size := 10001

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		rBody, _ := ioutil.ReadAll(r.Body)

		var impressionsInPost []redis.ImpressionDTO
		err := json.Unmarshal(rBody, &impressionsInPost)
		if err != nil {
			t.Error(err)
			return
		}

	}))

	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	// API initilization
	api.Initialize()

	//Initialize by default
	conf.Initialize()

	conf.Data.Redis.Prefix = "postimpressionsintest"

	//Redis storage by default
	redis.Initialize(conf.Data.Redis)

	//INSERT MOCK DATA
	//----------------
	itemsToAdd := 10003
	impressionListName := conf.Data.Redis.Prefix + ".SPLITIO.impressions"

	impressionJSON := `{"m":{"s":"test-1.0.0","i":"127.0.0.1","n":"SOME_MACHINE_NAME"},"i":{"k":"6c4829ab-a0d8-4e72-8176-a334f596fb79","b":"bucketing","f":"feature","t":"ON","c":12345,"r":"rule","timestamp":1516310749882}}`

	//Deleting previous test data
	res := redis.Client.Del(impressionListName)
	if res.Err() != nil {
		t.Error(res.Err().Error())
		return
	}

	//Pushing 10001 impressions
	for i := 0; i < itemsToAdd; i++ {
		redis.Client.RPush(impressionListName, impressionJSON)
	}
	//----------------

	impressionRecorderAdapter := recorder.ImpressionsHTTPRecorder{}
	impressionStorageAdapter := redis.NewImpressionStorageAdapter(redis.Client, conf.Data.Redis.Prefix)
	//Catching panic status and reporting error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Error("Recovered task", r)
			}
		}()
		count := int64(size)
		ImpressionsFlush(impressionRecorderAdapter, impressionStorageAdapter, &count, conf.Data.Redis.DisableLegacyImpressions, true)
		total := impressionStorageAdapter.Size()
		if total != 2 {
			t.Error("It should kept 2 element, but there are:", total)
		}
	}()

	redis.Client.Del("postimpressionsintest.SPLITIO.impressions")
}
