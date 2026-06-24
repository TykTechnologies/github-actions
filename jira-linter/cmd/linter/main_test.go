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

	transport := &basicAuthTransport{Email: "user@example.com", APIToken: "my-token"}
	client := &http.Client{Transport: transport}

	_, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("failed to GET %s: %v", srv.URL, err)
	}

	// base64("user@example.com:my-token") = "dXNlckBleGFtcGxlLmNvbTpteS10b2tlbg=="
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
