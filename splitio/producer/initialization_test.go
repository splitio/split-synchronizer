package producer

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	config "github.com/splitio/go-split-commons/v4/conf"
	"github.com/splitio/go-split-commons/v4/dtos"
	"github.com/splitio/go-split-commons/v4/service"
	"github.com/splitio/go-split-commons/v4/service/mocks"
	predis "github.com/splitio/go-split-commons/v4/storage/redis"
	"github.com/splitio/go-toolkit/v5/logging"
	cconf "github.com/splitio/split-synchronizer/v5/splitio/common/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/producer/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/util"
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
		calculated := util.HashAPIKey(apikey)
		if calculated != hash {
			t.Errorf("Apikey %s should hash to %d. Instead got %d", apikey, hash, calculated)
		}
	}
}

func TestIsApikeyValidOk(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "{\"splits\": [], \"since\": -1, \"till\": -1}")
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	httpSplitFetcher := mocks.MockSplitFetcher{
		FetchCall: func(changeNumber int64, fetchOptions *service.FetchOptions) (*dtos.SplitChangesDTO, error) {
			return nil, nil
		},
	}

	if !isValidApikey(httpSplitFetcher) {
		t.Error("APIKEY should be valid.")
	}
}

func TestIsApikeyValidNotOk(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "error", http.StatusNotFound)
	}))
	defer ts.Close()

	os.Setenv("SPLITIO_SDK_URL", ts.URL)
	os.Setenv("SPLITIO_EVENTS_URL", ts.URL)

	httpSplitFetcher := mocks.MockSplitFetcher{
		FetchCall: func(changeNumber int64, fetchOptions *service.FetchOptions) (*dtos.SplitChangesDTO, error) {
			return nil, errors.New("Some")
		},
	}

	if isValidApikey(httpSplitFetcher) {
		t.Error("APIKEY should be invalid.")
	}
}

func TestSanitizeRedisWithForcedCleanup(t *testing.T) {
	cfg := getDefaultConf()
	cfg.Apikey = "983564etyrudhijfgknf9i08euh"
	cfg.Initialization.ForceFreshStartup = true

	logger := logging.NewLogger(nil)

	redisClient, err := predis.NewRedisClient(&config.RedisConfig{
		Host:     "localhost",
		Port:     6379,
		Prefix:   "some_prefix",
		Database: 1,
	}, logger)
	if err != nil {
		t.Error("It should be nil")
	}

	err = redisClient.Set("SPLITIO.test1", "123", 0)
	if err != nil {
		t.Error("It should be nil")
	}
	value, err := redisClient.Get("SPLITIO.test1")
	if value != "123" {
		t.Error("Value should have been set properly")
	}

	miscStorage := predis.NewMiscStorage(redisClient, logger)
	value, err = redisClient.Get("SPLITIO.test1")
	err = sanitizeRedis(cfg, miscStorage, logger)
	if err != nil {
		t.Error("It should be nil", err)
	}

	value, _ = redisClient.Get("SPLITIO.test1")
	if value != "" {
		t.Error("Value should have been null, and was ", value)
	}

	value, err = redisClient.Get("SPLITIO.hash")
	if value != "1497926959" {
		t.Error("Incorrect apikey hash set in redis after sanitization operation.", value)
	}

	redisClient.Del("SPLITIO.hash")
}

func TestSanitizeRedisWithRedisEqualApiKey(t *testing.T) {
	cfg := getDefaultConf()
	cfg.Apikey = "djasghdhjasfganyr73dsah9"

	logger := logging.NewLogger(nil)

	redisClient, err := predis.NewRedisClient(&config.RedisConfig{
		Host:     "localhost",
		Port:     6379,
		Prefix:   "some_prefix",
		Database: 1,
	}, logger)
	if err != nil {
		t.Error("It should be nil")
	}

	redisClient.Set("SPLITIO.test1", "123", 0)
	redisClient.Set("SPLITIO.hash", "3376912823", 0)

	miscStorage := predis.NewMiscStorage(redisClient, logger)
	err = sanitizeRedis(cfg, miscStorage, logger)
	if err != nil {
		t.Error("No error should have occured.")
	}

	val, _ := redisClient.Get("SPLITIO.test1")
	if val != "123" {
		t.Error("Value should not have been removed!")
	}

	val, _ = redisClient.Get("SPLITIO.hash")
	if val != "3376912823" {
		t.Error("Incorrect apikey hash set in redis after sanitization operation.")
	}

	redisClient.Del("SPLITIO.test1")
}

func TestSanitizeRedisWithRedisDifferentApiKey(t *testing.T) {
	cfg := getDefaultConf()
	cfg.Apikey = "983564etyrudhijfgknf9i08euh"

	logger := logging.NewLogger(nil)

	redisClient, err := predis.NewRedisClient(&config.RedisConfig{
		Host:     "localhost",
		Port:     6379,
		Prefix:   "some_prefix",
		Database: 1,
	}, logger)
	if err != nil {
		t.Error("It should be nil")
	}

	redisClient.Set("SPLITIO.test1", "123", 0)
	redisClient.Set("SPLITIO.hash", "3376912823", 0)

	miscStorage := predis.NewMiscStorage(redisClient, logger)
	err = sanitizeRedis(cfg, miscStorage, logger)
	if err != nil {
		t.Error("No error should have occured.")
	}

	val, _ := redisClient.Get("SPLITIO.test1")
	if val != "" {
		t.Error("Value should have been removed!")
	}

	val, _ = redisClient.Get("SPLITIO.hash")
	if val != "1497926959" {
		t.Error("Incorrect apikey hash set in redis after sanitization operation.")
	}

	redisClient.Del("SPLITIO.test1")
}

func getDefaultConf() *conf.Main {
	var c conf.Main
	cconf.PopulateDefaults(&c)
	return &c
}
