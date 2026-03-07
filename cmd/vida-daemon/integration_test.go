// Integration tests for vida-daemon.
// Tests cover SCN-01 (startup), SCN-14 (provider switch via reload), SCN-18 (shortcut reload).
// These tests start a real daemon subprocess.
package main_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// buildDaemon compiles vida-daemon and returns the binary path.
func buildDaemon(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "vida-daemon")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "." // cmd/vida-daemon
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build vida-daemon: %v\n%s", err, out)
	}
	return bin
}

// startDaemon starts vida-daemon, waits for socket, returns cleanup func.
func startDaemon(t *testing.T, bin, sock, configPath string) func() {
	t.Helper()

	env := append(os.Environ(),
		"XDG_RUNTIME_DIR="+filepath.Dir(sock),
		"VIDA_SOCKET="+sock,
	)
	if configPath != "" {
		env = append(env, "VIDA_CONFIG="+configPath)
	}

	cmd := exec.Command(bin)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	// Wait for socket to accept connections (up to 500ms).
	// Use net.Dial rather than os.Stat so that a stale socket file from a
	// previously killed daemon does not produce a false positive.
	deadline := time.Now().Add(500 * time.Millisecond)
	ready := false
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("unix", sock, 50*time.Millisecond)
		if err == nil {
			c.Close()
			ready = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		cmd.Process.Kill()
		t.Fatalf("socket %q did not become ready within 500ms", sock)
	}

	return func() {
		cmd.Process.Kill()
		_ = cmd.Wait()
	}
}

// sendMsg sends a JSON message over a unix socket and returns the response.
func sendMsg(t *testing.T, sock string, msg map[string]any) map[string]any {
	t.Helper()
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("Dial %s: %v", sock, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	if err := json.NewEncoder(conn).Encode(msg); err != nil {
		t.Fatalf("encode: %v", err)
	}

	var resp map[string]any
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

// writeTOML writes a TOML config file to a temp dir.
func writeTOMLConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// --- SCN-01: Daemon starts and socket is ready ---

func TestDaemon_SocketReady(t *testing.T) {
	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")

	start := time.Now()
	cleanup := startDaemon(t, bin, sock, "")
	defer cleanup()
	elapsed := time.Since(start)

	// AC-P3: socket ready within 200ms
	if elapsed > 200*time.Millisecond {
		t.Errorf("socket ready in %v, want < 200ms (AC-P3)", elapsed)
	}

	// vida ping should return pong
	resp := sendMsg(t, sock, map[string]any{"type": "ping"})
	if resp["type"] != "pong" {
		t.Errorf("ping response type = %q, want pong", resp["type"])
	}
}

// AC-P2: daemon idle RSS < 30MB
func TestDaemon_IdleRSS(t *testing.T) {
	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")
	cleanup := startDaemon(t, bin, sock, "")
	defer cleanup()

	// Allow indexing to complete
	time.Sleep(2 * time.Second)

	// Find daemon PID via status command
	resp := sendMsg(t, sock, map[string]any{"type": "status"})
	pidVal, ok := resp["pid"]
	if !ok {
		t.Skip("daemon status did not include pid; skipping RSS check")
	}
	pid := int(pidVal.(float64))

	rssKB := readRSS(t, pid)
	rssKB_limit := 30 * 1024 // 30 MB in KB
	if rssKB > rssKB_limit {
		t.Errorf("daemon RSS = %d KB (%d MB), want < 30 MB (AC-P2)", rssKB, rssKB/1024)
	}
}

func readRSS(t *testing.T, pid int) int {
	t.Helper()
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		t.Fatalf("read /proc/%d/status: %v", pid, err)
	}
	var rss int
	for _, line := range splitLines(string(data)) {
		if len(line) > 6 && line[:6] == "VmRSS:" {
			fmt.Sscanf(line[6:], "%d", &rss)
			return rss
		}
	}
	t.Fatalf("VmRSS not found in /proc/%d/status", pid)
	return 0
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	return lines
}

// --- SCN-14: AI provider switch via reload ---

func TestDaemon_ProviderSwitch(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" || os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY and OPENAI_API_KEY required for provider switch test")
	}

	bin := buildDaemon(t)

	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.toml")

	writeConfig := func(provider string) {
		content := fmt.Sprintf(`
[ai]
provider = "%s"
`, provider)
		_ = os.WriteFile(configPath, []byte(content), 0600)
	}

	sock := filepath.Join(t.TempDir(), "vida.sock")

	writeConfig("claude")
	cleanup := startDaemon(t, bin, sock, configPath)
	defer cleanup()

	// Verify initial provider
	resp := sendMsg(t, sock, map[string]any{"type": "status"})
	if resp["provider"] != "claude" {
		t.Errorf("initial provider = %q, want claude", resp["provider"])
	}

	// Switch to openai
	writeConfig("openai")
	sendMsg(t, sock, map[string]any{"type": "reload"})
	time.Sleep(100 * time.Millisecond)

	resp = sendMsg(t, sock, map[string]any{"type": "status"})
	if resp["provider"] != "openai" {
		t.Errorf("after reload provider = %q, want openai", resp["provider"])
	}
}

// --- SCN-18: New shortcut active after reload ---

func TestDaemon_ShortcutReload(t *testing.T) {
	bin := buildDaemon(t)
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.toml")

	_ = os.WriteFile(configPath, []byte(`
[ai]
provider = "claude"
`), 0600)

	sock := filepath.Join(t.TempDir(), "vida.sock")
	cleanup := startDaemon(t, bin, sock, configPath)
	defer cleanup()

	// Confirm "docs" shortcut is not present initially
	resp := sendMsg(t, sock, map[string]any{
		"type":  "query",
		"id":    "test-1",
		"input": "docs context",
	})
	// Should not be a shortcut result
	if resp["kind"] == "shortcut" {
		t.Error("'docs' shortcut matched before it was configured")
	}

	// Add docs shortcut to config
	_ = os.WriteFile(configPath, []byte(`
[ai]
provider = "claude"

[search.shortcuts]
docs = "https://pkg.go.dev/search?q=%s"
`), 0600)

	sendMsg(t, sock, map[string]any{"type": "reload"})
	time.Sleep(100 * time.Millisecond)

	// Now "docs context" should resolve as shortcut
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = ctx

	resp = sendMsg(t, sock, map[string]any{
		"type":  "query",
		"id":    "test-2",
		"input": "docs context",
	})
	if resp["kind"] != "shortcut" {
		t.Errorf("after reload: query kind = %q, want shortcut", resp["kind"])
	}
}

// FR-01b: stale socket is cleaned up on restart
func TestDaemon_StaleSocketCleanup(t *testing.T) {
	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")

	// Start and stop daemon (leaves socket file)
	cleanup := startDaemon(t, bin, sock, "")
	cleanup()
	time.Sleep(100 * time.Millisecond)

	// Socket file may still exist; second start should succeed
	cleanup2 := startDaemon(t, bin, sock, "")
	defer cleanup2()

	resp := sendMsg(t, sock, map[string]any{"type": "ping"})
	if resp["type"] != "pong" {
		t.Errorf("after restart: ping = %q, want pong", resp["type"])
	}
}
