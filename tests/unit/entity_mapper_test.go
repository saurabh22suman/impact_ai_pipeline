package unit

import (
	"testing"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/enrich"
)

func TestEntityMapperUsesBoundaryAwareMatching(t *testing.T) {
	mapper := enrich.NewEntityMapper([]config.Entity{{
		ID:      "nse-infy",
		Name:    "Infosys",
		Symbol:  "INFY",
		Aliases: []string{"INFY"},
		Enabled: true,
	}})

	matches := mapper.Map(core.Article{Title: "Infytech outlook improves"})
	if len(matches) != 0 {
		t.Fatalf("expected no matches, got %+v", matches)
	}
}
