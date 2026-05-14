// Package push publishes rendered bootwright workspaces to a git provider
// (GitHub, GitLab, Gitea).
package push

import (
	"fmt"
	"net/url"
	"strings"
)

type Options struct {
	Provider      string // github | gitlab | gitea
	BaseURL       string
	OwnerType     string // org | user (ignored by GitLab)
	Token         string
	User          string
	Branch        string
	CommitMessage string
	Visibility    string // private | public | internal
	CreateMissing bool
	Flatten       bool
	Force         bool
	DryRun        bool
}

type ParsedBaseURL struct {
	Scheme string
	Host   string
	Owner  string
}

func ParseBaseURL(raw string) (ParsedBaseURL, error) {
	if raw == "" {
		return ParsedBaseURL{}, fmt.Errorf("--base-url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ParsedBaseURL{}, fmt.Errorf("parse --base-url %q: %w", raw, err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return ParsedBaseURL{}, fmt.Errorf("--base-url %q: scheme must be https (or http for self-hosted)", raw)
	}
	if u.Host == "" {
		return ParsedBaseURL{}, fmt.Errorf("--base-url %q: host is empty", raw)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return ParsedBaseURL{}, fmt.Errorf("--base-url %q: query and fragment are not supported", raw)
	}
	owner := strings.Trim(u.Path, "/")
	if owner == "" {
		return ParsedBaseURL{}, fmt.Errorf("--base-url %q: owner/group path is required (e.g. https://%s/myorg)", raw, u.Host)
	}
	return ParsedBaseURL{Scheme: u.Scheme, Host: u.Host, Owner: owner}, nil
}

func RemoteRepoName(rendered string, flatten bool) string {
	if !flatten {
		return rendered
	}
	return strings.ReplaceAll(rendered, "/", "-")
}

func (p ParsedBaseURL) CloneURL(repo string) string {
	return fmt.Sprintf("%s://%s/%s/%s.git", p.Scheme, p.Host, p.Owner, repo)
}

// InjectCredentials bakes basic-auth creds into the URL's userinfo;
// callers should prefer git credential helpers when possible because
// tokens in URLs are visible in `ps` and crash dumps.
func InjectCredentials(rawURL, user, token string) (string, error) {
	if token == "" {
		return rawURL, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("inject credentials: %w", err)
	}
	if user == "" {
		user = "x-access-token"
	}
	u.User = url.UserPassword(user, token)
	return u.String(), nil
}
