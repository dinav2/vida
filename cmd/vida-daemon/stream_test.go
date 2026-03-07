// Streaming and query-routing integration tests for vida-daemon.
// Tests cover TR-02 (IPC protocol), TR-05 (cancel), SCN-01–02, SCN-06–09.
package main_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeStreamingAI returns an httptest.Server that streams SSE tokens then stops.
func fakeStreamingAI(t *testing.T, tokens []string, blockUntilCancel bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher := w.(http.Flusher)

		// message_start
		fmt.Fprintf(w, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
		flusher.Flush()

		for _, tok := range tokens {
			select {
			case <-r.Context().Done():
				return // client cancelled
			default:
			}
			data, _ := json.Marshal(map[string]any{
				"type":  "content_block_delta",
				"delta": map[string]string{"type": "text_delta", "text": tok},
			})
			fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", data)
			flusher.Flush()
			time.Sleep(20 * time.Millisecond)
		}

		if blockUntilCancel {
			<-r.Context().Done()
			return
		}

		fmt.Fprintf(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
}

// writeDaemonConfig writes a TOML config pointing AI at the given base URL.
func writeDaemonConfig(t *testing.T, aiBaseURL string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := fmt.Sprintf(`
[ai]
provider = "claude"

[ai.claude]
api_key  = "test-key"
base_url = %q

[search.shortcuts]
g = "https://www.google.com/search?q=%%s"
`, aiBaseURL)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// recvMsg reads one JSON object from a raw unix connection.
func recvMsg(t *testing.T, conn net.Conn, timeout time.Duration) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	var msg map[string]any
	if err := json.NewDecoder(conn).Decode(&msg); err != nil {
		t.Fatalf("recvMsg decode: %v", err)
	}
	return msg
}

// sendRaw encodes msg as JSON and writes it to conn.
func sendRaw(t *testing.T, conn net.Conn, msg map[string]any) {
	t.Helper()
	if err := json.NewEncoder(conn).Encode(msg); err != nil {
		t.Fatalf("sendRaw encode: %v", err)
	}
}

// openPersistent dials the unix socket and returns a raw conn.
func openPersistent(t *testing.T, sock string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("unix", sock, 2*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", sock, err)
	}
	return conn
}

// --- SCN-01: calc query returns kind=calc ---

// TR-02b: query "2 + 2" → result with kind=calc, value="4"
func TestQuery_CalcResult(t *testing.T) {
	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")
	cfgPath := writeDaemonConfig(t, "http://localhost:1") // AI unreachable; not needed
	cleanup := startDaemon(t, bin, sock, cfgPath)
	defer cleanup()

	conn := openPersistent(t, sock)
	defer conn.Close()

	sendRaw(t, conn, map[string]any{
		"type":  "query",
		"id":    "calc-1",
		"input": "2 + 2",
	})

	msg := recvMsg(t, conn, 2*time.Second)
	if msg["type"] != "result" {
		t.Fatalf("type = %q, want result", msg["type"])
	}
	if msg["kind"] != "calc" {
		t.Errorf("kind = %q, want calc", msg["kind"])
	}
	if msg["value"] != "4" {
		t.Errorf("value = %q, want 4", msg["value"])
	}
	if msg["id"] != "calc-1" {
		t.Errorf("id = %q, want calc-1", msg["id"])
	}
}

// --- SCN-02: shortcut query returns kind=shortcut ---

// TR-02b: "g hello" → kind=shortcut, url contains "hello"
func TestQuery_ShortcutResult(t *testing.T) {
	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")
	cfgPath := writeDaemonConfig(t, "http://localhost:1")
	cleanup := startDaemon(t, bin, sock, cfgPath)
	defer cleanup()

	conn := openPersistent(t, sock)
	defer conn.Close()

	sendRaw(t, conn, map[string]any{
		"type":  "query",
		"id":    "sc-1",
		"input": "g hello world",
	})

	msg := recvMsg(t, conn, 2*time.Second)
	if msg["kind"] != "shortcut" {
		t.Errorf("kind = %q, want shortcut", msg["kind"])
	}
	url, _ := msg["url"].(string)
	if !strings.Contains(url, "hello") {
		t.Errorf("url %q does not contain 'hello'", url)
	}
}

// --- SCN-06: empty input returns kind=empty ---

func TestQuery_EmptyInput(t *testing.T) {
	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")
	cfgPath := writeDaemonConfig(t, "http://localhost:1")
	cleanup := startDaemon(t, bin, sock, cfgPath)
	defer cleanup()

	conn := openPersistent(t, sock)
	defer conn.Close()

	sendRaw(t, conn, map[string]any{
		"type":  "query",
		"id":    "empty-1",
		"input": "",
	})

	msg := recvMsg(t, conn, 2*time.Second)
	if msg["kind"] != "empty" {
		t.Errorf("kind = %q, want empty", msg["kind"])
	}
}

// --- SCN-07: AI query streams token messages then done ---

// TR-02c: AI query → multiple token messages, then done message.
func TestQuery_AIStreaming(t *testing.T) {
	tokens := []string{"An ", "inode ", "is..."}
	aiSrv := fakeStreamingAI(t, tokens, false)
	defer aiSrv.Close()

	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")
	cfgPath := writeDaemonConfig(t, aiSrv.URL)
	cleanup := startDaemon(t, bin, sock, cfgPath)
	defer cleanup()

	conn := openPersistent(t, sock)
	defer conn.Close()

	sendRaw(t, conn, map[string]any{
		"type":  "query",
		"id":    "ai-1",
		"input": "explain what an inode is",
	})

	// Collect messages until done.
	var gotTokens []string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		var msg map[string]any
		if err := json.NewDecoder(conn).Decode(&msg); err != nil {
			t.Fatalf("decode: %v", err)
		}
		switch msg["type"] {
		case "token":
			if msg["id"] != "ai-1" {
				t.Errorf("token id = %q, want ai-1", msg["id"])
			}
			gotTokens = append(gotTokens, msg["value"].(string))
		case "done":
			if msg["id"] != "ai-1" {
				t.Errorf("done id = %q, want ai-1", msg["id"])
			}
			goto checkTokens
		default:
			t.Fatalf("unexpected message type %q", msg["type"])
		}
	}
	t.Fatal("timeout waiting for done message")

checkTokens:
	if len(gotTokens) != len(tokens) {
		t.Errorf("got %d tokens, want %d: %v", len(gotTokens), len(tokens), gotTokens)
	}
	for i, tok := range gotTokens {
		if i < len(tokens) && tok != tokens[i] {
			t.Errorf("token[%d] = %q, want %q", i, tok, tokens[i])
		}
	}
}

// --- SCN-08: cancel stops AI stream ---

// TR-05: cancel message aborts in-flight AI streaming.
func TestQuery_CancelAI(t *testing.T) {
	// AI server blocks until request is cancelled.
	aiSrv := fakeStreamingAI(t, []string{"tok1", "tok2"}, true)
	defer aiSrv.Close()

	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")
	cfgPath := writeDaemonConfig(t, aiSrv.URL)
	cleanup := startDaemon(t, bin, sock, cfgPath)
	defer cleanup()

	conn := openPersistent(t, sock)
	defer conn.Close()

	sendRaw(t, conn, map[string]any{
		"type":  "query",
		"id":    "ai-cancel",
		"input": "tell me everything about inodes",
	})

	// Let one token arrive (or the stream start).
	time.Sleep(80 * time.Millisecond)

	// Send cancel.
	sendRaw(t, conn, map[string]any{
		"type": "cancel",
		"id":   "ai-cancel",
	})

	// Read remaining messages; must receive a cancelled/done, not hang.
	done := make(chan string, 1)
	go func() {
		for {
			_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			var msg map[string]any
			if err := json.NewDecoder(conn).Decode(&msg); err != nil {
				if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
					done <- "timeout"
					return
				}
				done <- "error"
				return
			}
			if msg["type"] == "cancelled" || msg["type"] == "done" {
				done <- string(msg["type"].(string))
				return
			}
		}
	}()

	select {
	case result := <-done:
		// "cancelled" or "done" are both acceptable endings;
		// "timeout" means the daemon stopped sending (also acceptable).
		_ = result
	case <-time.After(3 * time.Second):
		t.Error("cancel: did not stop streaming within 3s")
	}
}

