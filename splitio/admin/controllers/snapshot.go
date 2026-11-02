package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v5/splitio/common/snapshot"
	"github.com/splitio/split-synchronizer/v5/splitio/common/storage"
)

// SnapshotController bundles endpoints associated to snapshot management
type SnapshotController struct {
	logger          logging.LoggerInterface
	db              storage.Snapshotter
	hash            string
	flagSpecVersion string
	flagSets        []string
}

// NewSnapshotController constructs a new snapshot controller.
// flagSpecVersion is written into snapshot metadata as the spec the exported data was fetched with.
// flagSets is the flag sets filter used to compute the content id when db supports it.
func NewSnapshotController(logger logging.LoggerInterface, db storage.Snapshotter, hash string, flagSpecVersion string, flagSets []string) *SnapshotController {
	return &SnapshotController{logger: logger, db: db, hash: hash, flagSpecVersion: flagSpecVersion, flagSets: flagSets}
}

// Register mounts the endpoints int he provided router
func (c *SnapshotController) Register(router gin.IRouter) {
	router.GET("/snapshot", c.downloadSnapshot)
}

func (c *SnapshotController) downloadSnapshot(ctx *gin.Context) {
	// curl http://localhost:3010/admin/proxy/snapshot --output split.proxy.0001.snapshot.gz
	snapshotName := fmt.Sprintf("split.proxy.%d.snapshot", time.Now().UnixNano())
	var b []byte
	var contentID string
	var err error
	if ci, ok := c.db.(storage.ContentIdentifiedSnapshotter); ok {
		b, contentID, err = ci.GetRawSnapshotWithContentID(c.flagSpecVersion, c.flagSets)
		if err != nil {
			c.logger.Warning("content id unavailable, serving snapshot without X-Snapshot-Content-Id header: ", err)
			contentID = ""
			b, err = c.db.GetRawSnapshot()
		}
	} else {
		b, err = c.db.GetRawSnapshot()
	}
	if err != nil {
		c.logger.Error("error getting contents from db to build snapshot: ", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "error reading data"})
		return
	}

	s, err := snapshot.New(snapshot.Metadata{
		Version:         1,
		Storage:         snapshot.StorageBoltDB,
		Hash:            c.hash,
		Checksum:        snapshot.PayloadChecksum(b),
		FlagSpecVersion: c.flagSpecVersion,
	}, b)
	if err != nil {
		c.logger.Error("error building snapshot: ", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "error building snapshot"})
		return
	}

	encodedSnap, err := s.Encode()
	if err != nil {
		c.logger.Error("error encoding snapshot: ", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "error encoding snapshot"})
		return
	}

	ctx.Writer.Header().Set("Content-Type", "application/octet-stream")
	ctx.Writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, snapshotName))
	ctx.Writer.Header().Set("Content-Length", strconv.Itoa(len(encodedSnap)))
	if contentID != "" {
		ctx.Writer.Header().Set("X-Snapshot-Content-Id", contentID)
	}
	ctx.Writer.Write(encodedSnap)
}
