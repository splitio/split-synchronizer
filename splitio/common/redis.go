package common

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/splitio/split-synchronizer/v5/splitio/common/conf"

	config "github.com/splitio/go-split-commons/v10/conf"
)

// ParseRedisOptions builds the Redis configuration for the synchronizer. It keeps the behavior the
// synchronizer always had: no TLS minimum version and no start-time validation of the options.
func ParseRedisOptions(cfg *conf.Redis) (*config.RedisConfig, error) {
	return parseRedisOptions(cfg, false)
}

// ParseRedisStateOptions builds the Redis configuration for the proxy Redis state connection. On top of
// ParseRedisOptions it sets a TLS 1.2 floor and validates the port, sentinel addresses/master and cluster nodes.
func ParseRedisStateOptions(cfg *conf.Redis) (*config.RedisConfig, error) {
	return parseRedisOptions(cfg, true)
}

func parseRedisOptions(cfg *conf.Redis, strict bool) (*config.RedisConfig, error) {
	tlsCfg, err := parseTLSConfig(cfg, strict)
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
		if !strict {
			redisCfg.SentinelAddresses = strings.Split(cfg.SentinelAddresses, ",")
			redisCfg.SentinelMaster = cfg.SentinelMaster
			return redisCfg, nil
		}
		redisCfg.SentinelAddresses = splitAddresses(cfg.SentinelAddresses)
		if len(redisCfg.SentinelAddresses) == 0 {
			return nil, errors.New("sentinel addresses must not be empty")
		}
		redisCfg.SentinelMaster = strings.TrimSpace(cfg.SentinelMaster)
		if redisCfg.SentinelMaster == "" {
			return nil, errors.New("sentinel master must not be empty")
		}
	} else if cfg.ClusterMode {
		redisCfg.ClusterKeyHashTag = cfg.ClusterKeyHashTag
		if !strict {
			redisCfg.ClusterNodes = strings.Split(cfg.ClusterNodes, ",")
			return redisCfg, nil
		}
		redisCfg.ClusterNodes = splitAddresses(cfg.ClusterNodes)
		if len(redisCfg.ClusterNodes) == 0 {
			return nil, errors.New("cluster nodes must not be empty")
		}
	} else {
		if strict && (cfg.Port < 1 || cfg.Port > 65535) {
			return nil, fmt.Errorf("redis port must be 1..65535, got %d", cfg.Port)
		}
		redisCfg.Host = cfg.Host
		redisCfg.Port = cfg.Port
		redisCfg.Database = cfg.Db
	}
	return redisCfg, nil
}

// splitAddresses splits a comma-separated list, trimming whitespace and dropping empty elements
func splitAddresses(list string) []string {
	var out []string
	for _, part := range strings.Split(list, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// firstAddress returns the first element of addrs, or an empty string when the list is empty
func firstAddress(addrs []string) string {
	if len(addrs) == 0 {
		return ""
	}
	return addrs[0]
}

// hostOf strips the port from a host:port address, returning the whole string when it has no port
func hostOf(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
			return addr[1 : len(addr)-1]
		}
		return addr
	}
	return host
}

// addressServerName returns the host of the first sentinel address, or of the first cluster node, or an empty string
func addressServerName(opt *conf.Redis) string {
	if opt.SentinelReplication {
		return hostOf(firstAddress(splitAddresses(opt.SentinelAddresses)))
	}
	if opt.ClusterMode {
		return hostOf(firstAddress(splitAddresses(opt.ClusterNodes)))
	}
	return ""
}

// parseTLSConfig builds the TLS config; strict adds the TLS 1.2 floor and the sentinel/cluster server name default
func parseTLSConfig(opt *conf.Redis, strict bool) (*tls.Config, error) {
	if !opt.TLS {
		return nil, nil
	}

	cfg := tls.Config{ServerName: opt.TLSServerName}
	if strict {
		cfg.MinVersion = tls.VersionTLS12
	}
	if cfg.ServerName == "" && strict {
		cfg.ServerName = addressServerName(opt)
	}
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
