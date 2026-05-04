package client

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/hashicorp/yamux"
	"github.com/orcs-to/lrok.io-cli/internal/protocol"
)

type Config struct {
	TunnelAddr  string
	LocalTarget string
	Hint        string
	AuthToken   string
	// Mode selects the tunnel kind. "" or protocol.ModeHTTP is HTTP (default
	// behavior, including request parsing). protocol.ModeTCP is raw TCP —
	// each accepted yamux stream is dialed straight to LocalTarget with no
	// HTTP-level handling.
	Mode string
	// BasicAuth, if non-empty, asks the edge to gate the public URL with HTTP
	// Basic Auth. Format is the literal "user:pass" string. HTTP mode only.
	BasicAuth string
	// Insecure disables TLS for the control-plane connection. Used for local
	// dev against an edge running with --tls-cert-source=none. Honors both
	// the --insecure flag and the LROK_INSECURE=1 env var.
	Insecure bool
}

func Run(cfg Config) error {
	conn, err := dialTunnel(cfg)
	if err != nil {
		return err
	}
	return runWithConn(conn, cfg)
}

// runWithConn is the post-dial half of Run, factored out so tests can drive
// it with an in-memory net.Conn (e.g. one half of a net.Pipe).
func runWithConn(conn net.Conn, cfg Config) error {
	sess, err := yamux.Client(conn, yamux.DefaultConfig())
	if err != nil {
		return fmt.Errorf("yamux client: %w", err)
	}
	defer sess.Close()

	ctrl, err := sess.OpenStream()
	if err != nil {
		return fmt.Errorf("open control stream: %w", err)
	}

	if err := json.NewEncoder(ctrl).Encode(protocol.RegisterRequest{
		Version:   protocol.Version,
		AuthToken: cfg.AuthToken,
		Hint:      cfg.Hint,
		Mode:      cfg.Mode,
		BasicAuth: cfg.BasicAuth,
	}); err != nil {
		return fmt.Errorf("send register: %w", err)
	}

	var resp protocol.RegisterResponse
	if err := json.NewDecoder(ctrl).Decode(&resp); err != nil {
		return fmt.Errorf("read register response: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("register failed: %s", resp.Error)
	}

	tcp := cfg.Mode == protocol.ModeTCP
	if tcp {
		fmt.Fprintf(os.Stdout, "\n  Forwarding tcp://%s -> tcp://%s\n\n", resp.PublicAddr, cfg.LocalTarget)
	} else {
		fmt.Fprintf(os.Stdout, "\n  Forwarding %s -> http://%s\n\n", resp.PublicURL, cfg.LocalTarget)
	}

	for {
		stream, err := sess.AcceptStream()
		if err != nil {
			return fmt.Errorf("accept stream: %w", err)
		}
		if tcp {
			go handleTCPStream(stream, cfg.LocalTarget)
		} else {
			go handleStream(stream, cfg.LocalTarget)
		}
	}
}

// dialTunnel opens a connection to the edge's control plane, defaulting to
// TLS with the system root store. LROK_INSECURE=1 (or cfg.Insecure) drops
// back to plain TCP for local dev.
func dialTunnel(cfg Config) (net.Conn, error) {
	insecure := cfg.Insecure || os.Getenv("LROK_INSECURE") == "1"
	if insecure {
		conn, err := net.Dial("tcp", cfg.TunnelAddr)
		if err != nil {
			return nil, fmt.Errorf("dial %s: %w", cfg.TunnelAddr, err)
		}
		return conn, nil
	}

	host, _, err := net.SplitHostPort(cfg.TunnelAddr)
	if err != nil {
		// No port specified; use the whole string as the SNI host.
		host = cfg.TunnelAddr
	}
	conn, err := tls.Dial("tcp", cfg.TunnelAddr, &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	})
	if err != nil {
		return nil, fmt.Errorf("tls dial %s: %w (set LROK_INSECURE=1 for plain TCP against a local edge)", cfg.TunnelAddr, err)
	}
	return conn, nil
}

// handleTCPStream forwards a raw TCP yamux stream straight to target with no
// HTTP-level parsing. Used by `lrok tcp <port>`.
func handleTCPStream(stream net.Conn, target string) {
	defer stream.Close()

	local, err := net.Dial("tcp", target)
	if err != nil {
		// No HTTP frame to write; just log to stderr and drop the conn.
		fmt.Fprintf(os.Stderr, "lrok cli: cannot reach local target %s: %v\n", target, err)
		return
	}
	defer local.Close()

	type writeCloser interface{ CloseWrite() error }
	halfCloseWrite := func(c net.Conn) {
		if wc, ok := c.(writeCloser); ok {
			_ = wc.CloseWrite()
			return
		}
		_ = c.Close()
	}

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(stream, local)
		halfCloseWrite(stream)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(local, stream)
		halfCloseWrite(local)
		done <- struct{}{}
	}()
	<-done
	<-done
}

func handleStream(stream net.Conn, target string) {
	defer stream.Close()

	// Buffered reader so we can both http.ReadRequest AND, for upgrades,
	// continue copying any bytes the buffered reader pulled past the head.
	br := bufio.NewReader(stream)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}

	local, err := net.Dial("tcp", target)
	if err != nil {
		writeError(stream, http.StatusBadGateway, "lrok cli: cannot reach local target "+target)
		return
	}
	defer local.Close()

	if err := req.Write(local); err != nil {
		return
	}

	if isUpgradeRequest(req) {
		// HTTP/1.1 protocol upgrade (e.g. WebSocket). After the local
		// server writes its 101 response, it expects bidirectional
		// traffic on the same connection. Bridge stream <-> local both
		// ways so frames flow in each direction.
		done := make(chan struct{}, 2)
		go func() {
			_, _ = io.Copy(stream, local)
			done <- struct{}{}
		}()
		go func() {
			_, _ = io.Copy(local, br)
			done <- struct{}{}
		}()
		<-done
		return
	}

	_, _ = io.Copy(stream, local)
}

// isUpgradeRequest reports whether the request opted into an HTTP/1.1
// protocol upgrade (e.g. WebSocket handshake). Per RFC 7230 the
// Connection header must contain the "upgrade" token and Upgrade names
// the target protocol.
func isUpgradeRequest(r *http.Request) bool {
	if !headerContainsToken(r.Header, "Connection", "upgrade") {
		return false
	}
	return r.Header.Get("Upgrade") != ""
}

func headerContainsToken(h http.Header, name, token string) bool {
	token = strings.ToLower(token)
	for _, v := range h.Values(name) {
		for _, part := range strings.Split(v, ",") {
			if strings.ToLower(strings.TrimSpace(part)) == token {
				return true
			}
		}
	}
	return false
}

func writeError(w io.Writer, code int, msg string) {
	body := msg + "\n"
	resp := &http.Response{
		StatusCode:    code,
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
	}
	resp.Header.Set("Content-Type", "text/plain; charset=utf-8")
	_ = resp.Write(w)
}
