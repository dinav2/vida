// Package config implements TOML configuration loading.
// Tests cover FR-10.
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dinav2/vida/internal/config"
)

func writeTOML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// FR-10b: missing config file returns defaults, no error
func TestLoad_MissingFile(t *testing.T) {
	cfg, err := config.Load("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatalf("Load missing file: expected no error, got %v", err)
	}
	if cfg == nil {
		t.Fatal("Load missing file: returned nil config")
	}
	// Verify defaults
	if cfg.AI.Provider != "claude" {
		t.Errorf("default AI.Provider = %q, want %q", cfg.AI.Provider, "claude")
	}
	if cfg.UI.Width != 640 {
		t.Errorf("default UI.Width = %d, want 640", cfg.UI.Width)
	}
	if cfg.UI.MaxResults != 8 {
		t.Errorf("default UI.MaxResults = %d, want 8", cfg.UI.MaxResults)
	}
	if cfg.UI.Position != "center" {
		t.Errorf("default UI.Position = %q, want %q", cfg.UI.Position, "center")
	}
}

// FR-10c: invalid TOML returns error
func TestLoad_InvalidTOML(t *testing.T) {
	path := writeTOML(t, "this is not [ valid toml !!!!")
	_, err := config.Load(path)
	if err == nil {
		t.Error("Load invalid TOML: expected error, got nil")
	}
}

// FR-10a: valid config is loaded correctly
func TestLoad_ValidConfig(t *testing.T) {
	path := writeTOML(t, `
[ai]
provider = "openai"

[ai.claude]
api_key = "sk-claude-test"
model = "claude-sonnet-4-6"

[ai.openai]
api_key = "sk-openai-test"
model = "gpt-4o"

[ui]
width = 800
position = "top"
max_results = 12

[search.shortcuts]
g  = "https://www.google.com/search?q=%s"
gh = "https://github.com/search?q=%s"
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.AI.Provider != "openai" {
		t.Errorf("AI.Provider = %q, want %q", cfg.AI.Provider, "openai")
	}
	if cfg.AI.Claude.APIKey != "sk-claude-test" {
		t.Errorf("Claude.APIKey = %q, want %q", cfg.AI.Claude.APIKey, "sk-claude-test")
	}
	if cfg.AI.OpenAI.Model != "gpt-4o" {
		t.Errorf("OpenAI.Model = %q, want %q", cfg.AI.OpenAI.Model, "gpt-4o")
	}
	if cfg.UI.Width != 800 {
		t.Errorf("UI.Width = %d, want 800", cfg.UI.Width)
	}
	if cfg.UI.MaxResults != 12 {
		t.Errorf("UI.MaxResults = %d, want 12", cfg.UI.MaxResults)
	}
	if len(cfg.Search.Shortcuts) != 2 {
		t.Errorf("Shortcuts count = %d, want 2", len(cfg.Search.Shortcuts))
	}
}

// FR-08c: API key falls back to env var (config awareness)
func TestLoad_EnvVarAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "env-anthropic-key")
	t.Setenv("OPENAI_API_KEY", "env-openai-key")

	// Config with no API keys set
	path := writeTOML(t, `
[ai]
provider = "claude"
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Config struct itself may not resolve env vars — that's the provider's job.
	// But config.EffectiveClaudeKey() should return the env var value.
	key := cfg.EffectiveClaudeKey()
	if key != "env-anthropic-key" {
		t.Errorf("EffectiveClaudeKey() = %q, want %q", key, "env-anthropic-key")
	}

	key = cfg.EffectiveOpenAIKey()
	if key != "env-openai-key" {
		t.Errorf("EffectiveOpenAIKey() = %q, want %q", key, "env-openai-key")
	}
}

// Config file key takes precedence over env var
func TestLoad_ConfigKeyPrecedence(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "env-key")

	path := writeTOML(t, `
[ai.claude]
api_key = "config-key"
`)

	cfg, _ := config.Load(path)
	key := cfg.EffectiveClaudeKey()
	if key != "config-key" {
		t.Errorf("EffectiveClaudeKey() = %q, want config key to take precedence", key)
	}
}

// Default shortcuts are present when not overridden
func TestLoad_DefaultShortcuts(t *testing.T) {
	cfg, _ := config.Load("/nonexistent")

	defaults := []string{"g", "gh", "yt", "dd"}
	for _, prefix := range defaults {
		if _, ok := cfg.Search.Shortcuts[prefix]; !ok {
			t.Errorf("default shortcut %q missing", prefix)
		}
	}
}

// Partial config merges with defaults
func TestLoad_PartialConfig(t *testing.T) {
	path := writeTOML(t, `
[ui]
width = 900
`)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.UI.Width != 900 {
		t.Errorf("UI.Width = %d, want 900", cfg.UI.Width)
	}
	// Unspecified fields should retain defaults
	if cfg.UI.MaxResults != 8 {
		t.Errorf("UI.MaxResults = %d, want default 8", cfg.UI.MaxResults)
	}
	if cfg.AI.Provider != "claude" {
		t.Errorf("AI.Provider = %q, want default 'claude'", cfg.AI.Provider)
	}
}
