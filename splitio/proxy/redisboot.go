package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/splitio/go-split-commons/v10/flagsets"
	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/splitio/split-synchronizer/v5/splitio/common"
	pconf "github.com/splitio/split-synchronizer/v5/splitio/proxy/conf"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/redisstate"
	"github.com/splitio/split-synchronizer/v5/splitio/util"
)

// BootSource names the source of the proxy state at boot.
type BootSource string

const (
	// BootSourceRedis restores the proxy state from redis.
	BootSourceRedis BootSource = "redis"
	// BootSourceSnapshotFile restores the proxy state from the snapshot file.
	BootSourceSnapshotFile BootSource = "snapshotFile"
	// BootSourceCold starts with no restored state (no usable redis state and no snapshot file).
	BootSourceCold BootSource = "cold"
)

// BootDecision is the boot outcome. State is non-nil whenever redis was read, including absent, corrupt, foreign and
// empty-valid states, and nil only when no read happened. Only BootSourceRedis is restored.
type BootDecision struct {
	Source BootSource
	State  *redisstate.BootState
	Reason string
}

// bootReadClient is the boot read client: reader commands, key prefix and Close.
type bootReadClient interface {
	redisstate.ReadClient
	Prefix() string
	Close() error
}

// bootClientOpener builds the boot read client. It is called only after the redis state settings are validated.
type bootClientOpener func(cfg *pconf.Main) (bootReadClient, error)

// newBootReadClient is the injection seam for the boot read client; tests replace it with a fake opener.
var newBootReadClient bootClientOpener = openRedisBootReadClient

// openRedisBootReadClient builds the read client from the redis state config. It opens no connection.
func openRedisBootReadClient(cfg *pconf.Main) (bootReadClient, error) {
	opts, err := common.ParseRedisOptions(&cfg.Storage.Redis.Connection)
	if err != nil {
		return nil, common.NewInitError(fmt.Errorf("error parsing redis state config: %w", err), common.ExitInvalidConfiguration)
	}
	client, err := redisstate.NewReadClient(opts)
	if err != nil {
		return nil, common.NewInitError(fmt.Errorf("error building redis state read client: %w", err), common.ExitInvalidConfiguration)
	}
	return client, nil
}

// resolveRedisBootInto reads redis state and picks the boot source, and runs before the snapshot file block. A valid
// state wins over the snapshot file. Absent, corrupt, foreign or entity-less states are never served and fall back to
// the snapshot file or cold start. Unreachable redis is fatal. Disabled redis or force-fresh-startup skips the read.
// With a sink, flags, rule-based segments and each segment are restored while the read runs and BootState.Segments
// stays empty. The sink may hold partial data when the decision is not BootSourceRedis or an error is returned, so the
// caller must discard what it wrote. A nil sink keeps every segment payload in BootState.Segments.
func resolveRedisBootInto(cfg *pconf.Main, logger logging.LoggerInterface, open bootClientOpener, sink redisstate.RestoreSink) (BootDecision, error) {
	if !cfg.Storage.Redis.Enabled {
		return fallbackBootDecision(cfg, nil, "redis state is disabled"), nil
	}
	if cfg.Initialization.ForceFreshStartup {
		logger.Info("force-fresh-startup is set, redis state is not read")
		return fallbackBootDecision(cfg, nil, "force-fresh-startup"), nil
	}
	if err := validateRedisStateSettings(&cfg.Storage.Redis); err != nil {
		return BootDecision{}, err
	}

	client, err := open(cfg)
	if err != nil {
		return BootDecision{}, err
	}
	// Release the read-only client when the decision is made, not at shutdown. A close error does not change the decision.
	defer func() { _ = client.Close() }()

	keys, err := redisstate.NewKeys(client.Prefix(), cfg.Apikey, cfg.Storage.Redis.Namespace)
	if err != nil {
		return BootDecision{}, common.NewInitError(fmt.Errorf("error building redis state keys: %w", err), common.ExitInvalidConfiguration)
	}
	fingerprint := util.ConfigFingerprint(cfg.Apikey, cfg.FlagSpecVersion, cfg.FlagSetsFilter)
	reader, err := redisstate.NewReader(client, keys, fingerprint)
	if err != nil {
		return BootDecision{}, common.NewInitError(fmt.Errorf("error creating redis state reader: %w", err), common.ExitInvalidConfiguration)
	}

	// No overall deadline; the reader bounds each call and its read budget.
	logger.Info("reading the proxy state from redis")
	var state redisstate.BootState
	if sink != nil {
		state, err = reader.ReadInto(context.Background(), sink)
	} else {
		state, err = reader.Read(context.Background(), func(redisstate.SegmentState) error { return nil })
	}
	if err != nil {
		return BootDecision{}, common.NewInitError(fmt.Errorf("error reading redis state at boot: %w", err), common.ExitRedisInitializationFailed)
	}
	return decideFromState(cfg, logger, state)
}

