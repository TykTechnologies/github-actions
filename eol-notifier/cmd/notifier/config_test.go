package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, name, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("failed to write %s: %v", name, err)
	}

	return path
}

func TestLoadDependencyConfig(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		wantErr  bool
	}{
		{
			name: "valid config",
			contents: `
thresholds_months: [12, 6, 1]
dependencies:
  - name: Redis
    product: redis
    track: [eol]
  - name: GCP MemoryStore
    product: redis
    upstream_proxy: true
`,
		},
		{
			name: "minimal config relies on defaults",
			contents: `
dependencies:
  - name: Redis
    product: redis
`,
		},
		{
			name: "unknown phase is rejected",
			contents: `
dependencies:
  - name: Redis
    product: redis
    track: [eol, discontinued]
`,
			wantErr: true,
		},
		{
			name: "duplicate dependency name is rejected",
			contents: `
dependencies:
  - name: Redis
    product: redis
  - name: Redis
    product: valkey
`,
			wantErr: true,
		},
		{
			name: "empty product is rejected",
			contents: `
dependencies:
  - name: Redis
    product: ''
`,
			wantErr: true,
		},
		{
			name: "empty name is rejected",
			contents: `
dependencies:
  - name: '  '
    product: redis
`,
			wantErr: true,
		},
		{
			name:     "no dependencies is rejected",
			contents: "thresholds_months: [12]\n",
			wantErr:  true,
		},
		{
			name: "non-positive threshold is rejected",
			contents: `
thresholds_months: [12, 0]
dependencies:
  - name: Redis
    product: redis
`,
			wantErr: true,
		},
		{
			name: "duplicate threshold is rejected",
			contents: `
thresholds_months: [6, 6]
dependencies:
  - name: Redis
    product: redis
`,
			wantErr: true,
		},
		{
			name: "misspelled key is rejected",
			contents: `
dependencies:
  - name: Redis
    product: redis
    upstream-proxy: true
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFile(t, "dependencies.yaml", tt.contents)

			config, err := loadDependencyConfig(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("loadDependencyConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if len(config.ThresholdsMonths) == 0 {
				t.Error("thresholds were not defaulted")
			}
			for _, dependency := range config.Dependencies {
				if len(dependency.Track) == 0 {
					t.Errorf("dependency %q has no tracked phase after defaulting", dependency.Name)
				}
			}
		})
	}
}

func TestLoadDependencyConfigMissingFile(t *testing.T) {
	if _, err := loadDependencyConfig(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("expected an error for a missing config file")
	}
}

// TestRepositoryConfigIsValid keeps the config this repository actually ships in
// step with the validation rules.
func TestRepositoryConfigIsValid(t *testing.T) {
	path := filepath.Join("..", "..", "..", ".github", "eol-notifier", "dependencies.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("repository config not present: %v", err)
	}

	config, err := loadDependencyConfig(path)
	if err != nil {
		t.Fatalf("failed to load the repository config: %v", err)
	}

	for _, want := range []string{"redis", "valkey", "postgresql", "mongodb"} {
		found := false
		for _, dependency := range config.Dependencies {
			if dependency.Product == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("repository config does not track %q", want)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name       string
		webhookURL string
		dryRun     bool
		wantErr    bool
	}{
		{name: "webhook present", webhookURL: "https://hooks.slack.test/abc"},
		{name: "webhook missing", wantErr: true},
		{name: "webhook missing but dry run", dryRun: true},
		{name: "blank webhook", webhookURL: "   ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			process := func(prefix string, spec interface{}) error {
				if prefix != "EN" {
					t.Errorf("envconfig prefix = %q, want EN", prefix)
				}
				config, ok := spec.(*Config)
				if !ok {
					return fmt.Errorf("unexpected spec type %T", spec)
				}
				config.SlackWebhookURL = tt.webhookURL

				return nil
			}

			config, err := loadConfig(process, tt.dryRun)
			if (err != nil) != tt.wantErr {
				t.Fatalf("loadConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && config.SlackWebhookURL != tt.webhookURL {
				t.Errorf("webhook URL = %q, want %q", config.SlackWebhookURL, tt.webhookURL)
			}
		})
	}
}

func TestLoadConfigPropagatesEnvironmentErrors(t *testing.T) {
	process := func(string, interface{}) error {
		return fmt.Errorf("boom")
	}

	_, err := loadConfig(process, false)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("loadConfig() error = %v, want the underlying failure to surface", err)
	}
}
