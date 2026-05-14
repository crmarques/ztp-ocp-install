package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Provider interface {
	Name() string
	EnsureRepo(ctx context.Context, owner, repo, visibility string, createMissing bool) (cloneURL string, err error)
}

type ProviderConfig struct {
	Base      ParsedBaseURL
	Token     string
	OwnerType string // org | user
	HTTP      *http.Client
}

func NewProvider(name string, cfg ProviderConfig) (Provider, error) {
	if cfg.HTTP == nil {
		cfg.HTTP = http.DefaultClient
	}
	if cfg.OwnerType == "" {
		cfg.OwnerType = "org"
	}
	switch strings.ToLower(name) {
	case "github":
		return &githubProvider{cfg: cfg}, nil
	case "gitlab":
		return &gitlabProvider{cfg: cfg}, nil
	case "gitea":
		return &giteaProvider{cfg: cfg}, nil
	case "":
		return nil, fmt.Errorf("--provider is required (github|gitlab|gitea)")
	default:
		return nil, fmt.Errorf("--provider %q: must be one of github|gitlab|gitea", name)
	}
}

func doJSON(ctx context.Context, client *http.Client, method, url string, headers map[string]string, body, out any) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("marshal %s %s: %w", method, url, err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("new request %s %s: %w", method, url, err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%s %s: %w", method, url, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read %s %s: %w", method, url, err)
	}
	if out != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return resp.StatusCode, respBody, fmt.Errorf("decode %s %s: %w", method, url, err)
		}
	}
	return resp.StatusCode, respBody, nil
}

func errMissingToken(name string) error {
	return fmt.Errorf("%s: a token is required to create repositories; set --token or BOOTWRIGHT_PUSH_TOKEN (or the provider env) or pass --create-missing=false", name)
}

func httpError(provider, action string, status int, body []byte) error {
	snippet := string(body)
	if len(snippet) > 512 {
		snippet = snippet[:512] + "…"
	}
	return fmt.Errorf("%s: %s returned HTTP %d: %s", provider, action, status, strings.TrimSpace(snippet))
}
