package util

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/splitio/split-synchronizer/v5/splitio"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-toolkit/v5/hasher"
	"github.com/splitio/go-toolkit/v5/nethelpers"
)

// HashAPIKey hashes apikey
func HashAPIKey(apikey string) uint32 {
	murmur32 := hasher.NewMurmur332Hasher(0)
	return murmur32.Hash([]byte(apikey))
}

// ConfigFingerprint returns the decimal fingerprint of the API key, flag spec version and flag sets.
// The fields are joined with a NUL byte so field boundaries cannot collide. It differs from the legacy apikey hash
// used by the Bolt based checks (HashAPIKey of the plain concatenation).
//
// The value is a 32-bit murmur hash, so two different configurations can collide (the same API key with different
// flag sets is the case to watch). A collision is not detected: the Redis writer only compares fingerprints and
// refuses to write when they differ (ErrEnvironmentMismatch), so a collision lets one configuration reuse the
// other's state.
func ConfigFingerprint(apikey, flagSpecVersion string, flagSets []string) string {
	return strconv.Itoa(int(HashAPIKey(apikey + "\x00" + flagSpecVersion + "\x00" + strings.Join(flagSets, "::"))))
}

// GetClientKey accepts an apikey and extracts the "client-key" portion of it
func GetClientKey(apikey string) (string, error) {
	if len(apikey) < 4 {
		return "", errors.New("apikey too short")
	}
	return apikey[len(apikey)-4:], nil
}

// GetMetadata wrapps metadata
func GetMetadata(proxy bool, ipAddressEnabled bool) dtos.Metadata {
	instanceName := "unknown"
	ipAddress := "unknown"
	if ipAddressEnabled {
		ip, err := nethelpers.ExternalIP()
		if err == nil {
			ipAddress = ip
			instanceName = fmt.Sprintf("ip-%s", strings.Replace(ipAddress, ".", "-", -1))
		}
	}

	appName := "SplitSyncProducerMode-"
	if proxy {
		appName = "SplitSyncProxyMode-"
	}

	return dtos.Metadata{
		MachineIP:   ipAddress,
		MachineName: instanceName,
		SDKVersion:  appName + splitio.Version,
	}
}
