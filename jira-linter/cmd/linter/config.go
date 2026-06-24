package main

type JiraConfig struct {
	BaseURL   string
	UserEmail string
	APIToken  string
}

type PRConfig struct {
	Number int
	Title  string
}

type Config struct {
	Jira JiraConfig
	PR   PRConfig

	GitHubToken      string `envconfig:"GITHUB_TOKEN"`
	GitHubRepository string `envconfig:"GITHUB_REPOSITORY"`
}
