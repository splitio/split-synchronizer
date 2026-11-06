package common

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/splitio/split-synchronizer/v5/splitio/common/conf"

	config "github.com/splitio/go-split-commons/v10/conf"
)

// ParseRedisOptions builds the go-split-commons Redis configuration from the shared Redis options
func ParseRedisOptions(cfg *conf.Redis) (*config.RedisConfig, error) {
	tlsCfg, err := parseTLSConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("error parsing redis tls config options: %w", err)
	}

	redisCfg := &config.RedisConfig{
		Username:     cfg.Username,
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

func parseTLSConfig(opt *conf.Redis) (*tls.Config, error) {
	if !opt.TLS {
		return nil, nil
	}

	cfg := tls.Config{ServerName: opt.TLSServerName}
	if cfg.ServerName == "" {
		cfg.ServerName = opt.Host
	}

	if len(opt.TLSCACertificates) > 0 {
		certPool := x509.NewCertPool()
		for _, cacert := range opt.TLSCACertificates {
			pemData, err := os.ReadFile(cacert)
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
