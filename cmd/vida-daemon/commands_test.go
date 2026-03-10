// Integration tests for command mode IPC (SPEC-20260309-006).
package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// SCN-01: querying ":" returns kind=command_list with all built-in commands.
func TestCommand_ColonReturnsCommandList(t *testing.T) {
	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")
	cfgPath := writeDaemonConfig(t, "http://localhost:1")
	cleanup := startDaemon(t, bin, sock, cfgPath)
	defer cleanup()

	conn := openPersistent(t, sock)
	defer conn.Close()

	sendRaw(t, conn, map[string]any{
		"type":  "query",
		"id":    "cmd-1",
		"input": ":",
	})

	msg := recvMsg(t, conn, 2*time.Second)

	if msg["kind"] != "command_list" {
		t.Fatalf("kind = %q, want command_list (SCN-01)", msg["kind"])
	}

	// commands JSON must be non-empty
	cmdsRaw, _ := msg["message"].(string)
	if cmdsRaw == "" {
		t.Fatal("command_list message is empty — no commands serialized")
	}

	var cmds []map[string]any
	if err := json.Unmarshal([]byte(cmdsRaw), &cmds); err != nil {
		t.Fatalf("command_list message is not valid JSON array: %v\nraw: %s", err, cmdsRaw)
	}

	names := make(map[string]bool)
	for _, c := range cmds {
		if n, ok := c["name"].(string); ok {
			names[n] = true
		}
	}

	for _, builtin := range []string{"lock", "sleep", "reload-hypr", "kill-window"} {
		if !names[builtin] {
			t.Errorf("built-in command %q missing from command_list (FR-03a)", builtin)
		}
	}
}

// SCN-02/03: querying ":lo" fuzzy-filters to lock.
func TestCommand_FuzzyFilter(t *testing.T) {
	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")
	cfgPath := writeDaemonConfig(t, "http://localhost:1")
	cleanup := startDaemon(t, bin, sock, cfgPath)
	defer cleanup()

	conn := openPersistent(t, sock)
	defer conn.Close()

	sendRaw(t, conn, map[string]any{
		"type":  "query",
		"id":    "cmd-2",
		"input": ":lo",
	})

	msg := recvMsg(t, conn, 2*time.Second)
	if msg["kind"] != "command_list" {
		t.Fatalf("kind = %q, want command_list", msg["kind"])
	}

	cmdsRaw, _ := msg["message"].(string)
	var cmds []map[string]any
	if err := json.Unmarshal([]byte(cmdsRaw), &cmds); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	found := false
	for _, c := range cmds {
		if c["name"] == "lock" {
			found = true
		}
	}
	if !found {
		t.Errorf("\":lo\" did not return 'lock' in filtered list (SCN-03)")
	}
}

// SCN-04: run_command for "lock" returns command_done.
func TestCommand_RunCommandDone(t *testing.T) {
	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")

	// Override lock to a no-op so it doesn't actually lock in CI
	cfgContent := `
[ai]
provider = "claude"

[ai.claude]
api_key  = "test-key"
base_url = "http://localhost:1"

[commands.builtins.lock]
exec = "true"
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cleanup := startDaemon(t, bin, sock, cfgPath)
	defer cleanup()

	conn := openPersistent(t, sock)
	defer conn.Close()

	sendRaw(t, conn, map[string]any{
		"type":  "run_command",
		"id":    "run-1",
		"name":  "lock",
		"input": "",
	})

	msg := recvMsg(t, conn, 3*time.Second)
	if msg["type"] != "command_done" {
		t.Errorf("run_command 'lock' response type = %q, want command_done (SCN-04)", msg["type"])
	}
	if msg["id"] != "run-1" {
		t.Errorf("response id = %q, want run-1", msg["id"])
	}
}

// SCN-05: run_command with output=palette returns command_result with stdout.
func TestCommand_RunCommandPaletteOutput(t *testing.T) {
	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")

	cfgContent := `
[ai]
provider = "claude"

[ai.claude]
api_key  = "test-key"
base_url = "http://localhost:1"

[[commands.user]]
name   = "greet"
exec   = "echo hello from vida"
output = "palette"
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cleanup := startDaemon(t, bin, sock, cfgPath)
	defer cleanup()

	conn := openPersistent(t, sock)
	defer conn.Close()

	sendRaw(t, conn, map[string]any{
		"type":  "run_command",
		"id":    "run-2",
		"name":  "greet",
		"input": "",
	})

	msg := recvMsg(t, conn, 3*time.Second)
	if msg["type"] != "command_result" {
		t.Fatalf("type = %q, want command_result (SCN-05)", msg["type"])
	}
	val, _ := msg["value"].(string)
	if !strings.Contains(val, "hello from vida") {
		t.Errorf("command_result value = %q, want 'hello from vida'", val)
	}
}

