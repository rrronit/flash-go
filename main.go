package main

import (
	"context"
	"log"
	"os"
	"runtime"

	"flash-go/internal/api"
	"flash-go/internal/redis"
	"flash-go/internal/worker"

	"github.com/gin-gonic/gin"
)

func main() {
	redisURL := getenv("REDIS_URL", "redis://127.0.0.1/")
	port := getenv("PORT", "3001")
	poolType := getenv("POOL_TYPE", "event")

	redisClient, err := redis.New(redisURL)
	if err != nil {
		log.Fatalf("redis init failed: %v", err)
	}

	ctx := context.Background()
	concurrency := runtime.NumCPU() * 2
	go func() {
		worker.New(redisClient).Start(ctx, concurrency, poolType)
	}()

	router := gin.Default()
	api.RegisterRoutes(router, api.NewHandler(redisClient))

	addr := ":" + port
	log.Printf("Server running on http://0.0.0.0%s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
