package util

import (
	"fmt"
	"strings"
	"time"

	"github.com/splitio/go-split-commons/v3/dtos"
	"github.com/splitio/go-toolkit/v4/hasher"
	"github.com/splitio/go-toolkit/v4/nethelpers"
	"github.com/splitio/split-synchronizer/v4/appcontext"
	"github.com/splitio/split-synchronizer/v4/conf"
)

// ParseTime parses a date to format d h m s
func ParseTime(date time.Time) string {
	upt := time.Since(date)
	d := int64(0)
	h := int64(0)
	m := int64(0)
	s := int64(upt.Seconds())

	if s > 60 {
		m = int64(s / 60)
		s = s - m*60
	}

	if m > 60 {
		h = int64(m / 60)
		m = m - h*60
	}

	if h > 24 {
		d = int64(h / 24)
		h = h - d*24
	}

	return fmt.Sprintf("%dd %dh %dm %ds", d, h, m, s)
}

// HashAPIKey hashes apikey
func HashAPIKey(apikey string) uint32 {
	murmur32 := hasher.NewMurmur332Hasher(0)
	return murmur32.Hash([]byte(apikey))
}

// GetMetadata wrapps metadata
func GetMetadata() dtos.Metadata {
	instanceName := "unknown"
	ipAddress := "unknown"
	if conf.Data.IPAddressesEnabled {
		ip, err := nethelpers.ExternalIP()
		if err == nil {
			ipAddress = ip
			instanceName = fmt.Sprintf("ip-%s", strings.Replace(ipAddress, ".", "-", -1))
		}
	}

	return dtos.Metadata{
		MachineIP:   ipAddress,
		MachineName: instanceName,
		SDKVersion:  appcontext.VersionHeader(),
	}
}