// SCN-06/08: {input} placeholder substituted correctly.
func TestCommand_InputPlaceholderSubstituted(t *testing.T) {
	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")

	cfgContent := `
[ai]
provider = "claude"

[ai.claude]
api_key  = "test-key"
base_url = "http://localhost:1"

[[commands.user]]
name   = "echo-input"
exec   = "echo '{input}'"
output = "palette"
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cleanup := startDaemon(t, bin, sock, cfgPath)
	defer cleanup()

	conn := openPersistent(t, sock)
	defer conn.Close()

	sendRaw(t, conn, map[string]any{
		"type":  "run_command",
		"id":    "run-3",
		"name":  "echo-input",
		"input": "hello world",
	})

	msg := recvMsg(t, conn, 3*time.Second)
	if msg["type"] != "command_result" {
		t.Fatalf("type = %q, want command_result", msg["type"])
	}
	val, _ := msg["value"].(string)
	if !strings.Contains(val, "hello world") {
		t.Errorf("{input} not substituted: value = %q (SCN-06)", val)
	}
}

// SCN-09: user commands reloaded after reload message.
func TestCommand_UserCommandReloaded(t *testing.T) {
	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	baseCfg := `
[ai]
provider = "claude"

[ai.claude]
api_key  = "test-key"
base_url = "http://localhost:1"
`
	if err := os.WriteFile(cfgPath, []byte(baseCfg), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cleanup := startDaemon(t, bin, sock, cfgPath)
	defer cleanup()

	conn := openPersistent(t, sock)
	defer conn.Close()

	// Confirm "newcmd" not present initially
	sendRaw(t, conn, map[string]any{"type": "query", "id": "q1", "input": ":newcmd"})
	msg := recvMsg(t, conn, 2*time.Second)
	cmdsRaw, _ := msg["message"].(string)
	if strings.Contains(cmdsRaw, "newcmd") {
		t.Error("newcmd present before reload — unexpected")
	}

	// Add the command and reload
	newCfg := baseCfg + `
[[commands.user]]
name = "newcmd"
exec = "echo new"
output = "palette"
`
	if err := os.WriteFile(cfgPath, []byte(newCfg), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	conn2 := openPersistent(t, sock)
	defer conn2.Close()
	sendRaw(t, conn2, map[string]any{"type": "reload"})
	recvMsg(t, conn2, 2*time.Second)
	time.Sleep(50 * time.Millisecond)

	// Now newcmd should be available
	conn3 := openPersistent(t, sock)
	defer conn3.Close()
	sendRaw(t, conn3, map[string]any{"type": "query", "id": "q2", "input": ":newcmd"})
	msg2 := recvMsg(t, conn3, 2*time.Second)
	cmdsRaw2, _ := msg2["message"].(string)
	if !strings.Contains(cmdsRaw2, "newcmd") {
		t.Errorf("newcmd not available after reload (SCN-09): commands = %s", cmdsRaw2)
	}
}

// SCN-11: exec exits non-zero → command_error returned.
func TestCommand_ExecFailure(t *testing.T) {
	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")

	cfgContent := `
[ai]
provider = "claude"

[ai.claude]
api_key  = "test-key"
base_url = "http://localhost:1"

[[commands.user]]
name   = "failcmd"
exec   = "exit 1"
output = "palette"
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cleanup := startDaemon(t, bin, sock, cfgPath)
	defer cleanup()

	conn := openPersistent(t, sock)
	defer conn.Close()

	sendRaw(t, conn, map[string]any{
		"type":  "run_command",
		"id":    "run-fail",
		"name":  "failcmd",
		"input": "",
	})

	msg := recvMsg(t, conn, 3*time.Second)
	if msg["type"] != "command_error" {
		t.Errorf("type = %q, want command_error for non-zero exit (SCN-11)", msg["type"])
	}
}
