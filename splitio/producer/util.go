package producer

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	config "github.com/splitio/go-split-commons/v4/conf"
	"github.com/splitio/go-split-commons/v4/service"
	"github.com/splitio/go-split-commons/v4/storage/redis"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/producer/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/util"
)

func parseTLSConfig(opt *conf.Redis) (*tls.Config, error) {
	if !opt.TLS {
		return nil, nil
	}

	cfg := tls.Config{}
	if opt.SentinelReplication || opt.ClusterMode {
		cfg.ServerName = ""
	} else if opt.TLSServerName != "" {
		cfg.ServerName = opt.TLSServerName
	} else {
		cfg.ServerName = opt.Host
	}

	if len(opt.TLSCACertificates) > 0 {
		certPool := x509.NewCertPool()
		for _, cacert := range opt.TLSCACertificates {
			pemData, err := ioutil.ReadFile(cacert)
			if err != nil {
				return nil, fmt.Errorf("failed to load root certificate: %w", err)
			}
			ok := certPool.AppendCertsFromPEM(pemData)
			if !ok {
				return nil, fmt.Errorf("failed to add certificate %s to the TLS configuration: ", cacert)
			}
		}
		cfg.RootCAs = certPool
	}

	cfg.InsecureSkipVerify = opt.TLSSkipNameValidation

	if opt.TLSClientKey != "" && opt.TLSClientCertificate != "" {
		certPair, err := tls.LoadX509KeyPair(
			opt.TLSClientCertificate,
			opt.TLSClientKey,
		)

		if err != nil {
			return nil, fmt.Errorf("unable to load client certificate and private key: %w", err)
		}

		cfg.Certificates = []tls.Certificate{certPair}
	} else if opt.TLSClientKey != opt.TLSClientCertificate {
		// If they aren't both set, and they aren't equal, it means that only one is set, which is invalid.
		return nil, errors.New("You must provide either both client certificate and client private key, or none")
	}

	return &cfg, nil
}

func parseRedisOptions(cfg *conf.Redis) (*config.RedisConfig, error) {
	tlsCfg, err := parseTLSConfig(cfg)
	if err != nil {
		return nil, errors.New("Error in Redis TLS Configuration")
	}

	redisCfg := &config.RedisConfig{
		Password:     cfg.Pass,
		Prefix:       cfg.Prefix,
		Network:      cfg.Network,
		MaxRetries:   cfg.MaxRetries,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		PoolSize:     cfg.PoolSize,
		TLSConfig:    tlsCfg,
	}

	if cfg.SentinelReplication {
		redisCfg.SentinelAddresses = strings.Split(cfg.SentinelAddresses, ",")
		redisCfg.SentinelMaster = cfg.SentinelMaster
	} else if cfg.ClusterMode {
		redisCfg.ClusterKeyHashTag = cfg.ClusterKeyHashTag
		redisCfg.ClusterNodes = strings.Split(cfg.ClusterNodes, ",")
	} else {
		redisCfg.Host = cfg.Host
		redisCfg.Port = cfg.Port
		redisCfg.Database = cfg.Db
	}
	return redisCfg, nil
}

func isValidApikey(splitFetcher service.SplitFetcher) bool {
	_, err := splitFetcher.Fetch(time.Now().UnixNano()/int64(time.Millisecond), false)
	return err == nil
}

func sanitizeRedis(cfg *conf.Main, miscStorage *redis.MiscStorage, logger logging.LoggerInterface) error {
	if miscStorage == nil {
		return errors.New("Could not sanitize redis")
	}
	currentHash := util.HashAPIKey(cfg.Apikey)
	currentHashAsStr := strconv.Itoa(int(currentHash))
	defer miscStorage.SetApikeyHash(currentHashAsStr)

	if cfg.Initialization.ForceFreshStartup {
		logger.Warning("Fresh startup requested. Cleaning up redis before initializing.")
		miscStorage.ClearAll()
		return nil
	}

	previousHashStr, err := miscStorage.GetApikeyHash()
	if err != nil && err.Error() != redis.ErrorHashNotPresent { // Missing hash is not considered an error
		return err
	}

	if currentHashAsStr != previousHashStr {
		logger.Warning("Previous apikey is missing/different from current one. Cleaning up redis before startup.")
		miscStorage.ClearAll()
	}
	return nil
}
