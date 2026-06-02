package watchdog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNextDelay(t *testing.T) {
	const base = 60 * time.Second
	const max = 600 * time.Second
	const factor = 2.0

	cases := []struct {
		name             string
		consecutiveIdle  int
		base, max        time.Duration
		factor           float64
		want             time.Duration
	}{
		{"first tick is base", 0, base, max, factor, base},
		{"one idle doubles", 1, base, max, factor, 120 * time.Second},
		{"two idles quadruple", 2, base, max, factor, 240 * time.Second},
		{"three idles octuple", 3, base, max, factor, 480 * time.Second},
		{"four idles cap at max", 4, base, max, factor, max},
		{"ten idles still cap", 10, base, max, factor, max},
		{"max ≤ base disables backoff", 5, base, 30 * time.Second, factor, base},
		{"factor ≤ 1 disables backoff", 5, base, max, 1.0, base},
		{"factor 1.5 with three idles", 3, base, max, 1.5, time.Duration(60 * 1.5 * 1.5 * 1.5 * float64(time.Second))},
		{"negative consecutive treated as zero", -1, base, max, factor, base},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NextDelay(tc.base, tc.max, tc.factor, tc.consecutiveIdle)
			if got != tc.want {
				t.Errorf("NextDelay(base=%s, max=%s, factor=%v, idle=%d) = %s, want %s",
					tc.base, tc.max, tc.factor, tc.consecutiveIdle, got, tc.want)
			}
		})
	}
}

func TestReadTickSummary_RoundTrips(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "last-tick.json")
	want := TickSummary{
		Spawned:       2,
		PendingReqs:   1,
		ReadyStories:  3,
		ReviewStories: 1,
		LiveWorkers:   2,
		LiveTechLeads: 0,
		LiveQA:        1,
		Timestamp:     1717100000,
	}
	data, _ := json.Marshal(want)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadTickSummary(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != want {
		t.Errorf("ReadTickSummary = %+v, want %+v", got, want)
	}
}

func TestReadTickSummary_MissingFileErrors(t *testing.T) {
	_, err := ReadTickSummary(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestSleepInterrupted_InboxChangeBreaksOut(t *testing.T) {
	inbox := t.TempDir()
	// pre-existing processed/ dir should NOT trigger interruption (it's filtered out)
	if err := os.Mkdir(filepath.Join(inbox, "processed"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Drop a new file after a short delay; sleepInterrupted should return true.
	go func() {
		time.Sleep(6 * time.Second) // longer than the 5s poll interval
		_ = os.WriteFile(filepath.Join(inbox, "req-1.txt"), []byte("hello"), 0o644)
	}()

	start := time.Now()
	interrupted := sleepInterrupted(context.Background(), make(chan os.Signal), inbox, 30*time.Second)
	elapsed := time.Since(start)

	if !interrupted {
		t.Errorf("expected interruption when inbox file appears, got false")
	}
	if elapsed >= 25*time.Second {
		t.Errorf("expected to break out well before the 30s deadline, took %s", elapsed)
	}
}

func TestSleepInterrupted_NaturalExpiryReturnsFalse(t *testing.T) {
	inbox := t.TempDir()
	start := time.Now()
	interrupted := sleepInterrupted(context.Background(), make(chan os.Signal), inbox, 100*time.Millisecond)
	elapsed := time.Since(start)
	if interrupted {
		t.Errorf("expected false on natural expiry, got true")
	}
	if elapsed < 100*time.Millisecond {
		t.Errorf("expected to wait at least the requested duration, only waited %s", elapsed)
	}
}

func TestSleepInterrupted_CtxCancelReturnsFalse(t *testing.T) {
	inbox := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	interrupted := sleepInterrupted(ctx, make(chan os.Signal), inbox, 30*time.Second)
	elapsed := time.Since(start)
	if interrupted {
		t.Errorf("expected false on ctx cancel, got true")
	}
	if elapsed > 5*time.Second {
		t.Errorf("ctx cancel should bail quickly, took %s", elapsed)
	}
}

func TestTickSummary_IsIdle(t *testing.T) {
	cases := []struct {
		name string
		s    TickSummary
		want bool
	}{
		{"all zero is idle", TickSummary{}, true},
		{"any spawn breaks idle", TickSummary{Spawned: 1}, false},
		{"pending req breaks idle", TickSummary{PendingReqs: 1}, false},
		{"ready story breaks idle", TickSummary{ReadyStories: 1}, false},
		{"review story breaks idle", TickSummary{ReviewStories: 1}, false},
		{"live worker breaks idle", TickSummary{LiveWorkers: 1}, false},
		{"live tech lead breaks idle", TickSummary{LiveTechLeads: 1}, false},
		{"live qa breaks idle", TickSummary{LiveQA: 1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.s.IsIdle(); got != tc.want {
				t.Errorf("IsIdle() = %v, want %v (summary=%+v)", got, tc.want, tc.s)
			}
		})
	}
}
