package nse

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
)

const defaultBhavcopyURLPattern = "https://archives.nseindia.com/products/content/sec_bhavdata_full_%s.csv"

type BhavcopyRow struct {
	Symbol string
	Series string
	Name   string
}

func BuildBhavcopyURL(date time.Time, pattern string) string {
	if strings.TrimSpace(pattern) == "" {
		pattern = defaultBhavcopyURLPattern
	}
	stamp := bhavcopyDateStampForPattern(date, pattern)
	return fmt.Sprintf(pattern, stamp)
}

func bhavcopyDateStampForPattern(date time.Time, pattern string) string {
	trimmed := strings.TrimSpace(pattern)
	upperPattern := strings.ToUpper(trimmed)
	lowerPattern := strings.ToLower(trimmed)
	if strings.Contains(upperPattern, "DDMMYYYY") {
		return date.UTC().Format("02012006")
	}
	if strings.Contains(upperPattern, "YYYYMMDD") {
		return date.UTC().Format("20060102")
	}
	if trimmed == defaultBhavcopyURLPattern || strings.Contains(lowerPattern, "sec_bhavdata_full_") {
		return date.UTC().Format("02012006")
	}
	return date.UTC().Format("20060102")
}

func FetchBhavcopyArchive(ctx context.Context, client *http.Client, date time.Time) ([]byte, string, error) {
	return FetchBhavcopyArchiveWithPattern(ctx, client, date, "")
}

func FetchBhavcopyArchiveWithPattern(ctx context.Context, client *http.Client, date time.Time, urlPattern string) ([]byte, string, error) {
	if client == nil {
		client = http.DefaultClient
	}
	url := BuildBhavcopyURL(date, urlPattern)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build bhavcopy request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/csv,application/zip,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", "https://www.nseindia.com/")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download bhavcopy: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, "", fmt.Errorf("bhavcopy download failed status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, "", fmt.Errorf("read bhavcopy payload: %w", err)
	}

	filename := bhavcopyFilename(date, url, resp.Header.Get("Content-Type"))
	return payload, filename, nil
}

func bhavcopyFilename(date time.Time, requestURL, contentType string) string {
	parsed, err := url.Parse(requestURL)
	if err == nil {
		base := path.Base(parsed.Path)
		if base != "" && base != "/" && base != "." {
			return base
		}
	}
	if isZipContentType(contentType) {
		return fmt.Sprintf("BhavCopy_NSE_CM_%s.csv.zip", date.UTC().Format("20060102"))
	}
	return fmt.Sprintf("sec_bhavdata_full_%s.csv", date.UTC().Format("02012006"))
}

func isZipContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.ToLower(strings.Split(contentType, ";")[0]))
	} else {
		mediaType = strings.ToLower(mediaType)
	}
	return mediaType == "application/zip" || mediaType == "application/x-zip-compressed"
}

func ParseBhavcopyRows(payload []byte) ([]BhavcopyRow, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty bhavcopy payload")
	}
	if rows, err := ParseBhavcopyRowsFromArchive(payload); err == nil {
		return rows, nil
	} else if !isZipFormatError(err) {
		return nil, err
	}
	return ParseBhavcopyCSV(bytes.NewReader(payload))
}

func ParseBhavcopyRowsFromArchive(payload []byte) ([]BhavcopyRow, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty bhavcopy archive payload")
	}
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return nil, fmt.Errorf("open bhavcopy zip: %w", err)
	}
	for _, file := range reader.File {
		name := strings.ToLower(file.Name)
		if !strings.HasSuffix(name, ".csv") {
			continue
		}
		fh, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open csv entry %s: %w", file.Name, err)
		}
		rows, parseErr := ParseBhavcopyCSV(fh)
		_ = fh.Close()
		if parseErr != nil {
			return nil, parseErr
		}
		return rows, nil
	}
	return nil, fmt.Errorf("bhavcopy archive has no csv entry")
}

func isZipFormatError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "zip")
}

func ParseBhavcopyCSV(r io.Reader) ([]BhavcopyRow, error) {
	cr := csv.NewReader(r)
	records, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse bhavcopy csv: %w", err)
	}
	if len(records) < 2 {
		return nil, nil
	}

	headers := make(map[string]int)
	for idx, h := range records[0] {
		headers[strings.ToUpper(strings.TrimSpace(h))] = idx
	}
	symbolIdx, okSymbol := headers["SYMBOL"]
	seriesIdx, okSeries := headers["SERIES"]
	nameIdx := -1
	if idx, ok := headers["NAME OF COMPANY"]; ok {
		nameIdx = idx
	} else if idx, ok := headers["NAME_OF_COMPANY"]; ok {
		nameIdx = idx
	}
	if !okSymbol || !okSeries {
		return nil, fmt.Errorf("bhavcopy csv missing SYMBOL/SERIES headers")
	}

	rows := make([]BhavcopyRow, 0, len(records)-1)
	for _, record := range records[1:] {
		if symbolIdx >= len(record) || seriesIdx >= len(record) {
			continue
		}
		symbol := strings.ToUpper(strings.TrimSpace(record[symbolIdx]))
		name := symbol
		if nameIdx >= 0 && nameIdx < len(record) {
			candidate := strings.TrimSpace(record[nameIdx])
			if candidate != "" {
				name = candidate
			}
		}
		rows = append(rows, BhavcopyRow{
			Symbol: symbol,
			Series: strings.ToUpper(strings.TrimSpace(record[seriesIdx])),
			Name:   name,
		})
	}
	return rows, nil
}

