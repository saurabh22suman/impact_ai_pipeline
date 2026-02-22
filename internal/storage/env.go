package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func NewStoreFromEnv(ctx context.Context) (EngineStore, func() error, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_MODE")))
	if mode == "" {
		mode = "file"
	}

	switch mode {
	case "memory":
		return NewInMemoryStore(), func() error { return nil }, nil
	case "file":
		root := envOrDefault("FILE_STORE_DIR", filepath.Join(".", "outputs"))
		store, err := NewFileStore(root)
		if err != nil {
			return nil, nil, err
		}
		return store, func() error { return nil }, nil
	case "persistent":
		postgresDSN := postgresDSNFromEnv()
		pg, err := NewPostgresStore(ctx, postgresDSN)
		if err != nil {
			return nil, nil, err
		}

		clickhouseHost := envOrDefault("CLICKHOUSE_HOST", "clickhouse")
		clickhousePort := envOrDefault("CLICKHOUSE_PORT", "8123")
		clickhouseDB := envOrDefault("CLICKHOUSE_DATABASE", "default")
		clickhouseUser := envOrDefault("CLICKHOUSE_USER", "default")
		clickhousePass := os.Getenv("CLICKHOUSE_PASSWORD")

		ch, err := NewClickHouseStore(ctx, clickhouseHost, clickhousePort, clickhouseDB, clickhouseUser, clickhousePass)
		if err != nil {
			_ = pg.Close()
			return nil, nil, err
		}

		minioEndpoint := envOrDefault("MINIO_ENDPOINT", "minio:9000")
		minioAccess := envOrDefault("MINIO_ACCESS_KEY", "minioadmin")
		minioSecret := envOrDefault("MINIO_SECRET_KEY", "minioadmin")
		minioBucket := envOrDefault("MINIO_BUCKET", "ai-impact-artifacts")
		minioUseSSL := envBoolOrDefault("MINIO_USE_SSL", false)

		minioStore, err := NewMinIOStore(ctx, minioEndpoint, minioAccess, minioSecret, minioBucket, minioUseSSL)
		if err != nil {
			_ = ch.Close()
			_ = pg.Close()
			return nil, nil, err
		}

		persistent, err := NewPersistentStore(pg, ch, minioStore)
		if err != nil {
			_ = ch.Close()
			_ = pg.Close()
			return nil, nil, err
		}

		cleanup := func() error {
			return persistent.Close()
		}
		return persistent, cleanup, nil
	default:
		return nil, nil, fmt.Errorf("unsupported STORAGE_MODE %q", mode)
	}
}

func postgresDSNFromEnv() string {
	if explicit := strings.TrimSpace(os.Getenv("POSTGRES_DSN")); explicit != "" {
		return explicit
	}
	host := envOrDefault("POSTGRES_HOST", "postgres")
	port := envOrDefault("POSTGRES_PORT", "5432")
	user := envOrDefault("POSTGRES_USER", "aiimpact")
	password := envOrDefault("POSTGRES_PASSWORD", "aiimpact")
	db := envOrDefault("POSTGRES_DB", "aiimpact")
	sslmode := envOrDefault("POSTGRES_SSLMODE", "disable")
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, password, host, port, db, sslmode)
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBoolOrDefault(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return parsed
}
