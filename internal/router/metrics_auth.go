package router

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func registerMetrics(r *gin.Engine, token string) {
	handler := promhttp.Handler()
	r.GET("/metrics", func(c *gin.Context) {
		// Explicit 404 avoids serving the SPA for a disabled metrics endpoint.
		if token == "" {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		if subtle.ConstantTimeCompare([]byte(c.GetHeader("Authorization")), []byte("Bearer "+token)) != 1 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(c.Writer, c.Request)
	})
}
