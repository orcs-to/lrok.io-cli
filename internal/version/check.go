// Package version implements `lrok update` — a *check-only* upgrade helper.
//
// We deliberately do NOT self-replace the binary: doing so portably across
// Linux/macOS/Windows (with code signing, anti-virus quarantines, package
// managers, etc.) is risky. Instead we tell the user the right one-liner.
package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// LatestReleaseURL is the GitHub API endpoint for the most recent release.
const LatestReleaseURL = "https://api.github.com/repos/orcs-to/lrok.io-cli/releases/latest"

// release matches the subset of the GitHub releases JSON we care about.
type release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// FetchLatestTag retrieves the latest release tag (e.g. "v0.4.1") from GitHub.
// A 5s timeout caps the request. Returns ("", nil) when the call should be
// treated as "couldn't check" (rate limit, transient error) so callers can
// degrade gracefully instead of crashing.
func FetchLatestTag(ctx context.Context) (string, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", LatestReleaseURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "lrok-cli")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	// Rate-limited or otherwise unhappy: signal "couldn't check" without
	// returning an error so the CLI can soft-fail.
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return "", "", nil
	}
	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("github releases: HTTP %d", resp.StatusCode)
	}

	var r release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", "", fmt.Errorf("decode release: %w", err)
	}
	return r.TagName, r.HTMLURL, nil
}

// Compare returns:
//
//	-1 if a is older than b
//	 0 if a == b (or unparseable — be conservative)
//	+1 if a is newer than b
//
// Inputs may carry a leading "v"; we strip it before comparing each
// dot-separated numeric component. Non-numeric suffixes (e.g. "-rc1") are
// ignored — keep this simple per the spec.
func Compare(a, b string) int {
	pa := splitVersion(a)
	pb := splitVersion(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}

func splitVersion(v string) []int {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	// Drop any pre-release / build suffix after "-" or "+".
	for _, sep := range []string{"-", "+"} {
		if i := strings.Index(v, sep); i >= 0 {
			v = v[:i]
		}
	}
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			// Non-numeric component: stop here, treat rest as zero.
			break
		}
		out = append(out, n)
	}
	return out
}

// InstallHint returns the platform-appropriate install one-liner.
func InstallHint() string {
	switch runtime.GOOS {
	case "windows":
		return `powershell -ExecutionPolicy Bypass -c "irm https://lrok.io/install.ps1 | iex"`
	default:
		return `curl -fsSL https://lrok.io/install.sh | sh`
	}
}

// AssetURL returns the direct-download URL for the current platform's
// pre-built binary at the given tag. Empty string if we can't guess.
func AssetURL(tag string) string {
	if tag == "" {
		return ""
	}
	osName := runtime.GOOS
	arch := runtime.GOARCH
	ext := ""
	if osName == "windows" {
		ext = ".exe"
	}
	// Mirrors the goreleaser naming convention used by the project.
	return fmt.Sprintf(
		"https://github.com/orcs-to/lrok.io-cli/releases/download/%s/lrok-%s-%s%s",
		tag, osName, arch, ext,
	)
}
