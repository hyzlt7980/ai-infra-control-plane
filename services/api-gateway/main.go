package main

import (
    "net/http"

    "github.com/gin-gonic/gin"
)

func main() {
    router := gin.Default()

    router.GET("/healthz", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{
            "service": "api-gateway",
            "status":  "ok",
        })
    })

    router.GET("/readyz", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{
            "service": "api-gateway",
            "status":  "ready",
        })
    })

    // Placeholder routes for future business logic.
    router.GET("/v1/ping", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{"message": "api-gateway placeholder endpoint"})
    })

    _ = router.Run(":8080")
}
