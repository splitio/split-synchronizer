package util

import (
	"errors"
	"fmt"
	"strings"

	"github.com/splitio/split-synchronizer/v5/splitio"

	"github.com/splitio/go-split-commons/v9/dtos"
	"github.com/splitio/go-toolkit/v5/hasher"
	"github.com/splitio/go-toolkit/v5/nethelpers"
)

// HashAPIKey hashes apikey
func HashAPIKey(apikey string) uint32 {
	murmur32 := hasher.NewMurmur332Hasher(0)
	return murmur32.Hash([]byte(apikey))
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
