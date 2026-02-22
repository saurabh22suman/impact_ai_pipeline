package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOStore struct {
	client *minio.Client
	bucket string
}

func NewMinIOStore(ctx context.Context, endpoint, accessKey, secretKey, bucket string, useSSL bool) (*MinIOStore, error) {
	if endpoint == "" {
		endpoint = "minio:9000"
	}
	if bucket == "" {
		bucket = "ai-impact-artifacts"
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("new minio client: %w", err)
	}

	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("check minio bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create minio bucket: %w", err)
		}
	}

	return &MinIOStore{client: client, bucket: bucket}, nil
}

func (s *MinIOStore) Put(ctx context.Context, key string, payload []byte) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("minio store is nil")
	}
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("artifact key is required")
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{})
	return err
}

func (s *MinIOStore) Get(ctx context.Context, key string) ([]byte, bool) {
	if s == nil || s.client == nil || strings.TrimSpace(key) == "" {
		return nil, false
	}
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, false
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, false
	}
	return data, true
}
