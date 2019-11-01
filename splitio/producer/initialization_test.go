package producer

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
	"github.com/splitio/split-synchronizer/splitio/api"
	"github.com/splitio/split-synchronizer/splitio/fetcher"
	"github.com/splitio/split-synchronizer/splitio/storage/redis"
)

func TestHashApiKey(t *testing.T) {
	testCases := map[string]uint32{
		"djasghdhjasfganyr73dsah9":        3376912823,
		"983564etyrudhijfgknf9i08euh":     1497926959,
		"fnhbsgyry738suirjnklm;,eokp3isr": 3290600706,
		"nfihua9380oekrjnuh9229i":         2236673735,
		"fjngrsy87398oji4grnkfjei":        866589948,
	}

	for apikey, hash := range testCases {
		calculated := hashApiKey(apikey)
		if calculated != hash {
			t.Errorf("Apikey %s should hash to %d. Instead got %d", apikey, hash, calculated)
		}
	}
}

func TestIsApikeyValidOk(t *testing.T) {
	stdoutWriter := ioutil.Discard
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "{\"splits\": [], \"since\": -1, \"till\": -1}")
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()

	httpSplitFetcher := fetcher.NewHTTPSplitFetcher()
	if !isApikeyValid(httpSplitFetcher) {
		t.Error("APIKEY should be valid.")
	}
}

func TestIsApikeyValidNotOk(t *testing.T) {
	stdoutWriter := ioutil.Discard
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "error", http.StatusNotFound)
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	api.Initialize()

	httpSplitFetcher := fetcher.NewHTTPSplitFetcher()
	if isApikeyValid(httpSplitFetcher) {
		t.Error("APIKEY should be invalid.")
	}
}

func TestSanitizeRedisWithForcedCleanup(t *testing.T) {
	stdoutWriter := ioutil.Discard
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	conf.Data.APIKey = "983564etyrudhijfgknf9i08euh"
	conf.Data.Redis.ForceFreshStartup = true
	conf.Data.Redis.Prefix = "some_prefix"
	conf.Data.Redis.Db = 1
	redis.Initialize(conf.Data.Redis)
	redis.Client.Set("some_prefix.SPLITIO.test1", "123", 0)
	if redis.Client.Get("some_prefix.SPLITIO.test1").Val() != "123" {
		t.Error("Value should have been set properly")
	}

	sanitizeRedis()
	if val := redis.Client.Get("some_prefix.SPLITIO.test1").Val(); val != "" {
		t.Error("Value should have been null, and was ", val)
	}

	if val := redis.Client.Get("some_prefix.SPLITIO.hash").Val(); val != "1497926959" {
		t.Error("Incorrect apikey hash set in redis after sanitization operation.")
	}

	redis.Client.Del("some_prefix.SPLITIO.hash")
}

func TestSanitizeRedisWithRedisError(t *testing.T) {
	stdoutWriter := ioutil.Discard
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	conf.Data.Redis.Port = 1234
	conf.Data.Redis.Prefix = "some_prefix"
	conf.Data.Redis.Db = 1
	redis.Initialize(conf.Data.Redis)
	redis.Client.Set("some_prefix.SPLITIO.test1", "123", 0)

	err := sanitizeRedis()
	if err == nil {
		t.Error("An error should have been returned for incorrect redis config")
	}

	if val := redis.Client.Get("some_prefix.SPLITIO.hash").Val(); val != "" {
		t.Error("Incorrect apikey hash set in redis after sanitization operation.")
	}

}

func TestSanitizeRedisWithRedisEqualApiKey(t *testing.T) {
	stdoutWriter := ioutil.Discard
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	conf.Data.APIKey = "djasghdhjasfganyr73dsah9"
	conf.Data.Redis.Port = 6379
	conf.Data.Redis.Prefix = "some_prefix"
	conf.Data.Redis.Db = 1
	redis.Initialize(conf.Data.Redis)
	redis.Client.Set("some_prefix.SPLITIO.test1", "123", 0)
	redis.Client.Set("some_prefix.SPLITIO.hash", "3376912823", 0)

	err := sanitizeRedis()
	if err != nil {
		t.Error("No error should have occured.")
	}

	if redis.Client.Get("some_prefix.SPLITIO.test1").Val() != "123" {
		t.Error("Value should not have been removed!")
	}

	if val := redis.Client.Get("some_prefix.SPLITIO.hash").Val(); val != "3376912823" {
		t.Error("Incorrect apikey hash set in redis after sanitization operation.")
	}

	redis.Client.Del("some_prefix.SPLITIO.test1")
}

func TestSanitizeRedisWithRedisDifferentApiKey(t *testing.T) {
	stdoutWriter := ioutil.Discard
	log.Initialize(stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter, stdoutWriter)

	conf.Initialize()
	conf.Data.APIKey = "983564etyrudhijfgknf9i08euh"
	conf.Data.Redis.Port = 6379
	conf.Data.Redis.Prefix = "some_prefix"
	conf.Data.Redis.Db = 1
	redis.Initialize(conf.Data.Redis)
	redis.Client.Set("some_prefix.SPLITIO.test1", "123", 0)
	redis.Client.Set("some_prefix.SPLITIO.hash", "3376912823", 0)

	err := sanitizeRedis()
	if err != nil {
		t.Error("No error should have occured.")
	}

	if redis.Client.Get("some_prefix.SPLITIO.test1").Val() != "" {
		t.Error("Value should have been removed!")
	}

	if val := redis.Client.Get("some_prefix.SPLITIO.hash").Val(); val != "1497926959" {
		t.Error("Incorrect apikey hash set in redis after sanitization operation.")
	}

	redis.Client.Del("some_prefix.SPLITIO.test1")
}