// decideFromState maps a redis boot state already read by resolveRedisBootInto to a boot decision. It logs why a state
// is not served and falls back through fallbackBootDecision. It does not touch redis.
func decideFromState(cfg *pconf.Main, logger logging.LoggerInterface, state redisstate.BootState) (BootDecision, error) {
	namespace := cfg.Storage.Redis.Namespace
	if namespace == "" {
		namespace = "(default, derived from the api key)"
	}

	switch state.Status {
	case redisstate.BootValid:
		if len(state.Flags) == 0 && len(state.RuleBasedSegments) == 0 && state.SegmentCount == 0 {
			logger.Warning(fmt.Sprintf("redis state holds no flags, rule-based segments or segments (%s), treated as absent, nothing restored from redis", state.Reason))
			return fallbackBootDecision(cfg, &state, "redis state holds no entities"), nil
		}
		if len(state.SkippedSegments) > 0 {
			logger.Warning(fmt.Sprintf("%d segments were not restored from redis and will be refetched from the cloud: %s",
				len(state.SkippedSegments), skippedSummary(state)))
		}
		logger.Info(fmt.Sprintf("starting from redis state (%d segments, %d flags, %d rule-based segments), valid redis state takes precedence over the snapshot file",
			state.SegmentCount, len(state.Flags), len(state.RuleBasedSegments)))
		return BootDecision{Source: BootSourceRedis, State: &state, Reason: "valid redis state"}, nil
	case redisstate.BootForeign:
		logger.Warning(fmt.Sprintf("redis state in namespace %s belongs to another environment or configuration (%s), nothing restored from redis; use force-fresh-startup or delete the keys of that namespace to replace it",
			namespace, state.Reason))
		return fallbackBootDecision(cfg, &state, "foreign redis state"), nil
	case redisstate.BootCorrupt:
		if state.BudgetExhausted {
			logger.Error(fmt.Sprintf("restore time limit reached (%s): reading all state from redis (feature flags, rule-based segments and segments) did not finish within the time allowed for the whole restore, so nothing was restored from redis. Starting from the snapshot file or empty, data may be stale until the first sync completes", state.Reason))
			return fallbackBootDecision(cfg, &state, "redis restore time limit reached"), nil
		}
		logger.Warning(fmt.Sprintf("redis state is corrupt (%s), nothing restored from redis", state.Reason))
		return fallbackBootDecision(cfg, &state, "corrupt redis state"), nil
	case redisstate.BootAbsent:
		logger.Warning(fmt.Sprintf("no redis state found (%s), nothing restored from redis", state.Reason))
		return fallbackBootDecision(cfg, &state, "absent redis state"), nil
	default:
		logger.Warning(fmt.Sprintf("unexpected redis state status %s, nothing restored from redis", state.Status))
		return fallbackBootDecision(cfg, &state, "unexpected redis state status"), nil
	}
}

// fallbackBootDecision is used when redis is not usable: the snapshot file if configured, else cold start.
// The snapshot block still validates the file.
func fallbackBootDecision(cfg *pconf.Main, state *redisstate.BootState, reason string) BootDecision {
	if cfg.Initialization.Snapshot != "" {
		return BootDecision{Source: BootSourceSnapshotFile, State: state, Reason: reason}
	}
	return BootDecision{Source: BootSourceCold, State: state, Reason: reason}
}

// applyCommittedCursors is the injection seam for the cursor step, so its failure can be injected.
var applyCommittedCursors = storage.ApplyCommittedCursors

// applyRedisCursors sets the committed cursors of a redis boot decision after the sink restored the entities. Any other
// source is a no-op. It must run before setupRedisState so the Redis writer starts after the restore.
func applyRedisCursors(boot BootDecision, splits *storage.ProxySplitStorageImpl, rbs *storage.ProxyRuleBasedSegmentsStorageImpl, segs *storage.ProxySegmentStorageImpl) error {
	if boot.Source != BootSourceRedis || boot.State == nil {
		return nil
	}
	if err := applyCommittedCursors(*boot.State, splits, rbs, segs); err != nil {
		return common.NewInitError(fmt.Errorf("error applying redis state cursors: %w", err), common.ExitRedisInitializationFailed)
	}
	return nil
}

// maxLoggedSkipped caps how many skipped segments are named in one log line.
const maxLoggedSkipped = 20

