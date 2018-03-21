package producer

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/splitio/storage"
	"github.com/splitio/split-synchronizer/splitio/web/dashboard"
)

// HealthCheck returns the service status
func HealthCheck(c *gin.Context) {

	producerStatus := make(map[string]interface{})
	storageStatus := make(map[string]interface{})

	// Producer service
	producerStatus["healthy"] = true
	producerStatus["message"] = "Synchronizer service working as expected"

	// Storage service
	splitStorage, exists := c.Get("SplitStorage")
	if exists {
		_, err := splitStorage.(storage.SplitStorage).ChangeNumber()
		if err != nil {
			storageStatus["healthy"] = false
			storageStatus["message"] = err.Error()
		} else {
			storageStatus["healthy"] = true
			storageStatus["message"] = "Storage service working as expected"
		}
	}

	if storageStatus["healthy"].(bool) {
		c.JSON(http.StatusOK, gin.H{"sync": producerStatus, "storage": storageStatus})
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"sync": producerStatus, "storage": storageStatus})
	}

}

// Dashboard returns a dashboard
func Dashboard(c *gin.Context) {

	// Storage service
	splitStorage, _ := c.Get("SplitStorage")
	segmentStorage, _ := c.Get("SegmentStorage")

	dash := dashboard.NewDashboard(conf.Data.Producer.Admin.Title, false,
		splitStorage.(storage.SplitStorage),
		segmentStorage.(storage.SegmentStorage),
	)

	//Write your 200 header status (or other status codes, but only WriteHeader once)
	c.Writer.WriteHeader(http.StatusOK)
	//Convert your cached html string to byte array
	c.Writer.Write([]byte(dash.HTML()))
	return
}

// DashboardSegmentKeys returns a keys for a given segment
func DashboardSegmentKeys(c *gin.Context) {

	segmentName := c.Param("segment")

	// Storage service
	splitStorage, _ := c.Get("SplitStorage")
	segmentStorage, _ := c.Get("SegmentStorage")

	dash := dashboard.NewDashboard(conf.Data.Producer.Admin.Title, false,
		splitStorage.(storage.SplitStorage),
		segmentStorage.(storage.SegmentStorage),
	)

	var toReturn = dash.HTMLSegmentKeys(segmentName)

	c.String(http.StatusOK, "%s", toReturn)
}
