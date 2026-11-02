package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	adminControllers "github.com/splitio/split-synchronizer/v5/splitio/admin/controllers"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/util"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/stretchr/testify/assert"
)

const (
	fixtureAdminPassword = "pw-fixture-123" // gitleaks:allow fake fixture
	fixtureSlackWebhook  = "https://hooks.example.test/T000/B000/XXXX"
	fixtureAPIKey        = "fixture-sdk-apikey-0123456789abcdefghijklmnopqrstuvwxyz"
	fixtureClientKey     = "fixture-client-key-0001"
)

func newAdminConfigFixture() *pconf.Main {
	cfg := &pconf.Main{}
	cfg.Apikey = fixtureAPIKey
	cfg.FlagSpecVersion = "1.3"
	cfg.FlagSetsFilter = []string{"set_a"}
	cfg.Server.Host = "0.0.0.0"
	cfg.Server.Port = 3000
	cfg.Server.ClientApikeys = []string{fixtureClientKey}
	cfg.Admin.Host = "127.0.0.1"
	cfg.Admin.Port = 3010
	cfg.Admin.Username = "admin-user"
	cfg.Admin.Password = fixtureAdminPassword
	cfg.Integrations.Slack.Webhook = fixtureSlackWebhook
	return cfg
}

func TestAdminConfigViewMasksPassword(t *testing.T) {
	cfg := newAdminConfigFixture()

	view := adminConfigView(cfg)

	assert.Equal(t, maskedSecret, view.Admin.Password)
	raw, err := json.Marshal(view)
	assert.Nil(t, err)
	assert.NotContains(t, string(raw), fixtureAdminPassword)
}

func TestAdminConfigViewMasksSlackWebhook(t *testing.T) {
	cfg := newAdminConfigFixture()

	view := adminConfigView(cfg)

	assert.Equal(t, maskedSecret, view.Integrations.Slack.Webhook)
	raw, err := json.Marshal(view)
	assert.Nil(t, err)
	assert.NotContains(t, string(raw), fixtureSlackWebhook)
}

func TestAdminConfigViewKeepsEmptySecretsEmpty(t *testing.T) {
	cfg := newAdminConfigFixture()
	cfg.Admin.Password = ""
	cfg.Integrations.Slack.Webhook = ""

	view := adminConfigView(cfg)

	assert.Equal(t, "", view.Admin.Password)
	assert.Equal(t, "", view.Integrations.Slack.Webhook)
}

func TestAdminConfigViewObfuscatesApikey(t *testing.T) {
	cfg := newAdminConfigFixture()

	view := adminConfigView(cfg)

	assert.Equal(t, logging.ObfuscateAPIKey(fixtureAPIKey), view.Apikey)
	assert.NotEqual(t, fixtureAPIKey, view.Apikey)
}

func TestAdminConfigViewKeepsEmptyApikeyEmpty(t *testing.T) {
	cfg := newAdminConfigFixture()
	cfg.Apikey = "" // offline mode runs without an SDK key

	view := adminConfigView(cfg)

	assert.Equal(t, "", view.Apikey)
}

func TestAdminConfigViewDoesNotMutateInput(t *testing.T) {
	cfg := newAdminConfigFixture()

	_ = adminConfigView(cfg)

	assert.Equal(t, fixtureAPIKey, cfg.Apikey)
	assert.Equal(t, fixtureAdminPassword, cfg.Admin.Password)
	assert.Equal(t, fixtureSlackWebhook, cfg.Integrations.Slack.Webhook)
}

func TestAdminConfigViewKeepsNonSecretFields(t *testing.T) {
	cfg := newAdminConfigFixture()

	view := adminConfigView(cfg)

	assert.Equal(t, cfg.Admin.Username, view.Admin.Username)
	assert.Equal(t, cfg.Admin.Host, view.Admin.Host)
	assert.Equal(t, cfg.Admin.Port, view.Admin.Port)
	assert.Equal(t, cfg.Server.Host, view.Server.Host)
	assert.Equal(t, cfg.Server.Port, view.Server.Port)
	assert.Equal(t, cfg.FlagSpecVersion, view.FlagSpecVersion)
	assert.Equal(t, cfg.FlagSetsFilter, view.FlagSetsFilter)
	assert.Equal(t, cfg.Server.ClientApikeys, view.Server.ClientApikeys)
}

func TestAdminConfigViewJSONHasNoSecrets(t *testing.T) {
	cfg := newAdminConfigFixture()

	raw, err := json.Marshal(adminConfigView(cfg))
	assert.Nil(t, err)
	body := string(raw)

	assert.NotContains(t, body, fixtureAdminPassword)
	assert.NotContains(t, body, fixtureSlackWebhook)
	assert.NotContains(t, body, fixtureAPIKey)
	assert.Contains(t, body, fixtureClientKey)
}

func TestInfoConfigEndpointDoesNotExposeSecrets(t *testing.T) {
	cfg := newAdminConfigFixture()

	router := gin.New()
	adminControllers.NewInfoController(true, nil, adminConfigView(cfg)).Register(router)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, fixtureAdminPassword)
	assert.NotContains(t, body, fixtureSlackWebhook)
	assert.NotContains(t, body, fixtureAPIKey)
	assert.Contains(t, body, maskedSecret)
}

func TestSnapshotHashUsesUnobfuscatedApikey(t *testing.T) {
	cfg := newAdminConfigFixture()
	before := *cfg

	got := adminSnapshotHash(cfg)

	want := strconv.Itoa(int(util.HashAPIKey(fixtureAPIKey + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))))
	assert.Equal(t, want, got)

	obfuscatedHash := strconv.Itoa(int(util.HashAPIKey(logging.ObfuscateAPIKey(fixtureAPIKey) + cfg.FlagSpecVersion + strings.Join(cfg.FlagSetsFilter, "::"))))
	assert.NotEqual(t, obfuscatedHash, got)

	assert.Equal(t, before.Apikey, cfg.Apikey, "hashing must not mutate the config")
}
