package config

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

type AppConfig struct {
	Port        string
	DatabaseURL string
	RedisAddr   string
	JWTSecret   string

	DB    *pgxpool.Pool
	Redis *redis.Client
}

func Init() (*AppConfig, error) {
	if err := godotenv.Load(); err != nil {
		log.Println("Notice: No .env file found, using system environment variables")
	}

	app := &AppConfig{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", ""),
		RedisAddr:   getEnv("REDIS_URL", "localhost:6379"),
		JWTSecret:   getEnv("JWT_SECRET", "super_secret_key"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	poolConfig, err := pgxpool.ParseConfig(app.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid database url: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("database connection failed: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database ping failed: %w", err)
	}
	app.DB = pool
	log.Println("Connected to PostgreSQL successfully!")

	rdb := redis.NewClient(&redis.Options{
		Addr: app.RedisAddr,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}
	app.Redis = rdb
	log.Println("Connected to Redis successfully!")

	return app, nil
}

func (a *AppConfig) Close() {
	if a.DB != nil {
		a.DB.Close()
	}
	if a.Redis != nil {
		a.Redis.Close()
	}
	log.Println("All database connections closed.")
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}
