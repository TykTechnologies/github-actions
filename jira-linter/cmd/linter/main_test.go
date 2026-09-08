package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andygrunwald/go-jira"
)

func TestFindIssueID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"uppercase in branch", "TT-16922-lts-resilience-pipeline", "TT-16922", false},
		{"lowercase in branch", "tt-16922-lts-resilience-pipeline", "TT-16922", false},
		{"mixed case", "Tt-16922-some-feature", "TT-16922", false},
		{"uppercase in title", "TT-16922: Run resilience tests", "TT-16922", false},
		{"feature branch prefix", "feature/tt-123-add-login", "TT-123", false},
		{"long suffix with hyphens and numbers", "TT-17123-poc-api-to-mcp-v3-part-1", "TT-17123", false},
		{"feat prefix with slashes", "feat/TT-17507/refactor-jira-linter-auth", "TT-17507", false},
		{"no ID", "main", "", true},
		{"empty string", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findIssueID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("findIssueID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("findIssueID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsBranchWithoutTicketID(t *testing.T) {
	tests := []struct {
		name       string
		branchName string
		want       bool
	}{
		// Gromit branches - should skip
		{"releng/master", "releng/master", true},
		{"releng/release-5.13", "releng/release-5.13", true},
		{"releng/release-5.13-go126", "releng/release-5.13-go126", true},
		// Automation branches - should skip
		{"dependabot", "dependabot/npm/lodash", true},
		{"renovate", "renovate/docker-digest", true},
		{"snyk", "snyk-something", true},
		// Regular branches with tickets - should NOT skip
		{"feat with ticket", "feat/TT-12345/new-feature", false},
		{"bugfix with ticket", "bugfix/ABC-999", false},
		// Regular branches without tickets - should NOT skip
		{"feature branch", "feature/my-branch", false},
		{"main", "main", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBranchWithoutTicketID(tt.branchName)
			if got != tt.want {
				t.Errorf("isBranchWithoutTicketID(%q) = %v, want %v", tt.branchName, got, tt.want)
			}
		})
	}
}

func TestValidateBranchAndTitle(t *testing.T) {
	tests := []struct {
		name       string
		branchName string
		prTitle    string
		want       string
		wantErr    bool
	}{
		{
			"lowercase branch with uppercase title",
			"tt-16922-lts-resilience-pipeline",
			"TT-16922: Run resilience tests against LTS versions on schedule",
			"TT-16922",
			false,
		},
		{
			"lowercase branch only",
			"tt-16922-lts-resilience-pipeline",
			"Run resilience tests",
			"TT-16922",
			false,
		},
		{
			"no ID anywhere",
			"feature-branch",
			"Add new feature",
			"",
			true,
		},
		{
			"releng branch with ticket in title",
			"releng/release-5.13-go126",
			"TT-17868: Fix jira linter false positives",
			"TT-17868",
			false,
		},
		{
			"dependabot branch with ticket in title",
			"dependabot/npm/lodash",
			"ABC-123: Update dependency",
			"ABC-123",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{PR: PRConfig{Title: tt.prTitle}}
			got, err := validateBranchAndTitle(config, tt.branchName)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBasicAuthTransport(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	transport := &basicAuthTransport{ReadAuth: "dXNlckBleGFtcGxlLmNvbTpteS10b2tlbg=="}
	client := &http.Client{Transport: transport}

	_, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("failed to GET %s: %v", srv.URL, err)
	}

	want := "Basic dXNlckBleGFtcGxlLmNvbTpteS10b2tlbg=="
	if gotHeader != want {
		t.Errorf("Authorization header = %q, want %q", gotHeader, want)
	}
}

func TestValidateJiraIssue(t *testing.T) {
	issue := &jira.Issue{
		Fields: &jira.IssueFields{
			Status: &jira.Status{Name: "In Dev"},
		},
	}

	if err := validateJiraIssue(issue, "", "TT-123"); err != nil {
		t.Errorf("valid status should not error: %v", err)
	}

	issue.Fields.Status.Name = "Done"
	if err := validateJiraIssue(issue, "", "TT-123"); err == nil {
		t.Error("invalid status should error")
	}
}
