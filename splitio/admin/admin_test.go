package admin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adminCommon "github.com/splitio/split-synchronizer/v5/splitio/admin/common"
	"github.com/splitio/split-synchronizer/v5/splitio/provisional/observability"

	"github.com/splitio/go-split-commons/v10/dtos"
	"github.com/splitio/go-split-commons/v10/storage/mocks"
	"github.com/splitio/go-toolkit/v5/datastructures/set"
	"github.com/splitio/go-toolkit/v5/logging"
)

type warningRecorder struct {
	warnings []string
}

func (r *warningRecorder) Error(msg ...interface{})   {}
func (r *warningRecorder) Info(msg ...interface{})    {}
func (r *warningRecorder) Debug(msg ...interface{})   {}
func (r *warningRecorder) Verbose(msg ...interface{}) {}
func (r *warningRecorder) Warning(msg ...interface{}) {
	r.warnings = append(r.warnings, fmt.Sprint(msg...))
}

func TestWarnIfAdminUnauthenticated(t *testing.T) {
	cases := []struct {
		name     string
		username string
		password string
		wantWarn bool
	}{
		{name: "both empty", username: "", password: "", wantWarn: true},
		{name: "only username", username: "u", password: "", wantWarn: true},
		{name: "only password", username: "", password: "p", wantWarn: true},
		{name: "both set", username: "u", password: "p", wantWarn: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &warningRecorder{}
			warnIfAdminUnauthenticated(rec, "0.0.0.0", 3010, tc.username, tc.password)

			if !tc.wantWarn {
				if len(rec.warnings) != 0 {
					t.Fatalf("expected no warning, got %d: %v", len(rec.warnings), rec.warnings)
				}
				return
			}

			if len(rec.warnings) != 1 {
				t.Fatalf("expected exactly 1 warning, got %d: %v", len(rec.warnings), rec.warnings)
			}
			msg := rec.warnings[0]
			for _, want := range []string{"/admin", "/info", "/shutdown", "admin-username", "admin-password", "0.0.0.0:3010"} {
				if !strings.Contains(msg, want) {
					t.Errorf("warning %q does not contain %q", msg, want)
				}
			}
		})
	}
}

func TestWarnIfAdminUnauthenticatedDoesNotLeakValues(t *testing.T) {
	rec := &warningRecorder{}
	warnIfAdminUnauthenticated(rec, "0.0.0.0", 3010, "", "s3cret-test-value")

	if len(rec.warnings) != 1 {
		t.Fatalf("expected exactly 1 warning, got %d", len(rec.warnings))
	}
	if strings.Contains(rec.warnings[0], "s3cret-test-value") {
		t.Errorf("warning leaks the password value: %q", rec.warnings[0])
	}
}

func TestWarnIfAdminUnauthenticatedNilLogger(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic with nil logger: %v", r)
		}
	}()
	warnIfAdminUnauthenticated(nil, "0.0.0.0", 3010, "", "")
}

// extSplitMock adds the error-reporting method the observability wrapper requires.
type extSplitMock struct {
	*mocks.MockSplitStorage
}

func (e *extSplitMock) UpdateWithErrors([]dtos.SplitDTO, []dtos.SplitDTO, int64) error { return nil }

// extSegmentMock adds the summary-reporting method the observability wrapper requires.
type extSegmentMock struct {
	*mocks.MockSegmentStorage
}

func (e *extSegmentMock) Size(string) (int, error) { return 0, nil }

func (e *extSegmentMock) UpdateWithSummary(string, *set.ThreadUnsafeSet, *set.ThreadUnsafeSet, int64) (int, int, error) {
	return 0, 0, nil
}

// newObservableTestStorages wraps minimal mock storages so the observability controller
// accepts them. NewServer rejects nil storages at construction time.
func newObservableTestStorages(logger *warningRecorder) (observability.ObservableSplitStorage, observability.ObservableSegmentStorage) {
	splitMock := &extSplitMock{MockSplitStorage: &mocks.MockSplitStorage{
		SplitNamesCall:         func() []string { return nil },
		SegmentNamesCall:       func() *set.ThreadUnsafeSet { return set.NewSet() },
		GetAllFlagSetNamesCall: func() []string { return nil },
	}}
	segmentMock := &extSegmentMock{MockSegmentStorage: &mocks.MockSegmentStorage{}}

	oSplit, err := observability.NewObservableSplitStorage(splitMock, logging.NewLogger(nil))
	if err != nil {
		panic(err)
	}
	oSegment, err := observability.NewObservableSegmentStorage(logging.NewLogger(nil), splitMock, segmentMock)
	if err != nil {
		panic(err)
	}
	return oSplit, oSegment
}

// newTestServerOptions returns Options with only the fields NewServer needs to build
// a router. Storages are left empty: the controllers only retain them at construction.
func newTestServerOptions(logger *warningRecorder, username, password string) *Options {
	splitStorage, segmentStorage := newObservableTestStorages(logger)
	return &Options{
		Storages: adminCommon.Storages{
			SplitStorage:   splitStorage,
			SegmentStorage: segmentStorage,
		},
		Host:     "127.0.0.1",
		Port:     3010,
		Name:     "test",
		Proxy:    false,
		Username: username,
		Password: password,
		Logger:   logger,
	}
}

func TestNewServerWarnsWhenCredentialsUnset(t *testing.T) {
	rec := &warningRecorder{}
	if _, err := NewServer(newTestServerOptions(rec, "", "")); err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if len(rec.warnings) != 1 {
		t.Fatalf("expected exactly 1 warning from NewServer, got %d: %v", len(rec.warnings), rec.warnings)
	}
}

func TestNewServerNoWarningWhenCredentialsSet(t *testing.T) {
	rec := &warningRecorder{}
	if _, err := NewServer(newTestServerOptions(rec, "u", "p")); err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if len(rec.warnings) != 0 {
		t.Fatalf("expected no warning when credentials are set, got %v", rec.warnings)
	}
}

func TestNewServerRouteAuthBehaviour(t *testing.T) {
	cases := []struct {
		name       string
		username   string
		password   string
		auth       bool
		wantStatus int
	}{
		{name: "open without credentials", username: "", password: "", auth: false, wantStatus: http.StatusOK},
		{name: "401 without auth header", username: "u", password: "p", auth: false, wantStatus: http.StatusUnauthorized},
		{name: "200 with valid auth header", username: "u", password: "p", auth: true, wantStatus: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, err := NewServer(newTestServerOptions(&warningRecorder{}, tc.username, tc.password))
			if err != nil {
				t.Fatalf("NewServer: %v", err)
			}

			req := httptest.NewRequest(http.MethodGet, "/info/ping", nil)
			if tc.auth {
				req.SetBasicAuth(tc.username, tc.password)
			}
			resp := httptest.NewRecorder()
			srv.server.Handler.ServeHTTP(resp, req)

			if resp.Code != tc.wantStatus {
				t.Errorf("GET /info/ping status = %d, want %d", resp.Code, tc.wantStatus)
			}
		})
	}
}
