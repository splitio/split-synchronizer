package proxy

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/splitio/go-agent/log"
	"github.com/splitio/go-agent/splitio/api"
	"github.com/splitio/go-agent/splitio/proxy/controllers"
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
		switch err {
		case boltdb.ErrorBucketNotFound:
			log.Warning.Printf("Bucket not found for segment [%s]\n", segmentName)
		default:
			log.Error.Println(err)
		}
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

//-----------------------------------------------------------------
//                 I M P R E S S I O N S
//-----------------------------------------------------------------
func postBulkImpressions(c *gin.Context) {
	sdkVersion := c.Request.Header.Get("SplitSDKVersion")
	machineIP := c.Request.Header.Get("SplitSDKMachineIP")
	data, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		log.Error.Println(err)
		c.JSON(http.StatusInternalServerError, nil)
	}

	controllers.AddImpressions(data, sdkVersion, machineIP)

	c.JSON(http.StatusOK, nil)
}

//-----------------------------------------------------------------
//-----------------------------------------------------------------

func postMetricsTimes(c *gin.Context) {
	postEvent(c, api.PostMetricsLatency)
	c.JSON(http.StatusOK, "")
}

func postMetricsCount(c *gin.Context) {
	postEvent(c, api.PostMetricsCounters)
	c.JSON(http.StatusOK, "")
}

func postMetricsGauge(c *gin.Context) {
	postEvent(c, api.PostMetricsGauge)
	c.JSON(http.StatusOK, "")
}

func postEvent(c *gin.Context, fn func([]byte, string, string) error) {
	sdkVersion := c.Request.Header.Get("SplitSDKVersion")
	machineIP := c.Request.Header.Get("SplitSDKMachineIP")
	data, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		log.Error.Println(err)
	}

	// TODO add channel to control number of posts
	go func() {
		log.Debug.Println(sdkVersion, machineIP, string(data))
		var e = fn(data, sdkVersion, machineIP)
		if e != nil {
			log.Error.Println(e)
		}
	}()
}
