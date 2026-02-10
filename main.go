package main

import (
	"context"
	"log"
	"runtime"

	"flash-go/internal/api"
	"flash-go/internal/redis"
	"flash-go/internal/utils"
	"flash-go/internal/worker"

	"github.com/gin-gonic/gin"
)

func main() {
	redisURL := utils.EnvString("REDIS_URL", "redis://127.0.0.1/")
	port := utils.EnvString("PORT", "3001")
	useBoxPool := utils.EnvBool("USE_BOX_POOL", false)
	queueLengthLimit := utils.EnvInt("QUEUE_LENGTH_LIMIT", 1000)

	redisClient, err := redis.New(redisURL)
	if err != nil {
		log.Fatalf("redis init failed: %v", err)
	}

	ctx := context.Background()
	concurrency := runtime.NumCPU() * 4

	go func() {
		worker.New(redisClient).Start(ctx, concurrency, useBoxPool)
	}()

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	api.RegisterRoutes(router, api.NewHandler(redisClient, queueLengthLimit, concurrency, useBoxPool))

	addr := ":" + port
	log.Printf("Server running on http://0.0.0.0%s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
