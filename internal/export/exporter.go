package export

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

type Exporter struct{}

func NewExporter() Exporter {
	return Exporter{}
}

func (e Exporter) JSONL(events []core.MarketAlignedEvent) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buf)
	for _, event := range events {
		if err := enc.Encode(event); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func (e Exporter) CSV(rows []core.FeatureRow) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	writer := csv.NewWriter(buf)
	headers := []string{"Index", "News Source", "URL", "Parent entity", "Child Entity", "Sentiment", "Weight", "Confidence Score", "Cost", "Summary"}
	if err := writer.Write(headers); err != nil {
		return nil, err
	}

	for idx, row := range rows {
		rec := []string{
			strconv.Itoa(idx + 1),
			strings.TrimSpace(row.NewsSource),
			strings.TrimSpace(row.URL),
			strings.TrimSpace(row.ParentEntity),
			strings.TrimSpace(row.ChildEntity),
			strings.TrimSpace(row.SentimentDisplay),
			fmt.Sprintf("%.6f", row.Weight),
			fmt.Sprintf("%.6f", row.ConfidenceScore),
			fmt.Sprintf("%.6f", row.EstimatedCostUS),
			truncateWords(row.Summary, 10),
		}
		if err := writer.Write(rec); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buf.Bytes(), writer.Error()
}

func truncateWords(text string, max int) string {
	words := strings.Fields(strings.TrimSpace(text))
	if max <= 0 || len(words) == 0 {
		return ""
	}
	if len(words) <= max {
		return strings.Join(words, " ")
	}
	return strings.Join(words[:max], " ")
}

func (e Exporter) TOON(events []core.MarketAlignedEvent) ([]byte, error) {
	// Compatibility mode: TOON edge format represented as line-delimited JSON objects.
	type toonEvent struct {
		RunID           string   `json:"run_id"`
		ConfigVersion   string   `json:"config_version"`
		PipelineProfile string   `json:"pipeline_profile"`
		Provider        string   `json:"provider"`
		Model           string   `json:"model"`
		PromptVersion   string   `json:"prompt_version"`
		ArticleID       string   `json:"article_id"`
		SourceID        string   `json:"source_id"`
		Title           string   `json:"title"`
		URL             string   `json:"url"`
		Symbols         []string `json:"symbols"`
		Factors         []string `json:"factors"`
		SentimentLabel  string   `json:"sentiment_label"`
		SentimentScore  float64  `json:"sentiment_score"`
		RelevanceScore  float64  `json:"relevance_score"`
		SessionDate     string   `json:"session_date"`
		SessionLabel    string   `json:"session_label"`
		InputTokens     int      `json:"input_tokens"`
		OutputTokens    int      `json:"output_tokens"`
		EstimatedCostUS float64  `json:"estimated_cost_usd"`
	}

	buf := bytes.NewBuffer(nil)
	enc := json.NewEncoder(buf)
	for _, event := range events {
		symbols := make([]string, 0, len(event.Event.Entities))
		for _, entity := range event.Event.Entities {
			symbols = append(symbols, entity.Symbol)
		}
		sort.Strings(symbols)

		factors := make([]string, 0, len(event.Event.Factors))
		for _, factor := range event.Event.Factors {
			factors = append(factors, factor.FactorID)
		}
		sort.Strings(factors)

		row := toonEvent{
			RunID:           event.Event.Metadata.RunID,
			ConfigVersion:   event.Event.Metadata.ConfigVersion,
			PipelineProfile: event.Event.Metadata.PipelineProfile,
			Provider:        event.Event.Metadata.Provider,
			Model:           event.Event.Metadata.Model,
			PromptVersion:   event.Event.Metadata.PromptVersion,
			ArticleID:       event.Event.Article.ID,
			SourceID:        event.Event.Article.SourceID,
			Title:           event.Event.Article.Title,
			URL:             event.Event.Article.URL,
			Symbols:         symbols,
			Factors:         factors,
			SentimentLabel:  event.Event.SentimentLabel,
			SentimentScore:  event.Event.SentimentScore,
			RelevanceScore:  event.Event.RelevanceScore,
			SessionDate:     event.Session.SessionDate.Format("2006-01-02"),
			SessionLabel:    event.Session.SessionLabel,
			InputTokens:     event.Event.Metadata.InputTokens,
			OutputTokens:    event.Event.Metadata.OutputTokens,
			EstimatedCostUS: event.Event.Metadata.EstimatedCostUS,
		}
		if err := enc.Encode(row); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}
