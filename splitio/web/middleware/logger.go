package middleware

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/splitio/split-synchronizer/v4/log"
)

// Logger middleware to log HTTP requests at Debug level
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start timer
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		// Stop timer
		end := time.Now()
		latency := end.Sub(start)

		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()

		message := fmt.Sprintf("%s |%3d| %v | %s | %s",
			method,
			statusCode,
			latency,
			clientIP,
			path)

		log.Instance.Debug(message)
	}
}
