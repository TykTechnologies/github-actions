package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
)

// State records the release cycles seen on the previous run, keyed by product
// slug. It is what makes "a new version appeared" detectable: the API itself
// carries no notion of when a cycle was added to it.
type State map[string][]string

// loadState reads the state file. A missing file is not an error: it marks a
// seed run, reported through the second return value.
func loadState(path string) (State, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to read state file %s: %w", path, err)
	}

	state := State{}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, false, fmt.Errorf("failed to parse state file %s: %w", path, err)
	}

	return state, true, nil
}

// saveState writes the state file with sorted keys and releases, so a run that
// changes nothing produces no diff.
func saveState(path string, state State) error {
	for product := range state {
		sort.Strings(state[product])
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode state: %w", err)
	}

	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("failed to write state file %s: %w", path, err)
	}

	return nil
}

// has reports whether the product has ever been recorded. A product that is
// absent has never been polled before, so its existing releases are a baseline
// rather than news.
func (s State) has(product string) bool {
	_, ok := s[product]

	return ok
}

// seen returns the release names already recorded for a product.
func (s State) seen(product string) map[string]bool {
	releases := make(map[string]bool, len(s[product]))
	for _, release := range s[product] {
		releases[release] = true
	}

	return releases
}

// record replaces the recorded releases for a product.
func (s State) record(product string, releases []Release) {
	names := make([]string, 0, len(releases))
	for _, release := range releases {
		names = append(names, release.Name)
	}
	sort.Strings(names)

	s[product] = names
}