func NormalizeBhavcopyRows(rows []BhavcopyRow, existing []config.Entity) []config.Entity {
	bySymbol := map[string]config.Entity{}
	for _, entity := range existing {
		symbol := strings.ToUpper(strings.TrimSpace(entity.Symbol))
		if symbol == "" {
			continue
		}
		if strings.TrimSpace(entity.Type) == "" {
			entity.Type = config.DefaultEntityTypeForSymbol(entity.Symbol)
		} else {
			entity.Type = config.NormalizeEntityType(entity.Type)
		}
		bySymbol[symbol] = entity
	}

	for _, row := range rows {
		symbol := strings.ToUpper(strings.TrimSpace(row.Symbol))
		if symbol == "" {
			continue
		}
		series := strings.ToUpper(strings.TrimSpace(row.Series))
		if series != "EQ" {
			continue
		}
		name := strings.TrimSpace(row.Name)
		if name == "" {
			name = symbol
		}
		updated := config.Entity{
			ID:       buildEntityID(symbol),
			Symbol:   symbol,
			Name:     name,
			Aliases:  []string{symbol, name},
			Exchange: "NSE",
			Sector:   "Unknown",
			Type:     config.EntityTypeEquity,
			Enabled:  true,
		}
		if existingEntity, ok := bySymbol[symbol]; ok {
			updated.ID = firstNonEmpty(existingEntity.ID, updated.ID)
			updated.Name = firstNonEmpty(existingEntity.Name, updated.Name)
			updated.Exchange = firstNonEmpty(existingEntity.Exchange, updated.Exchange)
			updated.Sector = firstNonEmpty(existingEntity.Sector, updated.Sector)
			updated.Enabled = existingEntity.Enabled
			updated.Type = config.NormalizeEntityType(firstNonEmpty(existingEntity.Type, updated.Type))
			updated.Aliases = mergeAliases(existingEntity.Aliases, updated.Aliases)
		}
		bySymbol[symbol] = updated
	}

	ensureIndexEntry(bySymbol, config.Entity{ID: "nse-index-niftit", Symbol: "NIFTIT", Name: "Nifty IT", Exchange: "NSE", Sector: "Index", Type: config.EntityTypeIndex, Enabled: true, Aliases: []string{"NIFTIT", "Nifty IT"}})
	ensureIndexEntry(bySymbol, config.Entity{ID: "nse-index-niftbank", Symbol: "NIFTBANK", Name: "Nifty Bank", Exchange: "NSE", Sector: "Index", Type: config.EntityTypeIndex, Enabled: true, Aliases: []string{"NIFTBANK", "Nifty Bank"}})

	entities := make([]config.Entity, 0, len(bySymbol))
	for _, entity := range bySymbol {
		if strings.TrimSpace(entity.Symbol) == "" {
			continue
		}
		if strings.TrimSpace(entity.ID) == "" {
			entity.ID = buildEntityID(entity.Symbol)
		}
		if strings.TrimSpace(entity.Name) == "" {
			entity.Name = entity.Symbol
		}
		entity.Type = config.NormalizeEntityType(entity.Type)
		if strings.TrimSpace(entity.Exchange) == "" {
			entity.Exchange = "NSE"
		}
		entity.Aliases = uniqueNonEmpty(entity.Aliases)
		entities = append(entities, entity)
	}

	sort.Slice(entities, func(i, j int) bool {
		return entities[i].Symbol < entities[j].Symbol
	})

	return entities
}

func ensureIndexEntry(bySymbol map[string]config.Entity, fallback config.Entity) {
	symbol := strings.ToUpper(strings.TrimSpace(fallback.Symbol))
	if symbol == "" {
		return
	}
	if existing, ok := bySymbol[symbol]; ok {
		existing.Type = config.EntityTypeIndex
		existing.Enabled = true
		existing.Name = firstNonEmpty(existing.Name, fallback.Name)
		existing.Exchange = firstNonEmpty(existing.Exchange, fallback.Exchange)
		existing.Sector = firstNonEmpty(existing.Sector, fallback.Sector)
		existing.ID = firstNonEmpty(existing.ID, fallback.ID)
		existing.Aliases = mergeAliases(existing.Aliases, fallback.Aliases)
		bySymbol[symbol] = existing
		return
	}
	fallback.Aliases = uniqueNonEmpty(fallback.Aliases)
	bySymbol[symbol] = fallback
}

func buildEntityID(symbol string) string {
	trimmed := strings.ToLower(strings.TrimSpace(symbol))
	trimmed = strings.ReplaceAll(trimmed, "&", "and")
	trimmed = strings.ReplaceAll(trimmed, " ", "-")
	trimmed = strings.ReplaceAll(trimmed, "_", "-")
	return "nse-" + trimmed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func mergeAliases(parts ...[]string) []string {
	out := make([]string, 0)
	for _, part := range parts {
		out = append(out, part...)
	}
	return uniqueNonEmpty(out)
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToUpper(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func ParseLastPrice(raw string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	return v
}
