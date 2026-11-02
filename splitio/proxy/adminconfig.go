package proxy

import (
	"strconv"
	"strings"

	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/util"

	"github.com/splitio/go-toolkit/v5/logging"
)

const maskedSecret = "xxxxxxxxxxxxxxx"

// adminConfigView returns a masked copy of cfg for /info/config. Empty secrets stay empty.
func adminConfigView(cfg *pconf.Main) pconf.Main {
	view := *cfg
	if view.Apikey != "" {
		view.Apikey = logging.ObfuscateAPIKey(cfg.Apikey)
	}
	if len(cfg.Server.ClientApikeys) > 0 {
		// Fresh slice: view is a shallow copy and must not alias (or mutate) cfg's keys.
		view.Server.ClientApikeys = make([]string, len(cfg.Server.ClientApikeys))
		for i, k := range cfg.Server.ClientApikeys {
			view.Server.ClientApikeys[i] = logging.ObfuscateAPIKey(k)
		}
	}
	if view.Admin.Password != "" {
		view.Admin.Password = maskedSecret
	}
	if view.Integrations.Slack.Webhook != "" {
		view.Integrations.Slack.Webhook = maskedSecret
	}
	return view
}

// adminSnapshotHash must hash the real API key, as the startup snapshot check does.
func adminSnapshotHash(cfg *pconf.Main) string {
	return strconv.Itoa(int(util.HashAPIKey(cfg.Apikey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))))
}
