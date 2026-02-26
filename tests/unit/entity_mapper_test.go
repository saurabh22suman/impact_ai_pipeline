package unit

import (
	"testing"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/enrich"
)

func TestEntityMapperDoesNotMatchAISubstringInLargerWord(t *testing.T) {
	mapper := enrich.NewEntityMapper([]config.Entity{
		{ID: "theme-ai", Symbol: "AI", Name: "AI", Aliases: []string{"AI"}, Enabled: true},
	})

	article := core.Article{
		Title:   "Executives said the main concern was regulation",
		Summary: "No standalone token appears here",
		Body:    "The briefing remained focused on cost controls.",
	}

	matches := mapper.Map(article)
	if len(matches) != 0 {
		t.Fatalf("expected no AI match from substrings, got %d matches", len(matches))
	}
}

func TestEntityMapperMatchesStandaloneAIToken(t *testing.T) {
	mapper := enrich.NewEntityMapper([]config.Entity{
		{ID: "theme-ai", Symbol: "AI", Name: "AI", Aliases: []string{"AI"}, Enabled: true},
	})

	article := core.Article{
		Title:   "AI investments increase in enterprise platforms",
		Summary: "Boards approved additional AI budgets",
	}

	matches := mapper.Map(article)
	if len(matches) != 1 {
		t.Fatalf("expected one standalone AI match, got %d", len(matches))
	}
	if matches[0].Symbol != "AI" {
		t.Fatalf("expected AI symbol match, got %q", matches[0].Symbol)
	}
}
