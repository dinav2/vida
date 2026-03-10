// Tests for the command registry (SPEC-20260309-006).
package commands_test

import (
	"strings"
	"testing"

	"github.com/dinav2/vida/internal/commands"
	"github.com/dinav2/vida/internal/config"
)

// --- FR-03: Built-in commands always available ---

func TestRegistry_BuiltinsPresent(t *testing.T) {
	r := commands.NewRegistry(nil)
	all := r.Filter("")
	names := make(map[string]bool)
	for _, c := range all {
		names[c.Name] = true
	}
	builtins := []string{"lock", "sleep", "reboot", "shutdown", "reload-hypr", "kill-window", "reload-vida"}
	for _, b := range builtins {
		if !names[b] {
			t.Errorf("built-in command %q not present in registry (FR-03a)", b)
		}
	}
}

// --- FR-02b: Fuzzy filter by name ---

func TestRegistry_FilterByName(t *testing.T) {
	r := commands.NewRegistry(nil)
	results := r.Filter("lo")
	found := false
	for _, c := range results {
		if c.Name == "lock" {
			found = true
		}
	}
	if !found {
		t.Errorf("Filter(\"lo\") did not return \"lock\" command (FR-02b)")
	}
}

func TestRegistry_FilterByDesc(t *testing.T) {
	cfg := &config.CommandsConfig{
		User: []config.UserCommand{
			{Name: "mytool", Desc: "Opens the project dashboard", Exec: "echo hi", Output: "palette"},
		},
	}
	r := commands.NewRegistry(cfg)
	results := r.Filter("dashboard")
	found := false
	for _, c := range results {
		if c.Name == "mytool" {
			found = true
		}
	}
	if !found {
		t.Errorf("Filter(\"dashboard\") did not match command by description (FR-02b)")
	}
}

func TestRegistry_FilterEmpty_ReturnsAll(t *testing.T) {
	r := commands.NewRegistry(nil)
	all := r.Filter("")
	if len(all) == 0 {
		t.Errorf("Filter(\"\") must return all commands (FR-02a)")
	}
}

func TestRegistry_FilterNoMatch_ReturnsEmpty(t *testing.T) {
	r := commands.NewRegistry(nil)
	results := r.Filter("zzznomatch")
	if len(results) != 0 {
		t.Errorf("Filter(\"zzznomatch\") should return empty slice, got %d results", len(results))
	}
}

// --- FR-04: User commands ---

func TestRegistry_UserCommandsLoaded(t *testing.T) {
	cfg := &config.CommandsConfig{
		User: []config.UserCommand{
			{Name: "note", Desc: "Open daily note", Exec: "echo note", Output: "none"},
		},
	}
	r := commands.NewRegistry(cfg)
	results := r.Filter("note")
	if len(results) == 0 {
		t.Fatalf("user command 'note' not found after loading config (FR-04a)")
	}
	if results[0].Name != "note" {
		t.Errorf("expected 'note', got %q", results[0].Name)
	}
}

func TestRegistry_UserCommandDefaultOutput(t *testing.T) {
	cfg := &config.CommandsConfig{
		User: []config.UserCommand{
			{Name: "mytask", Exec: "echo hi"}, // no Output set
		},
	}
	r := commands.NewRegistry(cfg)
	results := r.Filter("mytask")
	if len(results) == 0 {
		t.Fatal("mytask not found")
	}
	if results[0].Output != "none" {
		t.Errorf("default output must be 'none', got %q (FR-04c)", results[0].Output)
	}
}

// FR-04f: user command overrides built-in of same name
func TestRegistry_UserOverridesBuiltin(t *testing.T) {
	cfg := &config.CommandsConfig{
		User: []config.UserCommand{
			{Name: "lock", Desc: "Custom lock", Exec: "swaylock -f", Output: "none"},
		},
	}
	r := commands.NewRegistry(cfg)
	results := r.Filter("lock")
	// Should find exactly one "lock", and it should be the user override
	var found *commands.Command
	for i := range results {
		if results[i].Name == "lock" {
			found = &results[i]
			break
		}
	}
	if found == nil {
		t.Fatal("lock command not found")
	}
	if found.Exec != "swaylock -f" {
		t.Errorf("user override for 'lock' not applied: exec = %q, want 'swaylock -f' (FR-04f)", found.Exec)
	}
}

