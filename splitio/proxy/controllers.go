package proxy

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/splitio/go-agent/splitio/storage/boltdb"
	"github.com/splitio/go-agent/splitio/storage/boltdb/collections"

	"gopkg.in/gin-gonic/gin.v1"
)

func splitChanges(c *gin.Context) {
	sinceParam := c.DefaultQuery("since", "-1")
	since, err := strconv.Atoi(sinceParam)
	if err != nil {
		since = -1
	}

	splitCollection := collections.NewSplitChangesCollection(boltdb.DBB)
	items, err := splitCollection.FetchAll()

	till := int64(since)
	splits := make([]json.RawMessage, 0)
	for _, split := range items {
		if split.Status == "ACTIVE" && split.ChangeNumber >= int64(since) {
			if split.ChangeNumber > till {
				till = split.ChangeNumber
			}
			splits = append(splits, []byte(split.JSON))
		}
	}

	c.JSON(http.StatusOK, gin.H{"splits": splits, "since": since, "till": till})
}
