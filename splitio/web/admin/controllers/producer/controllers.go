package producer

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/splitio/storage"
)

// HealthCheck returns the service uptime
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
