package web

import (
	"bytes"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

//go:embed all:dist
var dist embed.FS

//go:embed dist/index.html
var indexHTML embed.FS

func RegisterHandlers(r *gin.Engine) {
	// Extract dist filesystem
	distDirFS, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}

	distIndexFS, err := fs.Sub(indexHTML, "dist")
	if err != nil {
		panic(err)
	}

	// Group under /app
	appGroup := r.Group("/app")

	// Serve static files from /app/*
	appGroup.GET("/*filepath", func(c *gin.Context) {
		filePath := strings.TrimPrefix(c.Param("filepath"), "/")
		if filePath == "" {
			filePath = "index.html"
		}

		file, err := distDirFS.Open(filePath)
		if err != nil {
			// Not found in static files — fallback to index.html for SPA routing
			serveIndexHTML(c, distIndexFS)
			return
		}

		defer file.Close()
		stat, err := file.Stat()
		if err != nil || stat.IsDir() {
			serveIndexHTML(c, distIndexFS)
			return
		}

		contentType := mime.TypeByExtension(filepath.Ext(filePath))
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		c.DataFromReader(http.StatusOK, stat.Size(), contentType, file, nil)
	})
}

// Serve index.html from distIndexFS
func serveIndexHTML(c *gin.Context, indexFS fs.FS) {
	data, err := fs.ReadFile(indexFS, "index.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "index.html not found")
		return
	}

	reader := bytes.NewReader(data)
	c.DataFromReader(http.StatusOK, int64(len(data)), "text/html; charset=utf-8", reader, map[string]string{
		"Last-Modified": time.Now().UTC().Format(http.TimeFormat),
	})
}