// --- FR-03c: Built-in exec override via config ---

func TestRegistry_BuiltinExecOverride(t *testing.T) {
	cfg := &config.CommandsConfig{
		Builtins: map[string]config.BuiltinOverride{
			"lock": {Exec: "swaylock --color 000000"},
		},
	}
	r := commands.NewRegistry(cfg)
	results := r.Filter("lock")
	var found *commands.Command
	for i := range results {
		if results[i].Name == "lock" {
			found = &results[i]
			break
		}
	}
	if found == nil {
		t.Fatal("lock not found")
	}
	if found.Exec != "swaylock --color 000000" {
		t.Errorf("built-in override not applied: exec = %q (FR-03c)", found.Exec)
	}
}

// --- FR-04d: {input} placeholder substitution ---

func TestRegistry_InputPlaceholder(t *testing.T) {
	cfg := &config.CommandsConfig{
		User: []config.UserCommand{
			{Name: "todo", Exec: "echo '- {input}' >> ~/todo.md", Output: "none"},
		},
	}
	r := commands.NewRegistry(cfg)
	cmd, ok := r.Get("todo")
	if !ok {
		t.Fatal("todo command not found")
	}
	expanded := commands.ExpandInput(cmd.Exec, "buy milk")
	if !strings.Contains(expanded, "buy milk") {
		t.Errorf("ExpandInput did not substitute {input}: got %q (FR-04d)", expanded)
	}
	if strings.Contains(expanded, "{input}") {
		t.Errorf("ExpandInput left {input} literal in exec: got %q", expanded)
	}
}

func TestRegistry_InputPlaceholder_Empty(t *testing.T) {
	result := commands.ExpandInput("echo '{input}'", "")
	if strings.Contains(result, "{input}") {
		t.Errorf("ExpandInput with empty input still contains {input}: %q", result)
	}
}

// --- FR-02e: ordering ---

func TestRegistry_OrderingBuiltinFirst(t *testing.T) {
	cfg := &config.CommandsConfig{
		User: []config.UserCommand{
			{Name: "zzuser", Exec: "echo hi"},
		},
	}
	r := commands.NewRegistry(cfg)
	all := r.Filter("")
	// Built-in system commands must appear before user commands
	builtinIdx := -1
	userIdx := -1
	for i, c := range all {
		if c.Name == "lock" {
			builtinIdx = i
		}
		if c.Name == "zzuser" {
			userIdx = i
		}
	}
	if builtinIdx < 0 || userIdx < 0 {
		t.Skip("commands not both found")
	}
	if builtinIdx > userIdx {
		t.Errorf("built-in 'lock' (idx %d) must appear before user 'zzuser' (idx %d) (FR-02e)", builtinIdx, userIdx)
	}
}

// --- AI commands present ---

func TestRegistry_AICommandsPresent(t *testing.T) {
	r := commands.NewRegistry(nil)
	all := r.Filter("")
	names := make(map[string]bool)
	for _, c := range all {
		names[c.Name] = true
	}
	for _, ai := range []string{"translate", "explain", "define", "fix", "summarize"} {
		if !names[ai] {
			t.Errorf("AI command %q not present in registry (FR-06a)", ai)
		}
	}
}

func TestRegistry_AICommandKind(t *testing.T) {
	r := commands.NewRegistry(nil)
	cmd, ok := r.Get("translate")
	if !ok {
		t.Fatal("translate command not found")
	}
	if cmd.Kind != "ai" {
		t.Errorf("translate command Kind = %q, want 'ai' (FR-06b)", cmd.Kind)
	}
}

// --- reload-vida is special ---

func TestRegistry_ReloadVidaKind(t *testing.T) {
	r := commands.NewRegistry(nil)
	cmd, ok := r.Get("reload-vida")
	if !ok {
		t.Fatal("reload-vida command not found")
	}
	if cmd.Kind != "system" {
		t.Errorf("reload-vida Kind = %q, want 'system'", cmd.Kind)
	}
	// reload-vida has no exec (daemon handles it via IPC)
	if cmd.Exec != "" {
		t.Errorf("reload-vida must have empty Exec, got %q (FR-03d)", cmd.Exec)
	}
}
