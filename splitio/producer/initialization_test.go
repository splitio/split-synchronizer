package producer

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	cconf "github.com/splitio/split-synchronizer/v5/splitio/common/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/producer/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/util"

	config "github.com/splitio/go-split-commons/v8/conf"
	"github.com/splitio/go-split-commons/v8/service/mocks"
	predis "github.com/splitio/go-split-commons/v8/storage/redis"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/stretchr/testify/mock"
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

	httpSplitFetcher := &mocks.MockSplitFetcher{}
	httpSplitFetcher.On("Fetch", mock.Anything).Return(nil, nil)

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

	httpSplitFetcher := &mocks.MockSplitFetcher{}
	httpSplitFetcher.On("Fetch", mock.Anything).Return(nil, errors.New("Some"))

	if isValidApikey(httpSplitFetcher) {
		t.Error("APIKEY should be invalid.")
	}
}

func TestSanitizeRedisWithForcedCleanup(t *testing.T) {
	cfg := getDefaultConf()
	cfg.Apikey = "983564etyrudhijfgknf9i08euh"
	cfg.FlagSpecVersion = "1.0"
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
	value, _ := redisClient.Get("SPLITIO.test1")
	if value != "123" {
		t.Error("Value should have been set properly")
	}

	miscStorage := predis.NewMiscStorage(redisClient, logger)
	err = sanitizeRedis(cfg, miscStorage, logger)
	if err != nil {
		t.Error("It should be nil", err)
	}

	value, _ = redisClient.Get("SPLITIO.test1")
	if value != "" {
		t.Error("Value should have been null, and was ", value)
	}

	value, _ = redisClient.Get("SPLITIO.hash")
	if value != "2298020180" {
		t.Error("Incorrect apikey hash set in redis after sanitization operation.", value)
	}

	redisClient.Del("SPLITIO.hash")
}

func TestSanitizeRedisWithRedisEqualApiKey(t *testing.T) {
	cfg := getDefaultConf()
	cfg.Apikey = "983564etyrudhijfgknf9i08euh"
	cfg.FlagSpecVersion = "1.0"

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
	hash := util.HashAPIKey(cfg.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))

	redisClient.Set("SPLITIO.test1", "123", 0)
	redisClient.Set("SPLITIO.hash", hash, 0)

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
	if val != strconv.FormatUint(uint64(hash), 10) {
		t.Error("Incorrect apikey hash set in redis after sanitization operation.")
	}

	redisClient.Del("SPLITIO.hash")
	redisClient.Del("SPLITIO.test1")
}

func TestSanitizeRedisWithRedisDifferentApiKey(t *testing.T) {
	cfg := getDefaultConf()
	cfg.Apikey = "983564etyrudhijfgknf9i08euh"
	cfg.FlagSpecVersion = "1.0"

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
	hash := util.HashAPIKey("djasghdhjasfganyr73dsah9" + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))

	redisClient.Set("SPLITIO.test1", "123", 0)
	redisClient.Set("SPLITIO.hash", "3216514561", 0)

	hash = util.HashAPIKey(cfg.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))

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
	if val != strconv.FormatUint(uint64(hash), 10) {
		t.Error("Incorrect apikey hash set in redis after sanitization operation.", val)
	}

	redisClient.Del("SPLITIO.hash")
	redisClient.Del("SPLITIO.test1")
}

func TestSanitizeRedisWithForcedCleanupByFlagSets(t *testing.T) {
	cfg := getDefaultConf()
	cfg.FlagSpecVersion = "1.0"
	cfg.Apikey = "983564etyrudhijfgknf9i08euh"
	cfg.Initialization.ForceFreshStartup = true
	cfg.FlagSetsFilter = []string{"flagset1", "flagset2"}

	hash := util.HashAPIKey(cfg.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))

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
	redisClient.Set("SPLITIO.hash", hash, 0)
	if err != nil {
		t.Error("It should be nil")
	}
	value, _ := redisClient.Get("SPLITIO.test1")
	if value != "123" {
		t.Error("Value should have been set properly")
	}

	cfg.FlagSetsFilter = []string{"flagset7"}
	miscStorage := predis.NewMiscStorage(redisClient, logger)
	err = sanitizeRedis(cfg, miscStorage, logger)
	if err != nil {
		t.Error("It should be nil", err)
	}

	value, _ = redisClient.Get("SPLITIO.test1")
	if value != "" {
		t.Error("Value should have been removed.")
	}

	val, _ := redisClient.Get("SPLITIO.hash")
	parsedHash, _ := strconv.ParseUint(val, 10, 64)
	if uint32(parsedHash) == hash {
		t.Error("ApiHash should have been updated.")
	}
	redisClient.Del("SPLITIO.hash")
	redisClient.Del("SPLITIO.test1")
}

func TestSanitizeRedisWithForcedCleanupBySpecVersion(t *testing.T) {
	cfg := getDefaultConf()
	cfg.Apikey = "983564etyrudhijfgknf9i08euh"
	cfg.Initialization.ForceFreshStartup = true
	cfg.FlagSpecVersion = "1.0"

	hash := util.HashAPIKey(cfg.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))

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
	redisClient.Set("SPLITIO.hash", hash, 0)
	if err != nil {
		t.Error("It should be nil")
	}
	value, _ := redisClient.Get("SPLITIO.test1")
	if value != "123" {
		t.Error("Value should have been set properly")
	}

	cfg.FlagSpecVersion = "1.1"
	miscStorage := predis.NewMiscStorage(redisClient, logger)
	err = sanitizeRedis(cfg, miscStorage, logger)
	if err != nil {
		t.Error("It should be nil", err)
	}

	value, _ = redisClient.Get("SPLITIO.test1")
	if value != "" {
		t.Error("Value should have been removed.")
	}

	val, _ := redisClient.Get("SPLITIO.hash")
	parsedHash, _ := strconv.ParseUint(val, 10, 64)
	if uint32(parsedHash) == hash {
		t.Error("ApiHash should have been updated.")
	}
	redisClient.Del("SPLITIO.hash")
	redisClient.Del("SPLITIO.test1")
}

func getDefaultConf() *conf.Main {
	var c conf.Main
	cconf.PopulateDefaults(&c)
	return &c
}
