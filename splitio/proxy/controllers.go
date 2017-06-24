package proxy

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/splitio/go-agent/log"
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
		if split.Status == "ACTIVE" && split.ChangeNumber > int64(since) {
			if split.ChangeNumber > till {
				till = split.ChangeNumber
			}
			splits = append(splits, []byte(split.JSON))
		}
	}

	c.JSON(http.StatusOK, gin.H{"splits": splits, "since": since, "till": till})
}

func segmentChanges(c *gin.Context) {
	sinceParam := c.DefaultQuery("since", "-1")
	since, err := strconv.Atoi(sinceParam)
	if err != nil {
		since = -1
	}

	added := make([]string, 0)
	removed := make([]string, 0)
	till := int64(since)

	segmentName := c.Param("name")
	segmentCollection := collections.NewSegmentChangesCollection(boltdb.DBB)
	item, err := segmentCollection.Fetch(segmentName)
	if err != nil {
		log.Error.Println(err)
		c.JSON(http.StatusOK, gin.H{"name": segmentName, "added": added,
			"removed": removed, "since": since, "till": till})
		return
	}

	for _, skey := range item.Keys {
		if skey.ChangeNumber > int64(since) {
			if skey.Removed {
				if since > 0 {
					removed = append(removed, skey.Name)
				}
			} else {
				added = append(added, skey.Name)
			}

			if since > 0 {
				if skey.ChangeNumber > till {
					till = skey.ChangeNumber
				}
			} else {
				if !skey.Removed && skey.ChangeNumber > till {
					till = skey.ChangeNumber
				}
			}

		}
	}
	c.JSON(http.StatusOK, gin.H{"name": segmentName, "added": added,
		"removed": removed, "since": since, "till": till})
}
