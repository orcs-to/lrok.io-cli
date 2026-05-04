package client

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/orcs-to/lrok.io-cli/internal/protocol"
)

// TestRunSerializesBasicAuth verifies that Config.BasicAuth is forwarded into
// the RegisterRequest JSON sent to the edge. We stand up a fake edge on
// net.Pipe, run yamux server-side, and read the first control-stream payload.
func TestRunSerializesBasicAuth(t *testing.T) {
	srvConn, cliConn := net.Pipe()

	// "Edge" side: accept yamux + read register.
	got := make(chan protocol.RegisterRequest, 1)
	errCh := make(chan error, 1)
	go func() {
		defer srvConn.Close()
		sess, err := yamux.Server(srvConn, yamux.DefaultConfig())
		if err != nil {
			errCh <- err
			return
		}
		stream, err := sess.AcceptStream()
		if err != nil {
			errCh <- err
			return
		}
		var req protocol.RegisterRequest
		if err := json.NewDecoder(stream).Decode(&req); err != nil {
			errCh <- err
			return
		}
		got <- req
		// Reply with an error so client.Run returns quickly without blocking
		// on AcceptStream forever.
		_ = json.NewEncoder(stream).Encode(protocol.RegisterResponse{
			OK:    false,
			Error: "test stub",
		})
		_ = sess.Close()
	}()

	// Run the client side via a thin wrapper that swaps in our pre-dialled
	// connection. We mirror what Run() does without the dialTunnel path.
	done := make(chan error, 1)
	go func() {
		done <- runWithConn(cliConn, Config{
			BasicAuth: "alice:s3cret",
			Hint:      "myhint",
			AuthToken: "tok",
		})
	}()

	select {
	case req := <-got:
		if req.BasicAuth != "alice:s3cret" {
			t.Errorf("BasicAuth=%q, want alice:s3cret", req.BasicAuth)
		}
		if req.Hint != "myhint" {
			t.Errorf("Hint=%q, want myhint", req.Hint)
		}
		if req.AuthToken != "tok" {
			t.Errorf("AuthToken=%q, want tok", req.AuthToken)
		}
	case err := <-errCh:
		t.Fatalf("server side: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for register payload")
	}

	// Drain the client goroutine.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}
