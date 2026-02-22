package eval

import (
	"sort"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

type Fold struct {
	TrainStart time.Time
	TrainEnd   time.Time
	TestStart  time.Time
	TestEnd    time.Time
}

func BuildPurgedWalkForward(rows []core.FeatureRow, trainDays, testDays, embargoDays int) []Fold {
	if len(rows) == 0 || trainDays <= 0 || testDays <= 0 {
		return nil
	}

	dates := uniqueDates(rows)
	if len(dates) < trainDays+testDays {
		return nil
	}

	folds := make([]Fold, 0)
	start := 0
	for {
		trainStart := start
		trainEnd := trainStart + trainDays - 1
		testStart := trainEnd + 1 + embargoDays
		testEnd := testStart + testDays - 1
		if testEnd >= len(dates) {
			break
		}
		folds = append(folds, Fold{
			TrainStart: dates[trainStart],
			TrainEnd:   dates[trainEnd],
			TestStart:  dates[testStart],
			TestEnd:    dates[testEnd],
		})
		start++
	}
	return folds
}

func uniqueDates(rows []core.FeatureRow) []time.Time {
	set := map[string]time.Time{}
	for _, row := range rows {
		day := row.SessionDate.UTC().Format("2006-01-02")
		set[day] = row.SessionDate.UTC()
	}
	out := make([]time.Time, 0, len(set))
	for _, ts := range set {
		out = append(out, ts)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}
