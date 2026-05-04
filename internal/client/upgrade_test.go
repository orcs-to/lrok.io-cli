package client

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestHandleStreamPlainHTTP guards the regression: the existing
// request/response replay must still work after the upgrade-aware changes.
func TestHandleStreamPlainHTTP(t *testing.T) {
	// Fake "local server" — accepts a single connection, echoes a 200.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		req, err := http.ReadRequest(bufio.NewReader(c))
		if err != nil {
			return
		}
		_ = req.Body.Close()
		_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"))
	}()

	// Fake "edge -> CLI stream" — net.Pipe simulates the yamux stream the
	// CLI receives via sess.AcceptStream.
	edgeSide, cliSide := net.Pipe()
	defer edgeSide.Close()

	go handleStream(cliSide, ln.Addr().String())

	_ = edgeSide.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := edgeSide.Write([]byte("GET /x HTTP/1.1\r\nHost: example.com\r\n\r\n")); err != nil {
		t.Fatalf("write req: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(edgeSide), nil)
	if err != nil {
		t.Fatalf("read resp: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("body=%q", body)
	}
}

// TestHandleStreamUpgrade verifies the CLI bridges bytes both ways after a
// 101 — without this fix, the local server's frames flow but the public
// client's frames never reach the local server.
func TestHandleStreamUpgrade(t *testing.T) {
	const greeting = "HELLO-FROM-LOCAL"
	const clientFrame = "HELLO-FROM-CLIENT"

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		br := bufio.NewReader(c)
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		_ = req.Body.Close()
		key := req.Header.Get("Sec-WebSocket-Key")
		head := "HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + wsAccept(key) + "\r\n\r\n"
		_, _ = c.Write([]byte(head))
		_, _ = c.Write([]byte(greeting))
		buf := make([]byte, len(clientFrame))
		if _, err := io.ReadFull(br, buf); err != nil {
			return
		}
		_, _ = c.Write(buf)
	}()

	edgeSide, cliSide := net.Pipe()
	defer edgeSide.Close()

	go handleStream(cliSide, ln.Addr().String())

	_ = edgeSide.SetDeadline(time.Now().Add(5 * time.Second))
	handshake := "GET /ws HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := edgeSide.Write([]byte(handshake)); err != nil {
		t.Fatalf("write handshake: %v", err)
	}

	br := bufio.NewReader(edgeSide)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read resp: %v", err)
	}
	if resp.StatusCode != 101 {
		t.Fatalf("status=%d, want 101", resp.StatusCode)
	}
	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") {
		t.Fatalf("missing Upgrade header; got %v", resp.Header)
	}
	got := make([]byte, len(greeting))
	if _, err := io.ReadFull(br, got); err != nil {
		t.Fatalf("read greeting: %v", err)
	}
	if string(got) != greeting {
		t.Fatalf("greeting=%q", got)
	}
	if _, err := edgeSide.Write([]byte(clientFrame)); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	echo := make([]byte, len(clientFrame))
	if _, err := io.ReadFull(br, echo); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(echo) != clientFrame {
		t.Fatalf("echo=%q, want %q", echo, clientFrame)
	}
}

func wsAccept(key string) string {
	h := sha1.New()
	h.Write([]byte(key))
	h.Write([]byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
