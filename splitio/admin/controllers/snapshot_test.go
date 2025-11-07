package controllers

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/splitio/split-synchronizer/v5/splitio/common/snapshot"
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

	ctrl := NewSnapshotController(logging.NewLogger(nil), dbInstance, "123456")

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

	dat, err := snap.Data()
	assert.Nil(t, err)
	resData, err := snapRes.Data()
	assert.Nil(t, err)
	assert.Equal(t, 0, bytes.Compare(dat, resData))
}
