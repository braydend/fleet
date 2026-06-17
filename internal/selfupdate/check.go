package selfupdate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HTTPClient is the minimal HTTP surface used by the package; *http.Client
// satisfies it.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Asset is one downloadable file attached to a release.
type Asset struct {
	Name string
	URL  string
}

// Release is the relevant subset of a GitHub release.
type Release struct {
	Version string // normalized, no leading "v"
	Assets  []Asset
}

// CheckResult is the outcome of a check.
type CheckResult struct {
	Available bool
	Current   string
	Latest    string
	Release   Release
}

// Checker queries GitHub's "latest release" endpoint and compares versions.
type Checker struct {
	Repo    string     // e.g. "braydend/fleet"
	Client  HTTPClient // required
	BaseURL string     // defaults to https://api.github.com
}

// Check returns whether a newer release than current is available. When current
// is a dev/unparseable version it returns immediately with no HTTP request.
func (c Checker) Check(current string) (CheckResult, error) {
	if IsDev(current) {
		return CheckResult{Available: false, Current: current}, nil
	}
	base := c.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	url := fmt.Sprintf("%s/repos/%s/releases/latest", base, c.Repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return CheckResult{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.Client.Do(req)
	if err != nil {
		return CheckResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return CheckResult{}, fmt.Errorf("github returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return CheckResult{}, err
	}
	latest := strings.TrimPrefix(payload.TagName, "v")
	rel := Release{Version: latest}
	for _, a := range payload.Assets {
		rel.Assets = append(rel.Assets, Asset{Name: a.Name, URL: a.URL})
	}
	return CheckResult{
		Available: Newer(current, latest),
		Current:   current,
		Latest:    latest,
		Release:   rel,
	}, nil
}
