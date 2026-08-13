package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStateMissingFileSeeds(t *testing.T) {
	state, existed, err := loadState(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatalf("loadState() error = %v, want a seed run", err)
	}
	if existed {
		t.Error("existed = true for a missing state file")
	}
	if len(state) != 0 {
		t.Errorf("state = %v, want empty", state)
	}
}

func TestLoadStateRejectsMalformedFile(t *testing.T) {
	path := writeFile(t, "state.json", "not json")

	if _, _, err := loadState(path); err == nil {
		t.Fatal("expected an error for a malformed state file")
	}
}

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	state := State{}
	state.record("postgresql", []Release{{Name: "17"}, {Name: "18"}, {Name: "16"}})
	state.markSent("postgresql", []string{"16|eol|12|2028-11-09"})

	if err := saveState(path, state); err != nil {
		t.Fatalf("saveState() error = %v", err)
	}

	reloaded, existed, err := loadState(path)
	if err != nil {
		t.Fatalf("loadState() error = %v", err)
	}
	if !existed {
		t.Error("existed = false after saving")
	}

	seen := reloaded.seen("postgresql")
	for _, release := range []string{"16", "17", "18"} {
		if !seen[release] {
			t.Errorf("release %s was not recorded", release)
		}
	}
	if seen["15"] {
		t.Error("release 15 was recorded but never seen")
	}

	if !reloaded.sent("postgresql")["16|eol|12|2028-11-09"] {
		t.Error("the delivered alert did not survive the round trip, so it would be sent again")
	}
}

// TestMarkSentIgnoresRepeats keeps a re-delivered alert from growing the state
// file on every run.
func TestMarkSentIgnoresRepeats(t *testing.T) {
	state := State{}
	state.markSent("redis", []string{"7.2|eol|1|2026-06-30", "7.2|eol|1|2026-06-30"})
	state.markSent("redis", []string{"7.2|eol|1|2026-06-30", "7.4|eol|ended|2026-05-25"})

	if got := len(state["redis"].Sent); got != 2 {
		t.Errorf("recorded %d key(s), want 2: %v", got, state["redis"].Sent)
	}
}

// TestSaveStateIsStable keeps the committed state file diff-free on runs that
// change nothing, so the job does not produce daily no-op commits.
func TestSaveStateIsStable(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.json")
	second := filepath.Join(dir, "second.json")

	stateA := State{}
	stateA.record("redis", []Release{{Name: "8.8"}, {Name: "8.2"}})
	stateA.record("postgresql", []Release{{Name: "18"}})
	stateA.markSent("redis", []string{"8.2|eol|1|2026-05-25", "8.4|eol|6|2026-10-31"})

	stateB := State{}
	stateB.record("postgresql", []Release{{Name: "18"}})
	stateB.record("redis", []Release{{Name: "8.2"}, {Name: "8.8"}})
	stateB.markSent("redis", []string{"8.4|eol|6|2026-10-31", "8.2|eol|1|2026-05-25"})

	if err := saveState(first, stateA); err != nil {
		t.Fatalf("saveState() error = %v", err)
	}
	if err := saveState(second, stateB); err != nil {
		t.Fatalf("saveState() error = %v", err)
	}

	a, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("failed to read %s: %v", first, err)
	}
	b, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("failed to read %s: %v", second, err)
	}

	if string(a) != string(b) {
		t.Errorf("state files differ despite identical content:\n%s\n---\n%s", a, b)
	}
}
