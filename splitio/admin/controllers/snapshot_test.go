package controllers

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v4/splitio/common/snapshot"
	"github.com/splitio/split-synchronizer/v4/splitio/proxy/storage/persistent"
)

func TestDownloadProxySnapshot(t *testing.T) {
	// Read DB snapshot for test
	path := "../../../test/snapshot/proxy.snapshot"
	snap, err := snapshot.DecodeFromFile(path)
	if err != nil {
		t.Error(err)
		return
	}

	tmpDataFile, err := snap.WriteDataToTmpFile()
	if err != nil {
		t.Error(err)
		return
	}

	// loading snapshot from disk
	dbInstance, err := persistent.NewBoltWrapper(tmpDataFile, nil)
	if err != nil {
		t.Error(err)
		return
	}

	ctrl := NewSnapshotController(logging.NewLogger(nil), dbInstance)

	resp := httptest.NewRecorder()
	ctx, router := gin.CreateTestContext(resp)
	ctrl.Register(router)

	ctx.Request, _ = http.NewRequest(http.MethodGet, "/snapshot", nil)
	router.ServeHTTP(resp, ctx.Request)

	responseBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Error(err)
		return
	}

	snapRes, err := snapshot.Decode(responseBody)
	if err != nil {
		t.Error(err)
		return
	}

	if snapRes.Meta().Version != 1 {
		t.Error("Invalid Metadata version")
	}

	if snapRes.Meta().Storage != 1 {
		t.Error("Invalid Metadata storage")
	}

	dat, err := snap.Data()
	if err != nil {
		t.Error(err)
	}
	resData, err := snapRes.Data()
	if err != nil {
		t.Error(err)
	}
	if bytes.Compare(dat, resData) != 0 {
		t.Error("loaded snapshot is different to downloaded")
	}
}
