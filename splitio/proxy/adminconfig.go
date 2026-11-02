package proxy

import (
	"strconv"
	"strings"

	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/util"

	"github.com/splitio/go-toolkit/v5/logging"
)

// maskedSecret is the fixed mask shown in place of secrets. It matches the mask the producer uses for the Redis password.
const maskedSecret = "xxxxxxxxxxxxxxx"

// adminConfigView returns the copy of cfg served by the admin /info/config endpoint, with secrets masked.
// The input is not modified. Empty secrets stay empty so the operator can tell they are unset.
func adminConfigView(cfg *pconf.Main) pconf.Main {
	view := *cfg
	view.Apikey = logging.ObfuscateAPIKey(cfg.Apikey)
	if view.Admin.Password != "" {
		view.Admin.Password = maskedSecret
	}
	if view.Integrations.Slack.Webhook != "" {
		view.Integrations.Slack.Webhook = maskedSecret
	}
	return view
}

// adminSnapshotHash returns the snapshot hash reported by the admin dashboard. It is computed from the real API key,
// the same way the startup snapshot check computes it, so it must never use the obfuscated key.
func adminSnapshotHash(cfg *pconf.Main) string {
	return strconv.Itoa(int(util.HashAPIKey(cfg.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))))
}
