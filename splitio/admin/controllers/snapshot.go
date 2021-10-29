package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/splitio/go-toolkit/v5/logging"
	"github.com/splitio/split-synchronizer/v4/splitio/common/snapshot"
	"github.com/splitio/split-synchronizer/v4/splitio/common/storage"
)

// SnapshotController bundles endpoints associated to snapshot management
type SnapshotController struct {
	logger logging.LoggerInterface
	db     storage.Snapshotter
}

// NewSnapshotController constructs a new snapshot controller
func NewSnapshotController(logger logging.LoggerInterface, db storage.Snapshotter) *SnapshotController {
	return &SnapshotController{logger: logger, db: db}
}

// Register mounts the endpoints int he provided router
func (c *SnapshotController) Register(router gin.IRouter) {
	router.GET("/snapshot", c.downloadSnapshot)
}

func (c *SnapshotController) downloadSnapshot(ctx *gin.Context) {
	// curl http://localhost:3010/admin/proxy/snapshot --output split.proxy.0001.snapshot.gz
	snapshotName := fmt.Sprintf("split.proxy.%d.snapshot", time.Now().UnixNano())
	b, err := c.db.GetRawSnapshot()
	if err != nil {
		c.logger.Error("error getting contents from db to build snapshot: ", err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "error reading data"})
		return
	}

	s, err := snapshot.New(snapshot.Metadata{Version: 1, Storage: snapshot.StorageBoltDB}, b)
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
	ctx.Writer.Write(encodedSnap)
}
