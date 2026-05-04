package client

import (
	"bufio"
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
}

func Run(cfg Config) error {
	conn, err := net.Dial("tcp", cfg.TunnelAddr)
	if err != nil {
		return fmt.Errorf("dial %s: %w", cfg.TunnelAddr, err)
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
