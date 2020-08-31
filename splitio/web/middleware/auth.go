// Package middleware implements proxy middleware functions
package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// ValidateAPIKeys validates a list of given apiKey
func ValidateAPIKeys(keys []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var apiKey string
		auth := strings.Split(c.Request.Header.Get("Authorization"), " ")
		if len(auth) == 2 {
			apiKey = auth[1]
		} else if len(auth) == 1 {
			apiKey = auth[0]
		} else {
			c.AbortWithStatus(401)
		}

		var validKey = false
		for _, key := range keys {
			if apiKey == key {
				validKey = true
				break
			}
		}

		if !validKey {
			c.AbortWithStatus(401)
		}

		c.Next()
	}
}

// HTTPBasicAuth middleware to check basic credentials
func HTTPBasicAuth(username string, password string) gin.HandlerFunc {
	return func(c *gin.Context) {

		c.Writer.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)

		rUsername, rPassword, authOK := c.Request.BasicAuth() //r.BasicAuth()
		if authOK == false {
			c.AbortWithStatus(401)
			return
		}

		if rUsername != username || rPassword != password {
			c.AbortWithStatus(401)
			return
		}

		c.Next()

	}
}
