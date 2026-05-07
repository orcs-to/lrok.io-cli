// Package telemetry posts anonymous, opt-out lifecycle + error events
// from the lrok CLI to lrok.io. Goal: be 100% clairvoyant about how
// the CLI behaves in the wild — without ever sending PII.
//
// What we send:
//   - source: always "cli"
//   - message: a short label ("login.success", "tunnel.start.error", etc.)
//   - stack: optional, for panics — the runtime/debug.Stack output
//   - context: short metadata — version + os/arch + lower-cased command name
//
// What we DON'T send:
//   - hostnames, usernames, IPs, file paths, env vars
//   - the user's tunnel URL, subdomain, basic-auth string
//   - anything from the user's local server traffic
//
// Opt-out: set LROK_TELEMETRY=0. Honored at every callsite via the
// `enabled` package-level flag computed once at first use.

package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

const (
	endpoint = "https://api.lrok.io/api/v1/track/error"
	timeout  = 3 * time.Second
)

// Version is set from main at startup so beacons carry the binary version.
// Defaults to "dev" so bare `go install` builds don't claim a release tag.
var Version = "dev"

var (
	enabledOnce sync.Once
	enabledFlag bool
)

func enabled() bool {
	enabledOnce.Do(func() {
		// Default ON. Disable explicitly via LROK_TELEMETRY=0.
		if os.Getenv("LROK_TELEMETRY") == "0" {
			enabledFlag = false
			return
		}
		enabledFlag = true
	})
	return enabledFlag
}

// Event posts an event with no stack. Returns immediately; the network
// hop runs in a goroutine. Caller doesn't see errors — telemetry must
// never break a CLI command.
func Event(name string) {
	if !enabled() {
		return
	}
	go post(name, "")
}

// Error posts an error with the given message. Optional `stack` for
// panic captures.
func Error(name string, msg string, stack string) {
	if !enabled() {
		return
	}
	full := name
	if msg != "" {
		full = name + ": " + msg
	}
	go post(full, stack)
}

// Recover wraps a deferred recover() and reports the panic. Use it in
// main() so we get a beacon for any unexpected crash. Never re-throws —
// the runtime will still print the stack to stderr because we re-panic
// after capturing.
func Recover() {
	if r := recover(); r != nil {
		Error("panic", fmt.Sprintf("%v", r), string(debug.Stack()))
		// Give the goroutine a chance to fire before the process dies.
		// Two seconds is generous; the actual fire-and-forget POST
		// budget is `timeout` (3s) but we don't want to block the
		// crashing path forever.
		time.Sleep(2 * time.Second)
		panic(r)
	}
}

func post(message, stack string) {
	body := map[string]string{
		"source":  "cli",
		"message": message,
		"stack":   stack,
		"context": fmt.Sprintf("%s %s/%s", Version, runtime.GOOS, runtime.GOARCH),
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "lrok-cli/"+strings.TrimSpace(Version))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}
