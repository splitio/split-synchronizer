module github.com/splitio/split-synchronizer/v4

go 1.13

require (
	github.com/boltdb/bolt v1.3.1
	github.com/gin-contrib/cors v0.0.0-20170318125340-cf4846e6a636
	github.com/gin-contrib/gzip v0.0.3-0.20200908134145-3aff12661394
	github.com/gin-gonic/gin v1.6.3
	github.com/json-iterator/go v1.1.10 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/splitio/go-split-commons/v3 v3.0.2-rc2
	github.com/splitio/go-toolkit/v4 v4.2.1
)

//replace github.com/splitio/go-split-commons/v3 => /Users/martinredolatti/split-sync-box/go-split-commons
