package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/andygrunwald/go-jira"
	"github.com/google/go-github/v74/github"
	"github.com/kelseyhightower/envconfig"
	"golang.org/x/oauth2"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// defaultAcceptedStatuses defines the default JIRA statuses that are valid for active development
var defaultAcceptedStatuses = map[string]bool{
	"in dev":           true,
	"in code review":   true,
	"ready for dev":    true,
	"dod check":        true,
	"merge":            true,
	"in design review": true,
}

// loadConfig loads and validates all environment configuration
func loadConfig() (*Config, error) {
	var config Config
	if err := envconfig.Process("JL", &config); err != nil {
		return nil, fmt.Errorf("failed to load environment configuration: %w", err)
	}

	if strings.TrimSpace(config.Jira.BaseURL) == "" {
		return nil, fmt.Errorf("failed to validate Jira base URL, cannot be empty or whitespace")
	}

	if strings.TrimSpace(config.Jira.UserEmail) == "" {
		return nil, fmt.Errorf("Jira user email is required for API authentication")
	}

	if strings.TrimSpace(config.Jira.APIToken) == "" {
		return nil, fmt.Errorf("Jira API token is required for API authentication")
	}

	if config.PR.Number <= 0 {
		return nil, fmt.Errorf("PR number must be a positive integer")
	}

	return &config, nil
}

func main() {
	if err := run(); err != nil {
		logError(err.Error())
		os.Exit(1)
	}
}

func run() error {
	log("Starting Jira to PR validation...")

	branchName := flag.String("branch", "", "The branch name")
	customStatuses := flag.String("statuses", "", "Comma-separated list of accepted JIRA statuses (empty to skip validation)")

	flag.Parse()

	if *branchName == "" {
		return fmt.Errorf("branch name is required")
	}

	config, err := loadConfig()
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	issueID, err := validateBranchAndTitle(config, *branchName)
	if err != nil {
		return fmt.Errorf("failed to validate branch and PR title rules: %w", err)
	}

	issue, err := getJiraIssue(config, issueID)
	if err != nil {
		return fmt.Errorf("failed to get Jira issue: %w", err)
	}

	err = updatePRDescription(config, issue)
	if err != nil {
		return fmt.Errorf("failed to update PR: %w", err)
	}

	err = validateJiraIssue(issue, *customStatuses, issueID)
	if err != nil {
		return fmt.Errorf("failed to validate Jira issue: %w", err)
	}

	log("Validation completed successfully")
	return nil
}

func validateBranchAndTitle(config *Config, branchName string) (string, error) {
	issueIDInBranch, branchErr := findIssueID(branchName)
	issueIDInTitle, titleErr := findIssueID(config.PR.Title)

	switch {
	case branchErr == nil && titleErr == nil:
		// Found in both; use branch, warn if they differ
		if issueIDInTitle != issueIDInBranch {
			logWarn("PR title contains '%s' but branch has '%s' - using branch value", issueIDInTitle, issueIDInBranch)
		}
		return issueIDInBranch, nil

	case branchErr == nil:
		// Found only in branch
		return issueIDInBranch, nil

	case titleErr == nil:
		// Found only in PR title
		return issueIDInTitle, nil

	default:
		// Found in neither
		return "", fmt.Errorf("neither branch name '%s' nor PR title '%s' contains a valid Jira ticket ID (e.g., ABC-123)", branchName, config.PR.Title)
	}
}

func getJiraClient(config *Config) (*jira.Client, error) {
	httpClient := &http.Client{
		Transport: &basicAuthTransport{
			Email:    config.Jira.UserEmail,
			APIToken: config.Jira.APIToken,
		},
	}

	client, err := jira.NewClient(httpClient, config.Jira.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create Jira client: %w", err)
	}

	return client, nil
}

type basicAuthTransport struct {
	Email    string
	APIToken string
}

func (t *basicAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	creds := base64.StdEncoding.EncodeToString([]byte(t.Email + ":" + t.APIToken))
	req.Header.Set("Authorization", "Basic "+creds)
	return http.DefaultTransport.RoundTrip(req)
}

var issueIDRegex = regexp.MustCompile(`(?i)^.*?([A-Z]{1,10}-[0-9]{1,10}).*?$`)

func findIssueID(input string) (string, error) {
	match := issueIDRegex.FindStringSubmatch(input)

	if len(match) < 2 {
		return "", fmt.Errorf("no valid Jira ticket ID found")
	}

	return strings.ToUpper(match[1]), nil
}

func createGitHubClient(ctx context.Context, token string) (*github.Client, error) {
	tc := oauth2.NewClient(ctx, oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	))

	return github.NewClient(tc), nil
}

type prCtx struct {
	Owner  string
	Repo   string
	Number int
}

func getPrCtx(config *Config) (*prCtx, error) {
	parts := strings.Split(config.GitHubRepository, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid GITHUB_REPOSITORY format: %s", config.GitHubRepository)
	}

	return &prCtx{
		Owner:  parts[0],
		Repo:   parts[1],
		Number: config.PR.Number,
	}, nil
}

const (
	jiraSectionMarkerStart = "<!---TykTechnologies/jira-linter starts here-->"
	jiraSectionMarkerEnd   = "<!---TykTechnologies/jira-linter ends here-->"
)

