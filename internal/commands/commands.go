// Package commands implements the vida command registry.
// Commands are named actions: built-in system ops, user-defined shell snippets,
// and AI commands that use the existing streaming infrastructure.
package commands

import (
	"strings"

	"github.com/dinav2/vida/internal/config"
)

// Kind classifies a command's execution strategy.
const (
	KindSystem = "system" // shell exec, fire-and-forget or captured output
	KindAI     = "ai"    // routed through AI provider with fixed system prompt
	KindUser   = "user"  // user-defined shell exec
	KindNote   = "note"  // note form — handled client-side in the UI
)

// Command is a single named action in the registry.
type Command struct {
	Name   string
	Desc   string
	Icon   string
	Kind   string // KindSystem | KindAI | KindUser
	Exec   string // shell command; empty for reload-vida and AI commands
	Output string // "none" | "palette"

	// SystemPrompt is set for AI commands.
	SystemPrompt string
}

// Registry holds all available commands.
type Registry struct {
	commands []Command
}

// builtinSystem is the default set of system commands.
var builtinSystem = []Command{
	{Name: "lock", Desc: "Lock screen", Icon: "system-lock-screen",
		Kind: KindSystem, Exec: "loginctl lock-session", Output: "none"},
	{Name: "sleep", Desc: "Suspend system", Icon: "system-suspend",
		Kind: KindSystem, Exec: "systemctl suspend", Output: "none"},
	{Name: "reboot", Desc: "Reboot system", Icon: "system-reboot",
		Kind: KindSystem, Exec: "systemctl reboot", Output: "none"},
	{Name: "shutdown", Desc: "Power off system", Icon: "system-shutdown",
		Kind: KindSystem, Exec: "systemctl poweroff", Output: "none"},
	{Name: "reload-hypr", Desc: "Reload Hyprland config", Icon: "view-refresh",
		Kind: KindSystem, Exec: "hyprctl reload", Output: "none"},
	{Name: "kill-window", Desc: "Kill active window", Icon: "window-close",
		Kind: KindSystem, Exec: "hyprctl kill", Output: "none"},
	// reload-vida: Exec is empty — daemon handles it via IPC reload message.
	{Name: "reload-vida", Desc: "Reload vida index and config", Icon: "view-refresh-symbolic",
		Kind: KindSystem, Exec: "", Output: "none"},
	// note: handled client-side in vida-ui; daemon returns command_done immediately.
	{Name: "note", Desc: "Create a new note", Icon: "text-editor",
		Kind: KindNote, Exec: "", Output: "none"},
}

// builtinAI is the set of AI-backed commands with fixed system prompts.
var builtinAI = []Command{
	{
		Name: "translate", Desc: "Translate text", Icon: "accessories-dictionary",
		Kind:         KindAI,
		SystemPrompt: "Translate the following text to English. If a target language is specified as the first word (e.g. 'es', 'fr', 'de'), translate to that language instead. Output only the translation, no commentary.",
	},
	{
		Name: "explain", Desc: "Explain a concept simply", Icon: "dialog-information",
		Kind:         KindAI,
		SystemPrompt: "Explain the following in 2-3 plain sentences suitable for a command palette display. No markdown headers. Be concise.",
	},
	{
		Name: "define", Desc: "Define a word or phrase", Icon: "accessories-dictionary",
		Kind:         KindAI,
		SystemPrompt: "Give a dictionary definition of the following word or phrase. Include part of speech, primary definition, and brief etymology if known. No markdown headers.",
	},
	{
		Name: "fix", Desc: "Fix grammar and spelling", Icon: "tools-check-spelling",
		Kind:         KindAI,
		SystemPrompt: "Fix the spelling and grammar of the following text. Output only the corrected text, nothing else.",
	},
	{
		Name: "summarize", Desc: "Summarize text", Icon: "format-justify-fill",
		Kind:         KindAI,
		SystemPrompt: "Summarize the following text as concise bullet points. No markdown headers. Max 5 bullets.",
	},
}

// NewRegistry creates a Registry with built-in commands plus any user config.
// User commands override built-ins of the same name (FR-04f).
// Built-in exec strings are overridable via cfg.Builtins (FR-03c).
func NewRegistry(cfg *config.CommandsConfig) *Registry {
	// Start with copies of built-ins so we can mutate safely.
	overrideNames := make(map[string]bool)

	var userCmds []Command
	if cfg != nil {
		for _, u := range cfg.User {
			out := u.Output
			if out == "" {
				out = "none"
			}
			userCmds = append(userCmds, Command{
				Name:   u.Name,
				Desc:   u.Desc,
				Icon:   u.Icon,
				Kind:   KindUser,
				Exec:   u.Exec,
				Output: out,
			})
			overrideNames[u.Name] = true
		}
	}

	var system []Command
	for _, b := range builtinSystem {
		if overrideNames[b.Name] {
			continue // user override wins
		}
		// Apply exec override from config.
		if cfg != nil {
			if ov, ok := cfg.Builtins[b.Name]; ok && ov.Exec != "" {
				b.Exec = ov.Exec
			}
		}
		system = append(system, b)
	}

	var ai []Command
	for _, a := range builtinAI {
		if overrideNames[a.Name] {
			continue
		}
		ai = append(ai, a)
	}

	// Order: system built-ins → AI built-ins → user-defined (FR-02e).
	all := make([]Command, 0, len(system)+len(ai)+len(userCmds))
	all = append(all, system...)
	all = append(all, ai...)
	all = append(all, userCmds...)

	return &Registry{commands: all}
}

// Filter returns commands whose name or description fuzzy-matches query.
// Empty query returns all commands (FR-02a).
func (r *Registry) Filter(query string) []Command {
	if query == "" {
		return append([]Command(nil), r.commands...)
	}
	q := strings.ToLower(query)
	var out []Command
	for _, c := range r.commands {
		if strings.Contains(strings.ToLower(c.Name), q) ||
			strings.Contains(strings.ToLower(c.Desc), q) {
			out = append(out, c)
		}
	}
	return out
}

// Get returns the command with the given name and whether it was found.
func (r *Registry) Get(name string) (Command, bool) {
	for _, c := range r.commands {
		if c.Name == name {
			return c, true
		}
	}
	return Command{}, false
}

// ExpandInput replaces {input} in exec with the provided input string (FR-04d).
func ExpandInput(exec, input string) string {
	return strings.ReplaceAll(exec, "{input}", input)
}
