package proxy

import "gopkg.in/gin-gonic/gin.v1"

func Run(port string) {
	router := gin.Default()

	router.GET("/api/splitChanges", splitChanges)

	router.Run(port)
}
