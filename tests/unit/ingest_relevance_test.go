package unit

import (
	"testing"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/ingest"
)

func TestDedupeByCanonicalHash(t *testing.T) {
	d := ingest.NewDedupeEngine()
	in := []core.Article{
		{ID: "1", CanonicalHash: "abc", URL: "u1", Title: "t1"},
		{ID: "2", CanonicalHash: "abc", URL: "u2", Title: "t2"},
		{ID: "3", CanonicalHash: "def", URL: "u3", Title: "t3"},
	}
	out := d.Filter(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 unique items, got %d", len(out))
	}
}

func TestRelevanceGateScoresExpectedRange(t *testing.T) {
	gate := ingest.NewRelevanceGate()
	article := core.Article{
		Title:   "Infosys expands AI capex and hiring",
		Summary: "Strong enterprise AI adoption and compliance initiatives",
		Body:    "Datacenter expansion and AI hiring plans continue",
	}
	factors := []config.Factor{
		{ID: "f1", Keywords: []string{"ai capex", "hiring", "compliance"}},
		{ID: "f2", Keywords: []string{"adoption", "margin"}},
	}
	entities := []config.Entity{
		{Name: "Infosys", Aliases: []string{"INFY"}},
		{Name: "Tata Consultancy Services", Aliases: []string{"TCS"}},
	}
	score := gate.Score(article, factors, entities)
	if score <= 0 {
		t.Fatalf("expected positive relevance score, got %.4f", score)
	}
	if score > 1 {
		t.Fatalf("expected score <= 1, got %.4f", score)
	}

	if !gate.NeedsLLMRefinement(score, 0.8, 0.2) {
		t.Fatalf("expected ambiguous score to trigger llm refinement")
	}
	if gate.NeedsLLMRefinement(0.9, 0.8, 0.2) {
		t.Fatalf("high novelty score should skip llm refinement")
	}
}

func TestRelevanceGateDoesNotMatchSymbolInsideLargerWord(t *testing.T) {
	gate := ingest.NewRelevanceGate()
	article := core.Article{Title: "Infytech demand rises", Summary: "", Body: ""}
	entities := []config.Entity{{Name: "Infosys", Symbol: "INFY", Aliases: []string{"INFY"}, Enabled: true}}

	score := gate.Score(article, nil, entities)
	if score != 0 {
		t.Fatalf("expected no INFY entity hit from Infytech, got %.4f", score)
	}
}

func TestParseTOONSupportsJSONLines(t *testing.T) {
	input := []byte("{\"id\":\"a1\",\"source_id\":\"s\",\"source_name\":\"S\",\"url\":\"https://x\",\"title\":\"T\",\"summary\":\"S\",\"body\":\"B\",\"language\":\"en\",\"region\":\"india\",\"published_at\":\"2026-02-22T12:00:00Z\"}\n{\"id\":\"a2\",\"source_id\":\"s\",\"source_name\":\"S\",\"url\":\"https://y\",\"title\":\"T2\",\"summary\":\"S2\",\"body\":\"B2\",\"language\":\"en\",\"region\":\"india\",\"published_at\":\"2026-02-22T12:05:00Z\"}")

	articles, err := ingest.ParseTOON(input)
	if err != nil {
		t.Fatalf("parse toon: %v", err)
	}
	if len(articles) != 2 {
		t.Fatalf("expected 2 articles, got %d", len(articles))
	}
	if articles[0].ID != "a1" || articles[1].ID != "a2" {
		t.Fatalf("unexpected article ids: %+v", articles)
	}
	if articles[0].PublishedAt.IsZero() || articles[0].PublishedAt.After(time.Now().Add(24*time.Hour)) {
		t.Fatalf("unexpected published timestamp parsed")
	}
}
