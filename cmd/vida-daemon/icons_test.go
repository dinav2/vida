// Icon field integration tests for vida-daemon (SPEC-20260308-005).
// Verifies that app_list IPC responses include icon names parallel to app names.
package main_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeDesktopWithIcon writes a .desktop file that includes an Icon field.
func writeDesktopWithIcon(t *testing.T, dir, filename, name, icon string) {
	t.Helper()
	content := "[Desktop Entry]\nName=" + name + "\nIcon=" + icon +
		"\nExec=" + strings.ToLower(name) + "\nType=Application\n"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("writeDesktopWithIcon: %v", err)
	}
}

// FR-04a / R-02: app_list response must include icons field with icon names.
func TestQuery_AppListHasIcons(t *testing.T) {
	appsDir := t.TempDir()
	writeDesktopWithIcon(t, appsDir, "firefox.desktop", "Firefox Web Browser", "firefox")

	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")
	cfgPath := writeDaemonConfig(t, "http://localhost:1")

	env := []string{
		"HOME=/tmp",
		"VIDA_SOCKET=" + sock,
		"VIDA_CONFIG=" + cfgPath,
		"VIDA_APPS_DIRS=" + appsDir,
	}
	cleanup := startDaemonEnv(t, bin, sock, env)
	defer cleanup()

	time.Sleep(300 * time.Millisecond) // wait for indexing

	conn := openPersistent(t, sock)
	defer conn.Close()

	sendRaw(t, conn, map[string]any{
		"type":  "query",
		"id":    "icon-1",
		"input": "firefox",
	})

	msg := recvMsg(t, conn, 2*time.Second)

	if msg["kind"] != "app_list" {
		t.Fatalf("kind = %q, want app_list", msg["kind"])
	}

	// icons field must be present (FR-04a)
	icons, _ := msg["icons"].(string)
	if icons == "" {
		t.Fatalf("icons field missing from app_list response — icon names not sent (FR-04a)")
	}

	// icon name must match what was in the .desktop file (R-02)
	if !strings.Contains(icons, "firefox") {
		t.Errorf("icons %q does not contain expected icon name 'firefox' (R-02)", icons)
	}
}

// R-03: app with no Icon field sends empty string in icons (not crash, not absent).
func TestQuery_AppListIconsMissingField(t *testing.T) {
	appsDir := t.TempDir()
	// Desktop file with no Icon= line
	writeDesktopRaw(t, appsDir, "noicon.desktop",
		"[Desktop Entry]\nName=NoIconApp\nExec=noicon\nType=Application\n")

	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")
	cfgPath := writeDaemonConfig(t, "http://localhost:1")

	env := []string{
		"HOME=/tmp",
		"VIDA_SOCKET=" + sock,
		"VIDA_CONFIG=" + cfgPath,
		"VIDA_APPS_DIRS=" + appsDir,
	}
	cleanup := startDaemonEnv(t, bin, sock, env)
	defer cleanup()

	time.Sleep(300 * time.Millisecond)

	conn := openPersistent(t, sock)
	defer conn.Close()

	sendRaw(t, conn, map[string]any{
		"type":  "query",
		"id":    "icon-2",
		"input": "noicon",
	})

	msg := recvMsg(t, conn, 2*time.Second)

	if msg["kind"] != "app_list" {
		t.Fatalf("kind = %q, want app_list", msg["kind"])
	}

	// icons field must still be present (can be empty string — that is fine)
	// but the response must not omit the key entirely
	_, hasIcons := msg["icons"]
	if !hasIcons {
		t.Errorf("icons key absent from app_list when app has no Icon field — should be empty string (R-03)")
	}
}

// Parallel count check: icons count matches names count.
func TestQuery_AppListIconsParallelToNames(t *testing.T) {
	appsDir := t.TempDir()
	writeDesktopWithIcon(t, appsDir, "app1.desktop", "AppOne", "icon-one")
	writeDesktopWithIcon(t, appsDir, "app2.desktop", "AppTwo", "icon-two")

	bin := buildDaemon(t)
	sock := filepath.Join(t.TempDir(), "vida.sock")
	cfgPath := writeDaemonConfig(t, "http://localhost:1")

	env := []string{
		"HOME=/tmp",
		"VIDA_SOCKET=" + sock,
		"VIDA_CONFIG=" + cfgPath,
		"VIDA_APPS_DIRS=" + appsDir,
	}
	cleanup := startDaemonEnv(t, bin, sock, env)
	defer cleanup()

	time.Sleep(300 * time.Millisecond)

	conn := openPersistent(t, sock)
	defer conn.Close()

	sendRaw(t, conn, map[string]any{
		"type":  "query",
		"id":    "icon-3",
		"input": "app",
	})

	msg := recvMsg(t, conn, 2*time.Second)
	if msg["kind"] != "app_list" {
		t.Fatalf("kind = %q, want app_list", msg["kind"])
	}

	names, _ := msg["message"].(string)
	icons, _ := msg["icons"].(string)

	nameCount := len(strings.Split(strings.TrimSpace(names), "\n"))
	iconCount := len(strings.Split(strings.TrimSpace(icons), "\n"))

	if nameCount != iconCount {
		t.Errorf("icons count (%d) != names count (%d) — must be parallel arrays", iconCount, nameCount)
	}
}
