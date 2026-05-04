package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRedactToken(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "(none)"},
		{"   ", "(none)"},
		{"ak_abcdefghijklmnopqrstuvwxyz_1234", "ak_...1234"},
		{"short", "***hort"}, // last 4 with leading ***
		{"abcd", "***abcd"},
		{"abc", "***abc"},
		// 8-char input takes the short branch.
		{"12345678", "***5678"},
		// 9-char triggers the prefix...last4 branch.
		{"123456789", "123...6789"},
	}
	for _, c := range cases {
		got := redactToken(c.in)
		if got != c.want {
			t.Errorf("redactToken(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestStatusSmoke spins up an httptest server pretending to be api.lrok.io,
// builds the CLI, and runs `lrok status` against it with a temporary HOME
// containing a saved token. We assert on the formatted output.
func TestStatusSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping smoke test in -short mode")
	}

	// Fake API: returns a plan and one tunnel.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/me/plan", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
			t.Errorf("plan: missing bearer token, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]int{
			"tunnelQuota":      4,
			"tunnelUsed":       1,
			"reservationQuota": -1,
			"reservationUsed":  2,
		})
	})
	mux.HandleFunc("/api/v1/me/tunnels", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"subdomain": "happy-otter",
			"publicUrl": "https://happy-otter.lrok.io",
			"protocol":  "http",
			"startedAt": time.Now().Add(-90 * time.Second).UTC().Format(time.RFC3339),
		}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Build the binary into a temp dir.
	bin := filepath.Join(t.TempDir(), "lrok-test")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, "./cmd/lrok")
	build.Dir = repoRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	// Fake HOME with a saved token so requireToken / status loads it.
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".lrok")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config"),
		[]byte(`{"token":"ak_test_token_xyz1234"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "status")
	env := os.Environ()
	if runtime.GOOS == "windows" {
		env = append(env, "USERPROFILE="+home)
	}
	env = append(env, "HOME="+home, "LROK_API="+srv.URL)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("lrok status failed: %v\nstderr:\n%s\nstdout:\n%s", err, stderr.String(), stdout.String())
	}

	out := stdout.String()
	checks := []string{
		"Signed in as",
		"ak_...1234",
		"1 used / 4",
		"unlimited",
		"happy-otter",
		"https://happy-otter.lrok.io",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	// Walk up from the test file's directory until we find go.mod.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("could not locate go.mod from %s", wd)
	return ""
}
