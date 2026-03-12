// Package config loads and provides vida configuration from a TOML file.
package config

import (
	"errors"
	"os"

	"github.com/BurntSushi/toml"
)

// Default values applied when config file is missing or fields are unset.
const (
	DefaultProvider   = "claude"
	DefaultClaudeModel = "claude-sonnet-4-6"
	DefaultOpenAIModel = "gpt-4o"
	DefaultUIWidth     = 640
	DefaultUIPosition  = "center"
	DefaultUIMaxResults = 8
)

var defaultShortcuts = map[string]string{
	"g":  "https://www.google.com/search?q=%s",
	"gh": "https://github.com/search?q=%s",
	"yt": "https://www.youtube.com/results?search_query=%s",
	"dd": "https://duckduckgo.com/?q=%s",
}

// Config is the top-level configuration structure.
type Config struct {
	AI       AIConfig       `toml:"ai"`
	Search   SearchConfig   `toml:"search"`
	UI       UIConfig       `toml:"ui"`
	Commands CommandsConfig `toml:"commands"`
	Notes    NotesConfig    `toml:"notes"`
}

// NotesConfig holds settings for the :note command.
type NotesConfig struct {
	Dir         string `toml:"dir"`
	DailySubdir string `toml:"daily_subdir"`
	InboxSubdir string `toml:"inbox_subdir"`
	Template    string `toml:"template"`
}

// CommandsConfig holds built-in overrides and user-defined commands.
type CommandsConfig struct {
	Builtins map[string]BuiltinOverride `toml:"builtins"`
	User     []UserCommand              `toml:"user"`
}

// BuiltinOverride lets users replace the exec string of a built-in command.
type BuiltinOverride struct {
	Exec string `toml:"exec"`
}

// UserCommand is a user-defined shell command exposed in command mode.
type UserCommand struct {
	Name   string `toml:"name"`
	Desc   string `toml:"desc"`
	Icon   string `toml:"icon"`
	Exec   string `toml:"exec"`
	Output string `toml:"output"` // "none" | "palette"; default "none"
}

// AIConfig holds AI provider selection and per-provider settings.
type AIConfig struct {
	Provider string       `toml:"provider"`
	Claude   ClaudeConfig `toml:"claude"`
	OpenAI   OpenAIConfig `toml:"openai"`
}

// ClaudeConfig holds Anthropic-specific settings.
type ClaudeConfig struct {
	APIKey  string `toml:"api_key"`
	Model   string `toml:"model"`
	BaseURL string `toml:"base_url"` // override for testing
}

// OpenAIConfig holds OpenAI-specific settings.
type OpenAIConfig struct {
	APIKey  string `toml:"api_key"`
	Model   string `toml:"model"`
	BaseURL string `toml:"base_url"` // override for testing
}

// SearchConfig holds web search shortcut definitions.
type SearchConfig struct {
	Shortcuts map[string]string `toml:"shortcuts"`
}

// UIConfig holds window appearance settings.
type UIConfig struct {
	Width      int    `toml:"width"`
	Position   string `toml:"position"`
	MaxResults int    `toml:"max_results"`
}

// Load reads the TOML config file at path and returns a Config with defaults
// applied for any unset fields. If the file does not exist, all defaults are
// used and no error is returned. If the file exists but is invalid TOML, an
// error is returned.
func Load(path string) (*Config, error) {
	cfg := defaults()

	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}

	// Decode into a separate struct and merge, so TOML zero values don't
	// overwrite defaults for fields the user didn't specify.
	var raw Config
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return nil, err
	}

	merge(cfg, &raw)
	return cfg, nil
}

// EffectiveClaudeKey returns the Anthropic API key to use: config file value
// takes precedence over the ANTHROPIC_API_KEY environment variable.
func (c *Config) EffectiveClaudeKey() string {
	if c.AI.Claude.APIKey != "" {
		return c.AI.Claude.APIKey
	}
	return os.Getenv("ANTHROPIC_API_KEY")
}

// EffectiveOpenAIKey returns the OpenAI API key to use: config file value
// takes precedence over the OPENAI_API_KEY environment variable.
func (c *Config) EffectiveOpenAIKey() string {
	if c.AI.OpenAI.APIKey != "" {
		return c.AI.OpenAI.APIKey
	}
	return os.Getenv("OPENAI_API_KEY")
}

// defaults returns a Config populated with all default values.
func defaults() *Config {
	shortcuts := make(map[string]string, len(defaultShortcuts))
	for k, v := range defaultShortcuts {
		shortcuts[k] = v
	}
	return &Config{
		AI: AIConfig{
			Provider: DefaultProvider,
			Claude:   ClaudeConfig{Model: DefaultClaudeModel},
			OpenAI:   OpenAIConfig{Model: DefaultOpenAIModel},
		},
		Search: SearchConfig{
			Shortcuts: shortcuts,
		},
		UI: UIConfig{
			Width:      DefaultUIWidth,
			Position:   DefaultUIPosition,
			MaxResults: DefaultUIMaxResults,
		},
	}
}

// merge applies non-zero values from raw into cfg.
func merge(cfg, raw *Config) {
	// AI
	if raw.AI.Provider != "" {
		cfg.AI.Provider = raw.AI.Provider
	}
	if raw.AI.Claude.APIKey != "" {
		cfg.AI.Claude.APIKey = raw.AI.Claude.APIKey
	}
	if raw.AI.Claude.Model != "" {
		cfg.AI.Claude.Model = raw.AI.Claude.Model
	}
	if raw.AI.Claude.BaseURL != "" {
		cfg.AI.Claude.BaseURL = raw.AI.Claude.BaseURL
	}
	if raw.AI.OpenAI.APIKey != "" {
		cfg.AI.OpenAI.APIKey = raw.AI.OpenAI.APIKey
	}
	if raw.AI.OpenAI.Model != "" {
		cfg.AI.OpenAI.Model = raw.AI.OpenAI.Model
	}
	if raw.AI.OpenAI.BaseURL != "" {
		cfg.AI.OpenAI.BaseURL = raw.AI.OpenAI.BaseURL
	}

	// UI
	if raw.UI.Width != 0 {
		cfg.UI.Width = raw.UI.Width
	}
	if raw.UI.Position != "" {
		cfg.UI.Position = raw.UI.Position
	}
	if raw.UI.MaxResults != 0 {
		cfg.UI.MaxResults = raw.UI.MaxResults
	}

	// Shortcuts: user-defined shortcuts replace defaults entirely.
	if len(raw.Search.Shortcuts) > 0 {
		cfg.Search.Shortcuts = raw.Search.Shortcuts
	}

	// Commands: always take raw values (no built-in defaults to preserve).
	if len(raw.Commands.User) > 0 {
		cfg.Commands.User = raw.Commands.User
	}
	if len(raw.Commands.Builtins) > 0 {
		cfg.Commands.Builtins = raw.Commands.Builtins
	}

	// Notes.
	if raw.Notes.Dir != "" {
		cfg.Notes.Dir = raw.Notes.Dir
	}
	if raw.Notes.DailySubdir != "" {
		cfg.Notes.DailySubdir = raw.Notes.DailySubdir
	}
	if raw.Notes.InboxSubdir != "" {
		cfg.Notes.InboxSubdir = raw.Notes.InboxSubdir
	}
	if raw.Notes.Template != "" {
		cfg.Notes.Template = raw.Notes.Template
	}
}
