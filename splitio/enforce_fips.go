//go:build enforce_fips
// +build enforce_fips

package splitio

import (
	_ "crypto/tls/fipsonly"
)
