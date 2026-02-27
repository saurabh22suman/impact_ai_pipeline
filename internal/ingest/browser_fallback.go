package ingest

import (
	"context"
	"fmt"
)

type BrowserContentExtractor interface {
	Extract(ctx context.Context, pageURL string) (title, summary, body string, err error)
}

type NoopBrowserExtractor struct{}

func (NoopBrowserExtractor) Extract(context.Context, string) (string, string, string, error) {
	return "", "", "", fmt.Errorf("browser fallback not configured")
}
