// Package ipc implements the Unix socket server and client.
// Tests cover FR-03 (protocol correctness) and SCN-20 (daemon not running).
package ipc_test

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dinav2/vida/internal/ipc"
)

func tempSocket(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "vida-test.sock")
}

// FR-03a: messages are single-line JSON terminated by \n
func TestServer_MessageFormat(t *testing.T) {
	sock := tempSocket(t)
	srv, err := ipc.Listen(sock)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer srv.Close()

	received := make(chan []byte, 1)
	srv.Handle(func(msg ipc.Message, reply ipc.ReplyFunc) {
		received <- msg.Raw
	})
	go srv.Serve(context.Background())

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	// Send a message with trailing newline
	_, _ = conn.Write([]byte(`{"type":"ping"}` + "\n"))

	select {
	case raw := <-received:
		if raw[len(raw)-1] == '\n' {
			t.Error("received message includes trailing newline; should be stripped by decoder")
		}
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Errorf("received message is not valid JSON: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for message")
	}
}

// Ping/pong roundtrip
func TestServer_Ping(t *testing.T) {
	sock := tempSocket(t)
	srv, err := ipc.Listen(sock)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer srv.Close()
	go srv.Serve(context.Background())

	c, err := ipc.Connect(sock)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	resp, err := c.Send(ipc.Message{Type: "ping"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Type != "pong" {
		t.Errorf("response type = %q, want %q", resp.Type, "pong")
	}
}

// FR-03e: unknown message type returns error, server does not crash
func TestServer_UnknownType(t *testing.T) {
	sock := tempSocket(t)
	srv, err := ipc.Listen(sock)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer srv.Close()
	go srv.Serve(context.Background())

	c, err := ipc.Connect(sock)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	resp, err := c.Send(ipc.Message{Type: "nonexistent_type_xyz"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.Type != "error" {
		t.Errorf("response type = %q, want %q", resp.Type, "error")
	}
	if resp.Message == "" {
		t.Error("error response missing message field")
	}
}

// TR-02d: show/hide are broadcast to all connected clients
func TestServer_Broadcast(t *testing.T) {
	sock := tempSocket(t)
	srv, err := ipc.Listen(sock)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer srv.Close()
	go srv.Serve(context.Background())

	// Connect two persistent clients
	c1, err := ipc.ConnectPersistent(sock)
	if err != nil {
		t.Fatalf("Connect c1: %v", err)
	}
	defer c1.Close()

	c2, err := ipc.ConnectPersistent(sock)
	if err != nil {
		t.Fatalf("Connect c2: %v", err)
	}
	defer c2.Close()

	// Register both as UI subscribers
	_ = c1.SendNoReply(ipc.Message{Type: "subscribe"})
	_ = c2.SendNoReply(ipc.Message{Type: "subscribe"})
	time.Sleep(50 * time.Millisecond) // allow subscription to register

	// Send show from a third (CLI) connection
	cli, err := ipc.Connect(sock)
	if err != nil {
		t.Fatalf("Connect cli: %v", err)
	}
	_, _ = cli.Send(ipc.Message{Type: "show"})
	cli.Close()

	// Both subscribers should receive "show"
	for i, c := range []*ipc.PersistentConn{c1, c2} {
		msg, err := c.Recv(500 * time.Millisecond)
		if err != nil {
			t.Errorf("client %d: Recv timeout: %v", i+1, err)
			continue
		}
		if msg.Type != "show" {
			t.Errorf("client %d: got %q, want %q", i+1, msg.Type, "show")
		}
	}
}

// TR-02e: socket file has 0600 permissions
func TestSocket_Permissions(t *testing.T) {
	sock := tempSocket(t)
	srv, err := ipc.Listen(sock)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer srv.Close()

	info, err := os.Stat(sock)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("socket perms = %o, want 0600", info.Mode().Perm())
	}
}

// FR-01b: stale socket file from dead process is removed on Listen
func TestServer_StaleSocket(t *testing.T) {
	sock := tempSocket(t)

	// Create a stale socket file (no process listening)
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("create stale socket: %v", err)
	}
	l.Close()
	// sock file exists but no listener

	// Listen should succeed by removing stale file
	srv, err := ipc.Listen(sock)
	if err != nil {
		t.Fatalf("Listen with stale socket: %v", err)
	}
	defer srv.Close()
}

// SCN-20: client returns clear error when daemon is not running
func TestConnect_DaemonNotRunning(t *testing.T) {
	sock := tempSocket(t)
	// Do NOT start server

	_, err := ipc.Connect(sock)
	if err == nil {
		t.Error("Connect to missing socket: expected error, got nil")
	}
	if !ipc.IsDaemonNotRunning(err) {
		t.Errorf("expected IsDaemonNotRunning(err) = true, got false for error: %v", err)
	}
}

// AC-R4: client disconnect does not crash server
func TestServer_ClientDisconnect(t *testing.T) {
	sock := tempSocket(t)
	srv, err := ipc.Listen(sock)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer srv.Close()
	go srv.Serve(context.Background())

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	// Abrupt close without sending any data
	conn.Close()

	// Server should still respond to subsequent connections
	time.Sleep(50 * time.Millisecond)
	c, err := ipc.Connect(sock)
	if err != nil {
		t.Fatalf("subsequent Connect failed: %v", err)
	}
	resp, _ := c.Send(ipc.Message{Type: "ping"})
	if resp.Type != "pong" {
		t.Errorf("post-disconnect ping: got %q, want pong", resp.Type)
	}
	c.Close()
}
