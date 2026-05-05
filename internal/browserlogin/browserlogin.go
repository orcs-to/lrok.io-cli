// Package browserlogin implements the lrok-CLI browser auth flow.
//
// Design notes:
//
//   - PKCE-style: the CLI generates a 256-bit `verifier`, sends only its
//     SHA-256 `challenge` to the web app at issue time, and presents the
//     verifier later when redeeming. The API-key secret never appears in
//     any URL. The only place it touches the wire after Clerk mints it is
//     the JSON body of the redeem POST, over HTTPS.
//
//   - Local listener binds to 127.0.0.1 only on an OS-assigned ephemeral
//     port so external machines can't intercept the callback.
//
//   - `state` is a random hex string echoed back by the web app and
//     verified by the CLI before redeeming. Mismatch aborts the flow.
//
//   - 5-minute total timeout — matches the server-side issue/redeem TTL.
package browserlogin

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"
)

// Result is the secret API key returned at the end of a successful flow.
type Result struct {
	Secret string
}

// Run drives the full flow. webBase is the lrok web origin (e.g.
// "https://lrok.io"). Blocks until success, error, ctx cancellation,
// or the 5-minute timeout, whichever comes first.
func Run(ctx context.Context, webBase string) (*Result, error) {
	verifier, err := randomBase64URL(32) // 256 bits
	if err != nil {
		return nil, fmt.Errorf("verifier: %w", err)
	}
	challenge := sha256Base64URL(verifier)
	state, err := randomBase64URL(32)
	if err != nil {
		return nil, fmt.Errorf("state: %w", err)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "cli"
	}

	// Bind 127.0.0.1:0 — OS picks a free ephemeral port. We never bind
	// 0.0.0.0; only the user's loopback can hit our callback.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("local listen: %w", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	// Channels: one for the parsed callback, one for any HTTP-server
	// error. Both are buffered so handler/server goroutines can exit
	// without blocking when the main goroutine has moved on.
	type cb struct {
		state string
		code  string
	}
	cbCh := make(chan cb, 1)
	srvErr := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "content-type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			State string `json:"state"`
			Code  string `json:"code"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		select {
		case cbCh <- cb{state: body.State, code: body.Code}:
		default:
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	srv := &http.Server{
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		err := srv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErr <- err
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	// Build the user-facing URL.
	q := url.Values{}
	q.Set("port", strconv.Itoa(port))
	q.Set("state", state)
	q.Set("challenge", challenge)
	q.Set("host", hostname)
	authURL := webBase + "/cli-auth?" + q.Encode()

	fmt.Fprintln(os.Stderr, "Opening browser for sign-in…")
	fmt.Fprintln(os.Stderr, "If the browser doesn't open, paste this URL:")
	fmt.Fprintln(os.Stderr, "  ", authURL)
	if err := openBrowser(authURL); err != nil {
		// Don't fail — user can paste manually. Just note it.
		fmt.Fprintln(os.Stderr, "(could not auto-open browser:", err.Error()+")")
	}

	// Wait for callback, error, ctx, or timeout. 5 min matches server TTL.
	timeout := time.NewTimer(5 * time.Minute)
	defer timeout.Stop()

	var got cb
	select {
	case got = <-cbCh:
	case err := <-srvErr:
		return nil, fmt.Errorf("callback server: %w", err)
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timeout.C:
		return nil, errors.New("timed out waiting for browser confirmation")
	}

	if got.state != state {
		return nil, errors.New("state mismatch — aborting (possible cross-session hijack)")
	}
	if got.code == "" {
		return nil, errors.New("empty code from web")
	}

	// Exchange code+verifier for the actual secret.
	redeemURL := webBase + "/api/cli/redeem"
	payload, _ := json.Marshal(map[string]string{
		"code":     got.code,
		"verifier": verifier,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, redeemURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	hc := &http.Client{Timeout: 10 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("redeem: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("redeem failed (%d): %s", resp.StatusCode, string(body))
	}
	var out struct {
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("redeem decode: %w", err)
	}
	if out.Secret == "" {
		return nil, errors.New("redeem returned empty secret")
	}
	return &Result{Secret: out.Secret}, nil
}

func randomBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func sha256Base64URL(input string) string {
	sum := sha256.Sum256([]byte(input))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// hexHelper is exported only so tests can poke at the encoding. Unused
// otherwise — keeping the explicit hex import out of compile would just
// cause an underscore-import dance.
var hexHelper = hex.EncodeToString

func openBrowser(target string) error {
	switch runtime.GOOS {
	case "windows":
		// `cmd /c start` interprets `&` etc., quote the URL.
		return exec.Command("cmd", "/c", "start", "", target).Start()
	case "darwin":
		return exec.Command("open", target).Start()
	default:
		// Linux/BSD/etc.
		return exec.Command("xdg-open", target).Start()
	}
}
