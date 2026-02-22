package unit

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/nse"
)

func TestBuildBhavcopyURLDefaultUsesDDMMYYYY(t *testing.T) {
	dt := time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC)
	url := nse.BuildBhavcopyURL(dt, "")
	if !strings.Contains(url, "sec_bhavdata_full_20022026.csv") {
		t.Fatalf("expected DDMMYYYY date stamp in default url, got %q", url)
	}
}

func TestBuildBhavcopyURLLegacyPatternUsesYYYYMMDD(t *testing.T) {
	dt := time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC)
	url := nse.BuildBhavcopyURL(dt, "https://example.com/BhavCopy_%s.csv.zip")
	if !strings.Contains(url, "20260220") {
		t.Fatalf("expected YYYYMMDD date stamp in legacy pattern url, got %q", url)
	}
}

func TestBuildBhavcopyURLSecBhavdataPatternUsesDDMMYYYY(t *testing.T) {
	dt := time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC)
	url := nse.BuildBhavcopyURL(dt, "https://archives.nseindia.com/products/content/sec_bhavdata_full_%s.csv")
	if !strings.Contains(url, "20022026") {
		t.Fatalf("expected DDMMYYYY date stamp for sec_bhavdata pattern, got %q", url)
	}
}

func TestParseBhavcopyRowsFromArchive(t *testing.T) {
	archive := mustCreateBhavcopyZip(t, "bhavcopy.csv", "SYMBOL,SERIES,NAME OF COMPANY\nINFY,EQ,Infosys Limited\nNIFTIT,INDEX,Nifty IT\n")
	rows, err := nse.ParseBhavcopyRows(archive)
	if err != nil {
		t.Fatalf("parse archive: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Symbol != "INFY" || rows[1].Symbol != "NIFTIT" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestParseBhavcopyRowsFromPlainCSV(t *testing.T) {
	rows, err := nse.ParseBhavcopyRows([]byte("SYMBOL,SERIES,NAME OF COMPANY\nINFY,EQ,Infosys Limited\n"))
	if err != nil {
		t.Fatalf("parse csv payload: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Symbol != "INFY" {
		t.Fatalf("expected INFY row, got %+v", rows[0])
	}
}

func TestNormalizeBhavcopyRowsIncludesIndexFallbacks(t *testing.T) {
	rows := []nse.BhavcopyRow{
		{Symbol: "INFY", Series: "EQ", Name: "Infosys Limited"},
		{Symbol: "TCS", Series: "EQ", Name: "Tata Consultancy Services"},
		{Symbol: "NIFTIT", Series: "INDEX", Name: "Nifty IT"},
	}
	existing := []config.Entity{{
		ID:       "nse-infobean",
		Symbol:   "INFOBEAN",
		Name:     "Infobeans",
		Aliases:  []string{"INFOBEAN", "Infobeans"},
		Exchange: "NSE",
		Sector:   "IT",
		Type:     config.EntityTypeEquity,
		Enabled:  true,
	}}

	normalized := nse.NormalizeBhavcopyRows(rows, existing)
	bySymbol := map[string]config.Entity{}
	for _, entity := range normalized {
		bySymbol[entity.Symbol] = entity
	}

	if _, ok := bySymbol["INFY"]; !ok {
		t.Fatalf("expected INFY in normalized entities")
	}
	if _, ok := bySymbol["TCS"]; !ok {
		t.Fatalf("expected TCS in normalized entities")
	}
	if _, ok := bySymbol["INFOBEAN"]; !ok {
		t.Fatalf("expected existing custom entity INFOBEAN to be preserved")
	}
	if niftIT, ok := bySymbol["NIFTIT"]; !ok {
		t.Fatalf("expected NIFTIT fallback entity")
	} else if niftIT.Type != config.EntityTypeIndex {
		t.Fatalf("expected NIFTIT type=index, got %q", niftIT.Type)
	}
	if niftBank, ok := bySymbol["NIFTBANK"]; !ok {
		t.Fatalf("expected NIFTBANK fallback entity")
	} else if niftBank.Type != config.EntityTypeIndex {
		t.Fatalf("expected NIFTBANK type=index, got %q", niftBank.Type)
	}
}

func TestParseBhavcopyCSVRejectsMissingSymbolSeriesHeaders(t *testing.T) {
	_, err := nse.ParseBhavcopyCSV(strings.NewReader("foo,bar\na,b\n"))
	if err == nil {
		t.Fatalf("expected missing header error")
	}
}

func TestParseBhavcopyCSVSucceedsWithoutCompanyNameHeader(t *testing.T) {
	rows, err := nse.ParseBhavcopyCSV(strings.NewReader("SYMBOL,SERIES\nINFY,EQ\n"))
	if err != nil {
		t.Fatalf("expected parser to accept missing company name header, got %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Name != "INFY" {
		t.Fatalf("expected symbol fallback as name, got %q", rows[0].Name)
	}
}

func mustCreateBhavcopyZip(t *testing.T, name, csvContent string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(name)
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	if _, err := f.Write([]byte(csvContent)); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}