// skippedSummary lists the skipped segments with their reasons, capped at maxLoggedSkipped.
func skippedSummary(state redisstate.BootState) string {
	parts := make([]string, 0, maxLoggedSkipped+1)
	for i, name := range state.SkippedSegments {
		if i == maxLoggedSkipped {
			parts = append(parts, fmt.Sprintf("and %d more", len(state.SkippedSegments)-maxLoggedSkipped))
			break
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", name, state.SkipReasons[name]))
	}
	return strings.Join(parts, "; ")
}

// Boot phases reported under "redis.bootPhase" on the boot admin health routes.
const (
	bootPhaseReading     = "reading"
	bootPhaseInitialSync = "initial-sync"
)

// bootAdminShutdownTimeout bounds how long stop waits for in-flight health requests before forcing the close.
const bootAdminShutdownTimeout = 5 * time.Second

// bootAdminUnavailable is the 503 body for every non-health route while the proxy starts.
const bootAdminUnavailable = "proxy is starting"

// bootAdmin is a minimal admin listener that holds the admin address while the proxy restores its state and runs the
// initial sync, so health probes get an answer instead of a refused connection. It serves 200 on the two health
// routes (liveness: the process is alive) and 503 on everything else. The body keeps the shape of the real health
// endpoint (healthy, healthySince, items) with healthy=false, plus a "redis" object for the boot. The routes are
// unauthenticated, like the real ones. startBootAdmin owns the serve goroutine; stop shuts the server down and joins
// it. The full admin server replaces it once the storages it needs exist.
type bootAdmin struct {
	server *http.Server  // set before the serve goroutine starts, read-only afterwards
	once   sync.Once     // makes stop idempotent
	done   chan struct{} // closed when the serve goroutine exits
	logger logging.LoggerInterface
	phase  atomic.Value // current boot phase string, written by setPhase and read by the health handler
}

// setPhase records the boot phase served under "redis.bootPhase". It is safe on a nil receiver.
func (b *bootAdmin) setPhase(phase string) {
	if b != nil {
		b.phase.Store(phase)
	}
}

// startBootAdmin binds the admin address synchronously, so an unusable address fails the boot before the redis read.
func startBootAdmin(cfg *pconf.Main, logger logging.LoggerInterface) (*bootAdmin, error) {
	tlsConfig, err := util.TLSConfigForServer(&cfg.Admin.TLS)
	if err != nil {
		return nil, common.NewInitError(fmt.Errorf("error setting up admin TLS config: %w", err), common.ExitTLSError)
	}
	addr := fmt.Sprintf("%s:%d", cfg.Admin.Host, cfg.Admin.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, common.NewInitError(fmt.Errorf("error starting admin server: %w", err), common.ExitAdminError)
	}

	b := &bootAdmin{logger: logger, done: make(chan struct{})}
	b.phase.Store(bootPhaseReading)
	health := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"healthy":      false,
			"healthySince": nil,
			"items":        []interface{}{},
			"redis":        map[string]interface{}{"enabled": true, "bootPhase": b.phase.Load()},
		})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/application", health)
	mux.HandleFunc("GET /health/dependencies", health)
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, bootAdminUnavailable, http.StatusServiceUnavailable)
	})

	b.server = &http.Server{Handler: mux, TLSConfig: tlsConfig, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		defer close(b.done)
		var serveErr error
		if tlsConfig != nil {
			serveErr = b.server.ServeTLS(listener, "", "")
		} else {
			serveErr = b.server.Serve(listener)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Error(fmt.Sprintf("boot admin server stopped: %v", serveErr))
			// Serve closes the listener on return; this covers an error before it took ownership. Nothing to do on failure.
			_ = listener.Close()
		}
	}()
	logger.Info(fmt.Sprintf("admin health endpoints are available on %s while the proxy starts", addr))
	return b, nil
}

// stop releases the admin address. It is safe to call more than once and on a nil receiver.
func (b *bootAdmin) stop() {
	if b == nil {
		return
	}
	b.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), bootAdminShutdownTimeout)
		defer cancel()
		if err := b.server.Shutdown(ctx); err != nil {
			// Shutdown already failed and is reported below; Close only force-drops the remaining connections
			_ = b.server.Close()
			b.logger.Warning(fmt.Sprintf("boot admin server did not shut down cleanly: %v", err))
		}
		<-b.done
	})
}

// newProxyStorages builds the three proxy storages over one db.
func newProxyStorages(db persistent.DBWrapper, logger logging.LoggerInterface, cfg *pconf.Main, preloadFromDisk bool) (
	*storage.ProxySplitStorageImpl, *storage.ProxyRuleBasedSegmentsStorageImpl, *storage.ProxySegmentStorageImpl,
) {
	return storage.NewProxySplitStorage(db, logger, flagsets.NewFlagSetFilter(cfg.FlagSetsFilter), preloadFromDisk),
		storage.NewProxyRuleBasedSegmentsStorage(db, logger, preloadFromDisk),
		storage.NewProxySegmentStorage(db, logger, preloadFromDisk)
}
