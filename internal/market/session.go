package market

import (
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

var ist = time.FixedZone("IST", 5*3600+1800)

func AlignToNSESession(eventTime time.Time) core.MarketSession {
	local := eventTime.In(ist)
	sessionDate := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, ist)

	marketOpen := time.Date(local.Year(), local.Month(), local.Day(), 9, 15, 0, 0, ist)
	marketClose := time.Date(local.Year(), local.Month(), local.Day(), 15, 30, 0, 0, ist)

	session := core.MarketSession{
		Exchange:    "NSE",
		SessionDate: sessionDate.UTC(),
	}

	switch {
	case local.Before(marketOpen):
		session.SessionLabel = "pre_open"
		session.PreOpen = true
	case (local.Equal(marketOpen) || local.After(marketOpen)) && local.Before(marketClose):
		session.SessionLabel = "during_session"
		session.DuringSession = true
	default:
		session.SessionLabel = "post_close"
		session.PostClose = true
	}

	return session
}

func LabelWindowEnd(eventTime time.Time) time.Time {
	session := AlignToNSESession(eventTime)
	sd := session.SessionDate.In(ist)
	return time.Date(sd.Year(), sd.Month(), sd.Day(), 15, 30, 0, 0, ist).Add(24 * time.Hour).UTC()
}
