package util

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/splitio/split-synchronizer/v5/splitio/common/conf"
)

var (
	ErrTLSEmptyCertOrPK  = errors.New("when TLS is enabled, server certificate chain & server private key parameters are mandatory")
	ErrTLSInvalidVersion = errors.New("invalid TLS version")
)

func TLSConfigForServer(cfg *conf.TLS) (*tls.Config, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	if cfg.CertChainFN == "" || cfg.PrivateKeyFN == "" {
		return nil, ErrTLSEmptyCertOrPK
	}

	version, err := parseMinTLSVersion(cfg.MinTLSVersion)
	if err != nil {
		return nil, fmt.Errorf("error parsing min tls version: %w", err)
	}

	cert, err := tls.LoadX509KeyPair(cfg.CertChainFN, cfg.PrivateKeyFN)
	if err != nil {
		return nil, fmt.Errorf("error loading cert/key pair: %w", err)
	}

	tlsConfig := &tls.Config{
		ServerName:   cfg.ServerName,
		MinVersion:   version,
		Certificates: []tls.Certificate{cert},
	}

	if len(cfg.AllowedCipherSuites) > 0 {
		suites, err := parseCipherSuites(strings.Split(cfg.AllowedCipherSuites, ","))
		if err != nil {
			return nil, fmt.Errorf("error parsing cipher suites: %w", err)
		}
		tlsConfig.CipherSuites = suites
	}

	if !cfg.ClientValidation {
		return tlsConfig, nil
	}

	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	if cfg.ClientValidationRootCert != "" {
		certBytes, err := os.ReadFile(cfg.ClientValidationRootCert)
		if err != nil {
			return nil, fmt.Errorf("error reading root certificate for client validation")
		}

		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(certBytes)
		tlsConfig.ClientCAs = certPool
	}

	return tlsConfig, nil
}

func parseMinTLSVersion(version string) (uint16, error) {
	switch version {
	case "1.0":
		return tls.VersionTLS10, nil
	case "1.1":
		return tls.VersionTLS11, nil
	case "1.2":
		return tls.VersionTLS12, nil
	case "1.3":
		return tls.VersionTLS13, nil
	}
	return 0, ErrTLSInvalidVersion
}

func parseCipherSuites(strSuites []string) ([]uint16, error) {
	valid := tls.CipherSuites()
	requested := make([]uint16, 0, len(strSuites))
	for _, suite := range strSuites {
		suite = strings.TrimSpace(suite)
		found := false
		for _, current := range valid {
			if current.Name == suite {
				requested = append(requested, current.ID)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("cipher suite '%s' not found in list of secure ones", suite)
		}
	}
	return requested, nil
}
