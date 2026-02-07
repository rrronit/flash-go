package main

import (
	"context"
	"log"
	"os"
	"runtime"
	"strconv"

	"flash-go/internal/api"
	"flash-go/internal/redis"
	"flash-go/internal/worker"

	"github.com/gin-gonic/gin"
)

func main() {
	redisURL := getenv("REDIS_URL", "redis://127.0.0.1/")
	port := getenv("PORT", "3001")
	useBoxPool := getenv("USE_BOX_POOL", "true") == "true"
	queueLengthLimit := getenvInt("QUEUE_LENGTH_LIMIT", 1000)

	redisClient, err := redis.New(redisURL)
	if err != nil {
		log.Fatalf("redis init failed: %v", err)
	}

	ctx := context.Background()
	concurrency := runtime.NumCPU() * 2

	go func() {
		worker.New(redisClient).Start(ctx, concurrency, useBoxPool)
	}()

	router := gin.Default()
	api.RegisterRoutes(router, api.NewHandler(redisClient, queueLengthLimit, concurrency, useBoxPool))

	addr := ":" + port
	log.Printf("Server running on http://0.0.0.0%s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
