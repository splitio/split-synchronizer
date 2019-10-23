package redis

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/log"
)

// Client is a redis client with a connection pool
var Client redis.UniversalClient

const clearAllSCriptTemplate = `
	local toDelete = redis.call('KEYS', '{KEY_NAMESPACE}')
	local count = 0
	for key in impkeys do
	    redis.call('DEL', key)
	    count = count + 1
	end
	return count
`

// BaseStorageAdapter basic redis storage adapter
type BaseStorageAdapter struct {
	*prefixAdapter
	client redis.UniversalClient
}

// Initialize Redis module with a pool connection
func Initialize(redisOptions conf.RedisSection) error {
	var err error
	Client, err = NewInstance(redisOptions)
	return err
}

func parseTLSConfig(opt conf.RedisSection) (*tls.Config, error) {
	if !opt.TLS {
		return nil, nil
	}

	if opt.SentinelReplication || opt.ClusterMode {
		return nil, errors.New("TLS encryption cannot be used with Sentinel replication or Cluster mode enabled")
	}

	cfg := tls.Config{}

	if opt.TLSServerName != "" {
		cfg.ServerName = opt.TLSServerName
	} else {
		cfg.ServerName = opt.Host
	}

	if len(opt.TLSCACertificates) > 0 {
		certPool := x509.NewCertPool()
		for _, cacert := range opt.TLSCACertificates {
			pemData, err := ioutil.ReadFile(cacert)
			if err != nil {
				log.Error.Println(fmt.Sprintf("Failed to load Root CA certificate: %s", cacert))
				return nil, err
			}
			ok := certPool.AppendCertsFromPEM(pemData)
			if !ok {
				log.Error.Println(fmt.Sprintf("Failed to add certificate %s to the TLS configuration", cacert))
				return nil, fmt.Errorf("Couldn't add certificate %s to redis TLS configuration", cacert)
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
			log.Error.Println("Unable to load client certificate and private key")
			return nil, err
		}

		cfg.Certificates = []tls.Certificate{certPair}
	} else if opt.TLSClientKey != opt.TLSClientCertificate {
		// If they aren't both set, and they aren't equal, it means that only one is set, which is invalid.
		return nil, errors.New("You must provide either both client certificate and client private key, or none")
	}

	return &cfg, nil
}

// NewInstance returns an instance of Redis Client
func NewInstance(opt conf.RedisSection) (redis.UniversalClient, error) {

	tlsCfg, err := parseTLSConfig(opt)
	if err != nil {
		return nil, errors.New("Error in Redis TLS Configuration")
	}

	if opt.SentinelReplication && opt.ClusterMode {
		return nil, errors.New("Incompatible configuration of redis, Sentinel and Cluster cannot be enabled at the same time")
	}

	if opt.SentinelReplication {
		if opt.SentinelMaster == "" {
			return nil, errors.New("Missing redis sentinel master name")
		}

		if opt.SentinelAddresses == "" {
			return nil, errors.New("Missing redis sentinels addresses")
		}

		addresses := strings.Split(opt.SentinelAddresses, ",")

		return redis.NewUniversalClient(
			&redis.UniversalOptions{
				MasterName:   opt.SentinelMaster,
				Addrs:        addresses,
				Password:     opt.Pass,
				DB:           opt.Db,
				MaxRetries:   opt.MaxRetries,
				PoolSize:     opt.PoolSize,
				DialTimeout:  time.Duration(opt.DialTimeout) * time.Second,
				ReadTimeout:  time.Duration(opt.ReadTimeout) * time.Second,
				WriteTimeout: time.Duration(opt.WriteTimeout) * time.Second,
				TLSConfig:    tlsCfg,
			}), nil
	}

	if opt.ClusterMode {
		if opt.ClusterNodes == "" {
			return nil, errors.New("Missing redis cluster addresses")
		}

		var keyHashTag = "{SPLITIO}"

		if opt.ClusterKeyHashTag != "" {
			keyHashTag = opt.ClusterKeyHashTag
			if len(keyHashTag) < 3 ||
				string(keyHashTag[0]) != "{" ||
				string(keyHashTag[len(keyHashTag)-1]) != "}" ||
				strings.Count(keyHashTag, "{") != 1 ||
				strings.Count(keyHashTag, "}") != 1 {
				return nil, errors.New("keyHashTag is not valid")
			}
		}

		conf.Data.Redis.Prefix = keyHashTag + opt.Prefix

		addresses := strings.Split(opt.ClusterNodes, ",")

		return redis.NewUniversalClient(
			&redis.UniversalOptions{
				Addrs:        addresses,
				Password:     opt.Pass,
				PoolSize:     opt.PoolSize,
				DialTimeout:  time.Duration(opt.DialTimeout) * time.Second,
				ReadTimeout:  time.Duration(opt.ReadTimeout) * time.Second,
				WriteTimeout: time.Duration(opt.WriteTimeout) * time.Second,
				TLSConfig:    tlsCfg,
			}), nil
	}

	return redis.NewUniversalClient(
		&redis.UniversalOptions{
			// Network:      opt.Network,
			Addrs:        []string{fmt.Sprintf("%s:%d", opt.Host, opt.Port)},
			Password:     opt.Pass,
			DB:           opt.Db,
			MaxRetries:   opt.MaxRetries,
			PoolSize:     opt.PoolSize,
			DialTimeout:  time.Duration(opt.DialTimeout) * time.Second,
			ReadTimeout:  time.Duration(opt.ReadTimeout) * time.Second,
			WriteTimeout: time.Duration(opt.WriteTimeout) * time.Second,
			TLSConfig:    tlsCfg,
		}), nil
}

// Size return the value of LLEN
func (b BaseStorageAdapter) Size(nameSpace string) int64 {
	llen := b.client.LLen(nameSpace)

	if llen.Err() != nil {
		log.Error.Println(llen.Err())
		return 0
	}

	return llen.Val()
}

// Drop removes elements from queue
func (b BaseStorageAdapter) Drop(nameSpace string, size *int64) error {
	if size == nil {
		b.client.Del(nameSpace)
		return nil
	}
	b.client.LTrim(nameSpace, *size, -1)
	return nil
}
