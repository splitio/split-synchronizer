package proxy

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/conf"
	"github.com/splitio/split-synchronizer/splitio/storage"
	"github.com/splitio/split-synchronizer/splitio/web/admin/controllers"
	"github.com/splitio/split-synchronizer/splitio/web/dashboard"
)

// HealthCheck returns the service status
func HealthCheck(c *gin.Context) {

	proxyStatus := make(map[string]interface{})

	// Producer service
	proxyStatus["healthy"] = true
	proxyStatus["message"] = "Proxy service working as expected"

	sdkStatus := controllers.GetSdkStatus()
	eventsStatus := controllers.GetEventsStatus()

	if sdkStatus["healthy"].(bool) || eventsStatus["healthy"].(bool) {
		c.JSON(http.StatusOK, gin.H{"proxy": proxyStatus, "sdk": sdkStatus, "events": eventsStatus})
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"proxy": proxyStatus, "sdk": sdkStatus, "events": eventsStatus})
	}
}

// Dashboard returns a dashboard
func Dashboard(c *gin.Context) {

	// Storage service
	splitStorage, _ := c.Get("SplitStorage")
	segmentStorage, _ := c.Get("SegmentStorage")

	dash := dashboard.NewDashboard(conf.Data.Proxy.Title, true,
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

	dash := dashboard.NewDashboard(conf.Data.Proxy.Title, true,
		splitStorage.(storage.SplitStorage),
		segmentStorage.(storage.SegmentStorage),
	)

	var toReturn = dash.HTMLSegmentKeys(segmentName)

	c.String(http.StatusOK, "%s", toReturn)
}
