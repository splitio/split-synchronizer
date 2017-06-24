package proxy

import "gopkg.in/gin-gonic/gin.v1"

func Run(port string) {
	//gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	router.GET("/api/splitChanges", splitChanges)
	router.GET("/api/segmentChanges/:name", segmentChanges)

	router.Run(port)
}
