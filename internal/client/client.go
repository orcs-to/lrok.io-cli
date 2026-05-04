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

	fmt.Fprintf(os.Stdout, "\n  Forwarding %s -> http://%s\n\n", resp.PublicURL, cfg.LocalTarget)

	for {
		stream, err := sess.AcceptStream()
		if err != nil {
			return fmt.Errorf("accept stream: %w", err)
		}
		go handleStream(stream, cfg.LocalTarget)
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

func handleStream(stream net.Conn, target string) {
	defer stream.Close()

	req, err := http.ReadRequest(bufio.NewReader(stream))
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

	_, _ = io.Copy(stream, local)
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
