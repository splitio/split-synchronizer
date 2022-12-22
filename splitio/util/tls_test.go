package util

import (
	"crypto/tls"
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/common/conf"
)

func TestTLSConfigForServer(t *testing.T) {
	res, err := TLSConfigForServer(&conf.TLS{Enabled: false})
	if err != nil {
		t.Error("no err should be returned. Got: ", err)
	}

	if res != nil {
		t.Error("config should be nil if TLS is not enabled. Got: ", res)
	}

	res, err = TLSConfigForServer(&conf.TLS{Enabled: true})
	if err != ErrTLSEmptyCertOrPK {
		t.Error("should return ErrTLSEmptyCertOrPK. Got: ", err)
	}

	if res != nil {
		t.Error("config should be nil on error. Got", res)
	}

	res, err = TLSConfigForServer(&conf.TLS{
		Enabled:      true,
		CertChainFN:  "nonexistant.crt",
		PrivateKeyFN: "nonexistant.pem",
	})
	if err == nil {
		t.Error("there should be an error with nonexistant files")
	}

	if res != nil {
		t.Error("config should be nil on error. Got", res)
	}

	res, err = TLSConfigForServer(&conf.TLS{
		Enabled:       true,
		CertChainFN:   "../../test/certs/https/proxy.crt",
		PrivateKeyFN:  "../../test/certs/https/proxy.key",
		MinTLSVersion: "1.3",
	})
	if err != nil {
		t.Error("there should be no error. Got: ", err)
	}

	if len(res.Certificates) != 1 {
		t.Error("there should be 1 certificate. Have: ", res.Certificates)
	}

	if res.ClientAuth != 0 {
		t.Error("client auth should be disabled")
	}

	res, err = TLSConfigForServer(&conf.TLS{
		Enabled:          true,
		CertChainFN:      "../../test/certs/https/proxy.crt",
		PrivateKeyFN:     "../../test/certs/https/proxy.key",
		MinTLSVersion:    "1.3",
		ClientValidation: true,
	})
	if err != nil {
		t.Error("there should be no error. Got: ", err)
	}

	if len(res.Certificates) != 1 {
		t.Error("there should be 1 certificate. Have: ", res.Certificates)
	}

	if res.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Error("client auth should be disabled")
	}

	if res.ClientCAs != nil {
		t.Error("no root CA should be used for client-validation purposes")
	}

	res, err = TLSConfigForServer(&conf.TLS{
		Enabled:                  true,
		CertChainFN:              "../../test/certs/https/proxy.crt",
		PrivateKeyFN:             "../../test/certs/https/proxy.key",
		MinTLSVersion:            "1.3",
		ClientValidation:         true,
		ClientValidationRootCert: "../../test/certs/https/ca.crt",
	})
	if err != nil {
		t.Error("there should be no error. Got: ", err)
	}

	if len(res.Certificates) != 1 {
		t.Error("there should be 1 certificate. Have: ", res.Certificates)
	}

	if res.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Error("client auth should be disabled")
	}

	if res.ClientCAs == nil {
		t.Error("a root certificate pool should be set for client validation")
	}
}
