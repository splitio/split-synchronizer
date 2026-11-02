package controllers

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/common/snapshot"
	"github.com/splitio/split-synchronizer/v5/splitio/common/storage"
	"github.com/splitio/split-synchronizer/v5/splitio/proxy/storage/persistent"

	"github.com/splitio/go-toolkit/v5/logging"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestDownloadProxySnapshot(t *testing.T) {
	// Read DB snapshot for test
	path := "../../../test/snapshot/proxy.snapshot"
	snap, err := snapshot.DecodeFromFile(path)
	assert.Nil(t, err)

	tmpDataFile, err := snap.WriteDataToTmpFile()
	assert.Nil(t, err)

	// loading snapshot from disk
	dbInstance, err := persistent.NewBoltWrapper(tmpDataFile, nil)
	assert.Nil(t, err)

	ctrl := NewSnapshotController(logging.NewLogger(nil), dbInstance, "123456", "1.3", nil)

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)
	ctrl.Register(router)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/snapshot", nil)
	router.ServeHTTP(resp, ctx.Request)

	responseBody, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)

	snapRes, err := snapshot.Decode(responseBody)
	assert.Nil(t, err)

	assert.Equal(t, uint64(1), snapRes.Meta().Version)
	assert.Equal(t, uint64(1), snapRes.Meta().Storage)
	assert.Equal(t, "123456", snapRes.Meta().Hash)
	assert.Equal(t, "1.3", snapRes.Meta().FlagSpecVersion)

	dat, err := snap.Data()
	assert.Nil(t, err)
	resData, err := snapRes.Data()
	assert.Nil(t, err)
	assert.Equal(t, 0, bytes.Compare(dat, resData))
	assert.Equal(t, snapshot.PayloadChecksum(resData), snapRes.Meta().Checksum)
}

func TestDownloadProxySnapshotContentIDHeader(t *testing.T) {
	path := "../../../test/snapshot/proxy.snapshot"
	snap, err := snapshot.DecodeFromFile(path)
	assert.Nil(t, err)

	tmpDataFile, err := snap.WriteDataToTmpFile()
	assert.Nil(t, err)

	dbInstance, err := persistent.NewBoltWrapper(tmpDataFile, nil)
	assert.Nil(t, err)

	ctrl := NewSnapshotController(logging.NewLogger(nil), dbInstance, "123456", "1.3", nil)

	doRequest := func() (*httptest.ResponseRecorder, []byte) {
		resp := httptest.NewRecorder()
		ctx, router := gin.CreateTestContext(resp)
		ctrl.Register(router)

		ctx.Request, _ = http.NewRequest(http.MethodGet, "/snapshot", nil)
		router.ServeHTTP(resp, ctx.Request)

		body, err := io.ReadAll(resp.Body)
		assert.Nil(t, err)
		return resp, body
	}

	resp, responseBody := doRequest()
	assert.Equal(t, http.StatusOK, resp.Code)

	contentID := resp.Header().Get("X-Snapshot-Content-Id")
	assert.Regexp(t, `^[0-9a-f]{64}$`, contentID)

	snapRes, err := snapshot.Decode(responseBody)
	assert.Nil(t, err)
	assert.Equal(t, uint64(1), snapRes.Meta().Version)
	assert.Equal(t, uint64(1), snapRes.Meta().Storage)
	assert.Equal(t, "123456", snapRes.Meta().Hash)
	assert.Equal(t, "1.3", snapRes.Meta().FlagSpecVersion)

	dat, err := snap.Data()
	assert.Nil(t, err)
	resData, err := snapRes.Data()
	assert.Nil(t, err)
	assert.Equal(t, 0, bytes.Compare(dat, resData))
	assert.Equal(t, snapshot.PayloadChecksum(resData), snapRes.Meta().Checksum)

	// A second request over the same data must report the same content id
	resp2, _ := doRequest()
	assert.Equal(t, http.StatusOK, resp2.Code)
	assert.Equal(t, contentID, resp2.Header().Get("X-Snapshot-Content-Id"))
}

// The pinned content id changes only if the fixture or the content id encoding (contentid.go) changes.
// Bump the version tag in contentid.go when changing the encoding on purpose.
func TestDownloadProxySnapshotContentIDGolden(t *testing.T) {
	snap, err := snapshot.DecodeFromFile("../../../test/snapshot/proxy.snapshot")
	assert.Nil(t, err)

	tmpDataFile, err := snap.WriteDataToTmpFile()
	assert.Nil(t, err)

	dbInstance, err := persistent.NewBoltWrapper(tmpDataFile, nil)
	assert.Nil(t, err)

	resp := serveSnapshot(t, dbInstance)
	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, "51c31fe0e1bf3378473aaa840cff3b47789e077ecfa2a78f9e3fef12e0cef33c", resp.Header().Get("X-Snapshot-Content-Id"))
}