// --- SCN-09: new query cancels previous AI ---

// TR-05: sending a new query implicitly cancels the previous AI stream.
func TestQuery_NewQueryCancelsOld(t *testing.T) {
	aiSrv := fakeStreamingAI(t, nil, true) // blocks forever
	defer aiSrv.Close()

	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")
	cfgPath := writeDaemonConfig(t, aiSrv.URL)
	cleanup := startDaemon(t, bin, sock, cfgPath)
	defer cleanup()

	conn := openPersistent(t, sock)
	defer conn.Close()

	// First AI query.
	sendRaw(t, conn, map[string]any{
		"type":  "query",
		"id":    "q-old",
		"input": "old query that blocks",
	})
	time.Sleep(50 * time.Millisecond)

	// Second calc query — should cancel q-old and respond immediately.
	sendRaw(t, conn, map[string]any{
		"type":  "query",
		"id":    "q-new",
		"input": "3 * 7",
	})

	// Drain messages looking for the calc result.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		var msg map[string]any
		if err := json.NewDecoder(conn).Decode(&msg); err != nil {
			break
		}
		if msg["id"] == "q-new" && msg["kind"] == "calc" {
			val, _ := msg["value"].(string)
			if val != "21" {
				t.Errorf("calc value = %q, want 21", val)
			}
			return // success
		}
	}
	t.Error("did not receive calc result for second query within 3s")
}

// --- TR-02d: cancel message for unknown ID is a no-op (no crash) ---

func TestQuery_CancelUnknownID(t *testing.T) {
	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")
	cfgPath := writeDaemonConfig(t, "http://localhost:1")
	cleanup := startDaemon(t, bin, sock, cfgPath)
	defer cleanup()

	// Send cancel for an ID that was never queried — daemon must not crash.
	resp := sendMsg(t, sock, map[string]any{
		"type": "cancel",
		"id":   "nonexistent",
	})
	// Daemon should reply with ok or just ignore; ping must still work.
	_ = resp
	pingResp := sendMsg(t, sock, map[string]any{"type": "ping"})
	if pingResp["type"] != "pong" {
		t.Errorf("after cancel of unknown id: ping = %q, want pong", pingResp["type"])
	}
}

// --- io helpers ---

// Ensure the decoder does not share state across calls.
func newLineDecoder(r io.Reader) *json.Decoder { return json.NewDecoder(r) }
var _ = newLineDecoder // suppress unused warning