func updatePRDescriptionBody(ctx context.Context, client *github.Client, prCtx *prCtx, jiraInfo string) error {
	pr, _, err := client.PullRequests.Get(ctx, prCtx.Owner, prCtx.Repo, prCtx.Number)
	if err != nil {
		return fmt.Errorf("failed to fetch PR #%d: %w", prCtx.Number, err)
	}

	body := pr.GetBody()
	newSection := fmt.Sprintf("\n%s\n%s\n%s\n", jiraSectionMarkerStart, jiraInfo, jiraSectionMarkerEnd)

	startIdx := strings.Index(body, jiraSectionMarkerStart)
	endIdx := strings.Index(body, jiraSectionMarkerEnd)

	var newBody string
	if startIdx != -1 && endIdx != -1 {
		newBody = body[:startIdx] + newSection + body[endIdx+len(jiraSectionMarkerEnd):]
	} else {
		newBody = body + newSection
	}

	updateRequest := &github.PullRequest{
		Body: &newBody,
	}

	_, _, err = client.PullRequests.Edit(ctx, prCtx.Owner, prCtx.Repo, prCtx.Number, updateRequest)
	if err != nil {
		return fmt.Errorf("failed to update PR description: %w", err)
	}

	log("Successfully updated PR with Jira info")
	return nil
}

// getAcceptedStatuses returns the status map to use for validation
// Returns nil if status validation should be skipped
func getAcceptedStatuses(customStatuses string) map[string]bool {
	if customStatuses != "" {
		return parseCustomStatuses(customStatuses)
	}

	return defaultAcceptedStatuses
}

// parseCustomStatuses parses comma-separated status string into a map
func parseCustomStatuses(statusString string) map[string]bool {
	if strings.TrimSpace(statusString) == "" {
		return nil
	}

	statusMap := make(map[string]bool)
	statuses := strings.Split(statusString, ",")

	for _, status := range statuses {
		trimmed := strings.TrimSpace(status)
		if trimmed != "" {
			statusMap[strings.ToLower(trimmed)] = true
		}
	}

	return statusMap
}

// isValidStatus checks if the given status is in the accepted list (case-insensitive)
func isValidStatus(status string, acceptedStatuses map[string]bool) bool {
	return acceptedStatuses[strings.TrimSpace(strings.ToLower(status))]
}

// getAcceptedStatusList returns a formatted string of accepted statuses for error messages
func getAcceptedStatusList(acceptedStatuses map[string]bool) string {
	var statuses []string

	titleCase := cases.Title(language.English)
	for status := range acceptedStatuses {
		statuses = append(statuses, titleCase.String(status))
	}

	return strings.Join(statuses, ", ")
}

func getJiraIssue(config *Config, issueID string) (*jira.Issue, error) {
	jiraClient, err := getJiraClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Jira client: %w", err)
	}

	issue, resp, err := jiraClient.Issue.Get(issueID, nil)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return nil, fmt.Errorf("Jira issue %s not found (HTTP 404). The issue may exist but the API token may lack permission to access it. Verify that the token owner has access to the project and that JL_JIRA_BASEURL (%s) is correct", issueID, config.Jira.BaseURL)
		}
		return nil, fmt.Errorf("failed to fetch Jira issue %s: %w", issueID, err)
	}

	return issue, nil
}

func updatePRDescription(config *Config, issue *jira.Issue) error {
	issueLink := fmt.Sprintf("%s/browse/%s", strings.TrimSuffix(config.Jira.BaseURL, "/"), issue.Key)
	jiraInfo := fmt.Sprintf(`
### Ticket Details

<details>
<summary>
<a href="%s" title="%s" target="_blank">%s</a>
</summary>

|         |    |
|---------|----|
| Status  | %s |
| Summary | %s |

Generated at: %s

</details>
`,
		issueLink, issue.Key, issue.Key,
		issue.Fields.Status.Name,
		issue.Fields.Summary,
		time.Now().UTC().Format("2006-01-02 15:04:05"),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	githubClient, err := createGitHubClient(ctx, config.GitHubToken)
	if err != nil {
		return fmt.Errorf("failed to connect to GitHub: %w", err)
	}

	prCtx, err := getPrCtx(config)
	if err != nil {
		return fmt.Errorf("failed to get PR context: %w", err)
	}

	err = updatePRDescriptionBody(ctx, githubClient, prCtx, jiraInfo)
	if err != nil {
		return fmt.Errorf("failed to update PR description: %w", err)
	}

	return nil
}

func validateJiraIssue(issue *jira.Issue, customStatuses, issueID string) error {
	acceptedStatuses := getAcceptedStatuses(customStatuses)
	if acceptedStatuses != nil && !isValidStatus(issue.Fields.Status.Name, acceptedStatuses) {
		return fmt.Errorf(
			"jira ticket %s has status '%s' but must be one of: %s",
			issueID, issue.Fields.Status.Name, getAcceptedStatusList(acceptedStatuses),
		)
	}

	return nil
}

func log(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, "[INFO] "+msg+"\n", args...)
}

func logWarn(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, "[WARN] "+msg+"\n", args...)
}

func logError(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, "[ERROR] "+msg+"\n", args...)
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
}