// plainSnapshotter only implements storage.Snapshotter, without content id support
type plainSnapshotter struct {
	data []byte
}

func (p *plainSnapshotter) GetRawSnapshot() ([]byte, error) {
	return p.data, nil
}

func TestDownloadSnapshotWithoutContentIDSupport(t *testing.T) {
	path := "../../../test/snapshot/proxy.snapshot"
	snap, err := snapshot.DecodeFromFile(path)
	assert.Nil(t, err)

	dat, err := snap.Data()
	assert.Nil(t, err)

	ctrl := NewSnapshotController(logging.NewLogger(nil), &plainSnapshotter{data: dat}, "123456", "1.3", nil)

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)
	ctrl.Register(router)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/snapshot", nil)
	router.ServeHTTP(resp, ctx.Request)

	assert.Equal(t, http.StatusOK, resp.Code)
	_, found := resp.Header()["X-Snapshot-Content-Id"]
	assert.False(t, found)

	responseBody, err := io.ReadAll(resp.Body)
	assert.Nil(t, err)
	snapRes, err := snapshot.Decode(responseBody)
	assert.Nil(t, err)
	assert.Equal(t, "123456", snapRes.Meta().Hash)
	assert.Equal(t, "1.3", snapRes.Meta().FlagSpecVersion)
}

// fakeContentSnapshotter implements storage.ContentIdentifiedSnapshotter with configurable failures
type fakeContentSnapshotter struct {
	data   []byte
	id     string
	idErr  error
	rawErr error
}

var _ storage.ContentIdentifiedSnapshotter = (*fakeContentSnapshotter)(nil)

func (f *fakeContentSnapshotter) GetRawSnapshot() ([]byte, error) {
	if f.rawErr != nil {
		return nil, f.rawErr
	}
	return f.data, nil
}

func (f *fakeContentSnapshotter) GetRawSnapshotWithContentID(flagSpecVersion string, flagSets []string) ([]byte, string, error) {
	if f.idErr != nil {
		return nil, "", f.idErr
	}
	return f.data, f.id, nil
}

func serveSnapshot(t *testing.T, db storage.Snapshotter) *httptest.ResponseRecorder {
	t.Helper()
	ctrl := NewSnapshotController(logging.NewLogger(nil), db, "123456", "1.3", nil)

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)
	ctrl.Register(router)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/snapshot", nil)
	router.ServeHTTP(resp, ctx.Request)
	return resp
}

func TestDownloadSnapshotContentIDFailureFallsBackToRawSnapshot(t *testing.T) {
	snap, err := snapshot.DecodeFromFile("../../../test/snapshot/proxy.snapshot")
	assert.Nil(t, err)
	dat, err := snap.Data()
	assert.Nil(t, err)

	resp := serveSnapshot(t, &fakeContentSnapshotter{data: dat, idErr: errors.New("corrupt item")})
	assert.Equal(t, http.StatusOK, resp.Code)
	_, found := resp.Header()["X-Snapshot-Content-Id"]
	assert.False(t, found)

	snapRes, err := snapshot.Decode(resp.Body.Bytes())
	assert.Nil(t, err)
	assert.Equal(t, "123456", snapRes.Meta().Hash)
	resData, err := snapRes.Data()
	assert.Nil(t, err)
	assert.Equal(t, 0, bytes.Compare(dat, resData))
}

func TestDownloadSnapshotContentIDAndRawFailureReturnsError(t *testing.T) {
	resp := serveSnapshot(t, &fakeContentSnapshotter{
		idErr:  errors.New("corrupt item"),
		rawErr: errors.New("db closed"),
	})
	assert.Equal(t, http.StatusInternalServerError, resp.Code)
	_, found := resp.Header()["X-Snapshot-Content-Id"]
	assert.False(t, found)
}

func TestDownloadSnapshotEmptyContentIDOmitsHeader(t *testing.T) {
	snap, err := snapshot.DecodeFromFile("../../../test/snapshot/proxy.snapshot")
	assert.Nil(t, err)
	dat, err := snap.Data()
	assert.Nil(t, err)

	resp := serveSnapshot(t, &fakeContentSnapshotter{data: dat, id: ""})
	assert.Equal(t, http.StatusOK, resp.Code)
	_, found := resp.Header()["X-Snapshot-Content-Id"]
	assert.False(t, found)

	snapRes, err := snapshot.Decode(resp.Body.Bytes())
	assert.Nil(t, err)
	assert.Equal(t, "123456", snapRes.Meta().Hash)
}
