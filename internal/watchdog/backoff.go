package watchdog

import (
	"encoding/json"
	"os"
	"time"
)

// TickSummary is the post-tick snapshot the manager writes to .hive/last-tick.json.
// The watchdog reads it to decide whether to extend the next sleep (idle backoff).
type TickSummary struct {
	Spawned       int   `json:"spawned"`        // workers/tech-leads/qa spawned this tick
	PendingReqs   int   `json:"pending_reqs"`   // requirements in inbox or decomposed-pending
	ReadyStories  int   `json:"ready_stories"`  // stories ready for worker pickup
	ReviewStories int   `json:"review_stories"` // stories awaiting QA
	LiveWorkers   int   `json:"live_workers"`
	LiveTechLeads int   `json:"live_tech_leads"`
	LiveQA        int   `json:"live_qa"`
	Timestamp     int64 `json:"ts"` // unix seconds, for debugging
}

// IsIdle returns true when no work is in flight or waiting — safe to extend sleep.
func (s TickSummary) IsIdle() bool {
	return s.Spawned == 0 &&
		s.PendingReqs == 0 &&
		s.ReadyStories == 0 &&
		s.ReviewStories == 0 &&
		s.LiveWorkers == 0 &&
		s.LiveTechLeads == 0 &&
		s.LiveQA == 0
}

// ReadTickSummary loads the manager's last tick summary. Missing/corrupt → zero
// value + error; callers should treat that as "non-idle" (safer than backing off).
func ReadTickSummary(path string) (TickSummary, error) {
	var s TickSummary
	data, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return TickSummary{}, err
	}
	return s, nil
}

// NextDelay returns the watchdog's next sleep duration given how many consecutive
// idle ticks have elapsed. Formula: base * factor^consecutiveIdle, capped at max.
// When max ≤ base or factor ≤ 1, backoff is disabled and base is returned.
func NextDelay(base, max time.Duration, factor float64, consecutiveIdle int) time.Duration {
	if max <= base || factor <= 1.0 || consecutiveIdle <= 0 {
		return base
	}
	delay := float64(base)
	for i := 0; i < consecutiveIdle; i++ {
		delay *= factor
		if delay >= float64(max) {
			return max
		}
	}
	return time.Duration(delay)
}
