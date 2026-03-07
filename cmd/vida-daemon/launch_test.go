// App launching integration tests for vida-daemon (SPEC-20260307-004).
// SCN-01, TR-03b: IPC app_list response must include desktop IDs alongside names.
package main_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TR-03b: app_list IPC response includes desktop IDs in a parallel field.
// Before fix: response only had names in "message"; IDs were never sent.
func TestQuery_AppListHasIDs(t *testing.T) {
	appsDir := t.TempDir()
	writeDesktop(t, appsDir, "myapp.desktop", "MyApp", "a great app")

	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")
	cfgPath := writeDaemonConfig(t, "http://localhost:1")

	env := append([]string{},
		"HOME=/tmp",
		"VIDA_SOCKET="+sock,
		"VIDA_CONFIG="+cfgPath,
		"VIDA_APPS_DIRS="+appsDir,
	)
	cleanup := startDaemonEnv(t, bin, sock, env)
	defer cleanup()

	time.Sleep(300 * time.Millisecond) // wait for indexing

	conn := openPersistent(t, sock)
	defer conn.Close()

	sendRaw(t, conn, map[string]any{
		"type":  "query",
		"id":    "app-ids-1",
		"input": "myapp",
	})

	msg := recvMsg(t, conn, 2*time.Second)

	if msg["kind"] != "app_list" {
		t.Fatalf("kind = %q, want app_list", msg["kind"])
	}

	// Names must be present
	message, _ := msg["message"].(string)
	if !strings.Contains(message, "MyApp") {
		t.Errorf("message %q missing app name", message)
	}

	// IDs must be present and parallel to names (TR-03b)
	ids, _ := msg["ids"].(string)
	if ids == "" {
		t.Fatalf("ids field is empty — desktop IDs not sent in app_list response (TR-03b)")
	}
	if !strings.Contains(ids, "myapp.desktop") {
		t.Errorf("ids %q missing desktop ID myapp.desktop", ids)
	}
}

// FR-03a: ExpandExec strips all .desktop field codes.
func TestQuery_AppListExecPresent(t *testing.T) {
	appsDir := t.TempDir()
	writeDesktopRaw(t, appsDir, "launcher.desktop",
		"[Desktop Entry]\nName=Launcher\nExec=myapp %u %F\nType=Application\n")

	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")
	cfgPath := writeDaemonConfig(t, "http://localhost:1")

	env := append([]string{},
		"HOME=/tmp",
		"VIDA_SOCKET="+sock,
		"VIDA_CONFIG="+cfgPath,
		"VIDA_APPS_DIRS="+appsDir,
	)
	cleanup := startDaemonEnv(t, bin, sock, env)
	defer cleanup()

	time.Sleep(300 * time.Millisecond)

	conn := openPersistent(t, sock)
	defer conn.Close()

	sendRaw(t, conn, map[string]any{
		"type":  "query",
		"id":    "exec-1",
		"input": "launcher",
	})

	msg := recvMsg(t, conn, 2*time.Second)
	if msg["kind"] != "app_list" {
		t.Fatalf("kind = %q, want app_list", msg["kind"])
	}

	// exec field must be present and must not contain raw placeholders (FR-03a)
	exec, _ := msg["exec"].(string)
	if exec == "" {
		t.Fatalf("exec field missing from app_list response")
	}
	if strings.Contains(exec, "%u") || strings.Contains(exec, "%F") {
		t.Errorf("exec %q still contains field codes (FR-03a)", exec)
	}
}

// writeDesktopRaw writes a .desktop file with arbitrary raw content.
func writeDesktopRaw(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("writeDesktopRaw: %v", err)
	}
}
