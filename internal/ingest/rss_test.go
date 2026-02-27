package ingest

import (
	"strings"
	"testing"
	"time"
)

func TestParseRSSSupportsISO88591Encoding(t *testing.T) {
	payload := append([]byte(`<?xml version="1.0" encoding="ISO-8859-1"?>
<rss version="2.0">
  <channel>
    <item>
      <title>Infosys caf`), 0xE9)
	payload = append(payload, []byte(` demand rises</title>
      <link>https://example.com/a1</link>
      <description>Strong growth update</description>
      <pubDate>Mon, 23 Feb 2026 10:00:00 +0000</pubDate>
      <guid>a1</guid>
    </item>
  </channel>
</rss>`)...)

	doc, err := parseRSS(payload)
	if err != nil {
		t.Fatalf("parse rss: %v", err)
	}
	if len(doc.Channel.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(doc.Channel.Items))
	}
	if !strings.Contains(doc.Channel.Items[0].Title, "café") {
		t.Fatalf("expected title to decode ISO-8859-1 bytes, got %q", doc.Channel.Items[0].Title)
	}
}

func TestParsePubDateRFC3339(t *testing.T) {
	now := time.Date(2026, 2, 23, 10, 0, 0, 0, time.UTC)
	got := parsePubDate("2026-02-23T09:30:00Z", func() time.Time { return now })
	if got.IsZero() {
		t.Fatalf("expected parsed timestamp")
	}
	if got.Format(time.RFC3339) != "2026-02-23T09:30:00Z" {
		t.Fatalf("unexpected parsed timestamp: %s", got.Format(time.RFC3339))
	}
}
