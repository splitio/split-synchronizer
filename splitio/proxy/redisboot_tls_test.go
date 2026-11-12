package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/splitio/split-synchronizer/v5/splitio/common/conf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeBootTestTLSCert writes a self-signed ECDSA P-256 certificate for 127.0.0.1 and its key into dir.
func writeBootTestTLSCert(t *testing.T, dir string) (certFile, keyFile string, certPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certFile, certPEM, 0o600))
	require.NoError(t, os.WriteFile(keyFile, keyPEM, 0o600))
	return certFile, keyFile, certPEM
}

// bootAdminGetTLS retries a GET until the TLS listener answers or 2s pass; returns status and decoded JSON body.
func bootAdminGetTLS(t *testing.T, client *http.Client, url string) (int, map[string]interface{}) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := client.Get(url)
		if err != nil {
			if time.Now().After(deadline) {
				t.Fatalf("GET %s: %v", url, err)
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}
		defer func() { _ = resp.Body.Close() }()
		var body map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return resp.StatusCode, body
	}
}

func TestBootAdminServesHealthOverTLS(t *testing.T) {
	certFile, keyFile, certPEM := writeBootTestTLSCert(t, t.TempDir())
	cfg := bootTestConfig("")
	cfg.Admin.Host = "127.0.0.1"
	cfg.Admin.Port = freeTCPPort(t)
	cfg.Admin.TLS = conf.TLS{Enabled: true, MinTLSVersion: "1.3", CertChainFN: certFile, PrivateKeyFN: keyFile}

	b, err := startBootAdmin(cfg, &bootTestLogger{})
	require.NoError(t, err)
	t.Cleanup(b.stop)

	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(certPEM))
	client := &http.Client{
		Timeout:   500 * time.Millisecond,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13}},
	}
	base := fmt.Sprintf("127.0.0.1:%d", cfg.Admin.Port)

	status, body := bootAdminGetTLS(t, client, "https://"+base+"/health/application")
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, false, body["healthy"])
	redis, ok := body["redis"].(map[string]interface{})
	require.True(t, ok, "redis section missing in %v", body)
	assert.Equal(t, bootPhaseReading, redis["bootPhase"])

	status, _ = bootAdminGetTLS(t, client, "https://"+base+"/other")
	assert.Equal(t, http.StatusServiceUnavailable, status)

	resp, err := client.Get("http://" + base + "/health/application")
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		assert.NotEqual(t, http.StatusOK, resp.StatusCode, "plain HTTP must not be served by a TLS admin listener")
	}
}
