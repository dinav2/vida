// vida-daemon is the always-running background process for vida.
// It manages IPC, app indexing, query routing, and AI provider calls.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dinav2/vida/internal/ai"
	"github.com/dinav2/vida/internal/apps"
	"github.com/dinav2/vida/internal/clipboard"
	"github.com/dinav2/vida/internal/commands"
	"github.com/dinav2/vida/internal/config"
	"github.com/dinav2/vida/internal/db"
	"github.com/dinav2/vida/internal/files"
	"github.com/dinav2/vida/internal/ipc"
	"github.com/dinav2/vida/internal/router"
	"github.com/dinav2/vida/internal/shortcuts"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("vida-daemon: %v", err)
	}
}

func run() error {
	configPath := os.Getenv("VIDA_CONFIG")
	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".config", "vida", "config.toml")
	}

	sockPath := os.Getenv("VIDA_SOCKET")
	if sockPath == "" {
		runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
		if runtimeDir == "" {
			runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
		}
		sockPath = filepath.Join(runtimeDir, "vida.sock")
	}

	d := &daemon{
		configPath: configPath,
		sockPath:   sockPath,
		pid:        os.Getpid(),
		inflight:   make(map[string]context.CancelFunc),
	}

	if err := d.loadConfig(); err != nil {
		log.Printf("warning: config load failed (%v), using defaults", err)
	}
	if err := d.initDB(); err != nil {
		return fmt.Errorf("init db: %w", err)
	}
	defer d.database.Close()

	// Ensure registry is initialised even if loadConfig fails.
	d.mu.Lock()
	if d.cmdRegistry == nil {
		d.cmdRegistry = commands.NewRegistry(nil)
	}
	d.mu.Unlock()

	d.rebuildRouter()
	go d.indexApps()
	go d.indexFiles()

	srv, err := ipc.Listen(sockPath)
	if err != nil {
		return fmt.Errorf("ipc listen: %w", err)
	}
	defer srv.Close()

	srv.Handle(d.handleMessage)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go srv.Serve(ctx)

	d.mu.RLock()
	clipEnabled := d.cfg == nil || d.cfg.Clipboard.Enabled
	d.mu.RUnlock()
	if clipEnabled {
		go d.watchClipboard(ctx, srv)
	}

	log.Printf("vida-daemon: socket ready at %s (pid %d)", sockPath, d.pid)
	<-ctx.Done()
	log.Println("vida-daemon: shutting down")
	return nil
}

type daemon struct {
	configPath string
	sockPath   string
	pid        int

	mu          sync.RWMutex
	cfg         *config.Config
	provider    ai.AIProvider
	rtr         *router.Router
	appIndex    *apps.Index
	fileIndex   *files.Index
	database    *db.DB
	cmdRegistry *commands.Registry
	clipStore   *clipboard.Store

	// inflight tracks cancellable in-flight AI queries (TR-05a).
	inflightMu sync.Mutex
	inflight   map[string]context.CancelFunc
}

func (d *daemon) loadConfig() error {
	cfg, err := config.Load(d.configPath)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.cfg = cfg
	d.cmdRegistry = commands.NewRegistry(&cfg.Commands)
	d.mu.Unlock()
	return nil
}

func (d *daemon) initDB() error {
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".local", "share", "vida", "history.db")
	_ = os.MkdirAll(filepath.Dir(dbPath), 0700)
	database, err := db.Open(dbPath)
	if err != nil {
		return err
	}
	d.database = database
	d.clipStore = clipboard.NewStore(database)
	return nil
}

func (d *daemon) rebuildRouter() {
	d.mu.RLock()
	cfg := d.cfg
	appIndex := d.appIndex
	fileIndex := d.fileIndex
	d.mu.RUnlock()

	var providerName string
	var provider ai.AIProvider

	if cfg != nil {
		providerName = cfg.AI.Provider
	}
	if providerName == "" {
		providerName = "claude"
	}

	switch providerName {
	case "openai":
		apiKey := ""
		baseURL := ""
		if cfg != nil {
			apiKey = cfg.EffectiveOpenAIKey()
			baseURL = cfg.AI.OpenAI.BaseURL
		}
		provider = ai.NewOpenAIProvider(ai.OpenAIConfig{APIKey: apiKey, BaseURL: baseURL})
	default: // "claude"
		apiKey := ""
		baseURL := ""
		if cfg != nil {
			apiKey = cfg.EffectiveClaudeKey()
			baseURL = cfg.AI.Claude.BaseURL
		}
		provider = ai.NewClaudeProvider(ai.ClaudeConfig{APIKey: apiKey, BaseURL: baseURL})
	}

	var shortcutMap map[string]string
	if cfg != nil && len(cfg.Search.Shortcuts) > 0 {
		shortcutMap = cfg.Search.Shortcuts
	}

	// AI func is a placeholder for the router; actual streaming is done in handleQuery.
	aiFunc := func(ctx context.Context, input string) router.Result {
		return router.Result{Kind: router.KindAIStream}
	}

	var opts []router.Option
	if shortcutMap != nil {
		opts = append(opts, router.WithShortcuts(shortcutMap))
	} else {
		opts = append(opts, router.WithShortcuts(shortcuts.DefaultShortcuts()))
	}
	if appIndex != nil {
		opts = append(opts, router.WithAppIndex(appIndex))
	}
	if fileIndex != nil {
		opts = append(opts, router.WithFileIndex(fileIndex))
	}
	opts = append(opts, router.WithAIFunc(aiFunc))

	d.mu.Lock()
	d.provider = provider
	d.rtr = router.New(opts...)
	d.mu.Unlock()
}

func (d *daemon) indexApps() {
	dirs := apps.DefaultDirs()
	if override := os.Getenv("VIDA_APPS_DIRS"); override != "" {
		dirs = strings.Split(override, ":")
	}
	idx, err := apps.BuildIndex(dirs)
	if err != nil {
		log.Printf("app indexing failed: %v", err)
		return
	}
	log.Printf("vida-daemon: indexed %d apps", idx.Len())
	d.mu.Lock()
	d.appIndex = idx
	d.mu.Unlock()
	d.rebuildRouter()
}

func (d *daemon) indexFiles() {
	d.mu.RLock()
	cfg := d.cfg
	d.mu.RUnlock()

	dirs := []string{}
	includeHidden := false
	if cfg != nil && len(cfg.Files.Dirs) > 0 {
		dirs = cfg.Files.Dirs
		includeHidden = cfg.Files.IncludeHidden
	} else {
		home, _ := os.UserHomeDir()
		dirs = []string{home}
	}

	idx, err := files.BuildIndex(dirs, includeHidden)
	if err != nil {
		log.Printf("file indexing failed: %v", err)
		return
	}
	log.Printf("vida-daemon: indexed %d files", idx.Len())
	d.mu.Lock()
	d.fileIndex = idx
	d.mu.Unlock()
	d.rebuildRouter()
}

func (d *daemon) handleMessage(msg ipc.Message, reply ipc.ReplyFunc) {
	switch msg.Type {
	case "ping":
		// handled by ipc built-in dispatch
	case "status":
		d.mu.RLock()
		providerName := "claude"
		if d.provider != nil {
			providerName = d.provider.Name()
		}
		d.mu.RUnlock()
		_ = reply(ipc.Message{
			Type:     "ok",
			PID:      d.pid,
			Provider: providerName,
		})
	case "reload":
		if err := d.loadConfig(); err != nil {
			log.Printf("reload: config error: %v", err)
		}
		d.rebuildRouter()
		go d.indexFiles()
		_ = reply(ipc.Message{Type: "ok"})
	case "query":
		d.handleQuery(msg, reply)
	case "run_command":
		d.handleRunCommand(msg, reply)
	case "cancel":
		d.cancelInflight(msg.ID)
		_ = reply(ipc.Message{Type: "ok", ID: msg.ID})
	case "clipboard_list":
		d.handleClipboardList(msg, reply)
	case "clipboard_paste":
		d.handleClipboardPaste(msg, reply)
	case "clipboard_delete":
		d.handleClipboardDelete(msg, reply)
	case "clipboard_pin":
		d.handleClipboardPin(msg, reply)
	case "clipboard_clear":
		d.handleClipboardClear(msg, reply)
	}
}

// cancelInflight cancels the in-flight AI query with the given ID (TR-05b).
func (d *daemon) cancelInflight(id string) {
	if id == "" {
		return
	}
	d.inflightMu.Lock()
	cancel, ok := d.inflight[id]
	if ok {
		delete(d.inflight, id)
	}
	d.inflightMu.Unlock()
	if ok {
		cancel()
	}
}

func (d *daemon) handleQuery(msg ipc.Message, reply ipc.ReplyFunc) {
	d.mu.RLock()
	rtr := d.rtr
	provider := d.provider
	d.mu.RUnlock()

	// Cancel any previous in-flight query with the same ID (FR-06d).
	d.cancelInflight(msg.ID)

	ctx := context.Background()
	result := rtr.Route(ctx, msg.Input)

	switch result.Kind {
	case router.KindAIStream:
		d.streamAI(msg.ID, msg.Input, provider, reply)
	case router.KindCommandList:
		// :cb / :clipboard → open clipboard window (TR-05, FR-03a).
		if result.CommandQuery == "cb" || result.CommandQuery == "clipboard" {
			_ = reply(ipc.Message{Type: "result", ID: msg.ID, Kind: "show_clipboard"})
			return
		}

		d.mu.RLock()
		reg := d.cmdRegistry
		d.mu.RUnlock()
		if reg == nil {
			reg = commands.NewRegistry(nil)
		}
		// Only filter by the first word; rest is the command's argument input.
		filterQuery := result.CommandQuery
		if i := strings.IndexByte(filterQuery, ' '); i >= 0 {
			filterQuery = filterQuery[:i]
		}
		matched := reg.Filter(filterQuery)
		type cmdJSON struct {
			Name string `json:"name"`
			Desc string `json:"desc"`
			Icon string `json:"icon"`
			Kind string `json:"kind"`
		}
		items := make([]cmdJSON, len(matched))
		for i, c := range matched {
			items[i] = cmdJSON{Name: c.Name, Desc: c.Desc, Icon: c.Icon, Kind: c.Kind}
		}
		b, _ := json.Marshal(items)
		_ = reply(ipc.Message{
			Type:    "result",
			ID:      msg.ID,
			Kind:    "command_list",
			Message: string(b),
		})
	default:
		resp := ipc.Message{
			Type: "result",
			ID:   msg.ID,
			Kind: string(result.Kind),
		}
		switch result.Kind {
		case router.KindCalc, router.KindConvert:
			resp.Value = result.CalcValue
		case router.KindShortcut:
			resp.URL = result.ShortcutURL
		case router.KindAppList:
			names := make([]string, len(result.Apps))
			ids := make([]string, len(result.Apps))
			execs := make([]string, len(result.Apps))
			icons := make([]string, len(result.Apps))
			for i, a := range result.Apps {
				names[i] = a.Name
				ids[i] = a.ID
				execs[i] = apps.ExpandExec(a.Exec)
				icons[i] = a.Icon
			}
			resp.Message = strings.Join(names, "\n")
			resp.IDs = strings.Join(ids, "\n")
			resp.Exec = strings.Join(execs, "\n")
			resp.Icons = strings.Join(icons, "\n")
		case router.KindFileList:
			names := make([]string, len(result.Files))
			paths := make([]string, len(result.Files))
			icons := make([]string, len(result.Files))
			for i, f := range result.Files {
				names[i] = f.Name
				paths[i] = f.Path
				icons[i] = f.Icon
			}
			resp.Message = strings.Join(names, "\n")
			resp.Paths = strings.Join(paths, "\n")
			resp.Icons = strings.Join(icons, "\n")
		}
		_ = reply(resp)
	}
}

// handleRunCommand executes a named command and replies with the appropriate message.
func (d *daemon) handleRunCommand(msg ipc.Message, reply ipc.ReplyFunc) {
	d.mu.RLock()
	reg := d.cmdRegistry
	provider := d.provider
	d.mu.RUnlock()

	if reg == nil {
		reg = commands.NewRegistry(nil)
	}

	cmd, ok := reg.Get(msg.Name)
	if !ok {
		_ = reply(ipc.Message{Type: "command_error", ID: msg.ID, Message: "unknown command: " + msg.Name})
		return
	}

	// reload-vida: send an internal reload instead of running a shell command.
	if msg.Name == "reload-vida" {
		if err := d.loadConfig(); err != nil {
			log.Printf("reload-vida: config error: %v", err)
		}
		d.rebuildRouter()
		_ = reply(ipc.Message{Type: "command_done", ID: msg.ID})
		return
	}

	// Note command is handled client-side; daemon just acknowledges.
	if cmd.Kind == commands.KindNote {
		_ = reply(ipc.Message{Type: "command_done", ID: msg.ID})
		return
	}

	// AI commands: stream via provider with injected system prompt.
	if cmd.Kind == commands.KindAI {
		input := cmd.SystemPrompt + "\n\n" + msg.Input
		d.streamAIWithHistory(msg.ID, input, msg.History, provider, reply)
		return
	}

	// Shell commands.
	execStr := commands.ExpandInput(cmd.Exec, msg.Input)

	if cmd.Output == "none" {
		// Fire and forget.
		c := exec.Command("sh", "-c", execStr)
		_ = c.Start()
		_ = reply(ipc.Message{Type: "command_done", ID: msg.ID})
		return
	}

	// output=palette: capture stdout, timeout 10s.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "sh", "-c", execStr)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		_ = reply(ipc.Message{Type: "command_error", ID: msg.ID, Message: errMsg})
		return
	}
	out := strings.TrimSpace(stdout.String())
	_ = reply(ipc.Message{Type: "command_result", ID: msg.ID, Value: out})
}

// --- Clipboard IPC handlers (TR-05, SPEC-20260318-011) ---

func (d *daemon) clipCfg() clipboard.Config {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.cfg == nil {
		return clipboard.Config{Enabled: true, MaxEntries: 500, MaxAgeDays: 30}
	}
	c := d.cfg.Clipboard
	return clipboard.Config{
		Enabled:    c.Enabled,
		MaxEntries: c.MaxEntries,
		MaxAgeDays: c.MaxAgeDays,
		Ignore:     c.Ignore,
	}
}

func (d *daemon) handleClipboardList(msg ipc.Message, reply ipc.ReplyFunc) {
	entries, err := d.clipStore.List(msg.Input, 200)
	if err != nil {
		_ = reply(ipc.Message{Type: "error", Message: err.Error()})
		return
	}
	contents := make([]string, len(entries))
	ids := make([]string, len(entries))
	pinned := make([]string, len(entries))
	for i, e := range entries {
		contents[i] = e.Content
		ids[i] = fmt.Sprintf("%d", e.ID)
		if e.Pinned {
			pinned[i] = "1"
		} else {
			pinned[i] = "0"
		}
	}
	_ = reply(ipc.Message{
		Type:  "clipboard_entries",
		Value: strings.Join(contents, "\n"),
		IDs:   strings.Join(ids, "\n"),
		Icons: strings.Join(pinned, "\n"),
	})
}

func (d *daemon) handleClipboardPaste(msg ipc.Message, reply ipc.ReplyFunc) {
	entries, err := d.clipStore.List("", 1000)
	if err != nil {
		_ = reply(ipc.Message{Type: "error", Message: err.Error()})
		return
	}
	var content string
	for _, e := range entries {
		if fmt.Sprintf("%d", e.ID) == msg.ID {
			content = e.Content
			break
		}
	}
	if content == "" || content == "[image]" {
		_ = reply(ipc.Message{Type: "ok"})
		return
	}
	cmd := exec.Command("wl-copy", "--", content)
	if err := cmd.Run(); err != nil {
		log.Printf("clipboard paste: wl-copy: %v", err)
	}
	_ = reply(ipc.Message{Type: "ok"})
}

func (d *daemon) handleClipboardDelete(msg ipc.Message, reply ipc.ReplyFunc) {
	var id int64
	fmt.Sscanf(msg.ID, "%d", &id)
	if err := d.clipStore.Delete(id); err != nil {
		_ = reply(ipc.Message{Type: "error", Message: err.Error()})
		return
	}
	_ = reply(ipc.Message{Type: "ok"})
}

func (d *daemon) handleClipboardPin(msg ipc.Message, reply ipc.ReplyFunc) {
	var id int64
	fmt.Sscanf(msg.ID, "%d", &id)
	if err := d.clipStore.TogglePin(id); err != nil {
		_ = reply(ipc.Message{Type: "error", Message: err.Error()})
		return
	}
	_ = reply(ipc.Message{Type: "ok"})
}

func (d *daemon) handleClipboardClear(msg ipc.Message, reply ipc.ReplyFunc) {
	if err := d.clipStore.Clear(); err != nil {
		_ = reply(ipc.Message{Type: "error", Message: err.Error()})
		return
	}
	_ = reply(ipc.Message{Type: "ok"})
}

// watchClipboard spawns wl-paste --watch and stores each new clipboard entry (FR-01a).
// It automatically restarts if the wl-paste process exits unexpectedly.
func (d *daemon) watchClipboard(ctx context.Context, srv *ipc.Server) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := d.runClipboardWatcher(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("clipboard watcher: %v; restarting in 2s", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (d *daemon) runClipboardWatcher(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "wl-paste", "--watch", "cat")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("wl-paste not available: %w", err)
	}
	log.Println("clipboard watcher: started")

	scanner := bufio.NewScanner(stdout)
	// Allow up to 1MB clipboard entries.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if ctx.Err() != nil {
			break
		}
		content := scanner.Text()
		if content == "" {
			continue
		}
		cfg := d.clipCfg()
		if _, err := d.clipStore.Add(content, cfg); err != nil {
			log.Printf("clipboard watcher: store: %v", err)
		}
	}
	err = cmd.Wait()
	log.Println("clipboard watcher: stopped")
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("wl-paste exited: %w", err)
	}
	return fmt.Errorf("wl-paste exited unexpectedly")
}

// streamAI calls the AI provider and sends token+done messages over reply (TR-02c).
func (d *daemon) streamAI(id, input string, provider ai.AIProvider, reply ipc.ReplyFunc) {
	d.streamAIWithHistory(id, input, nil, provider, reply)
}

func (d *daemon) streamAIWithHistory(id, input string, history []ipc.HistoryEntry, provider ai.AIProvider, reply ipc.ReplyFunc) {
	if provider == nil {
		_ = reply(ipc.Message{Type: "done", ID: id})
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Register for external cancellation (TR-05a).
	d.inflightMu.Lock()
	d.inflight[id] = cancel
	d.inflightMu.Unlock()

	// Convert IPC history to AI provider history.
	aiHistory := make([]ai.Message, len(history))
	for i, h := range history {
		aiHistory[i] = ai.Message{Role: h.Role, Content: h.Content}
	}

	go func() {
		defer func() {
			cancel()
			d.inflightMu.Lock()
			delete(d.inflight, id)
			d.inflightMu.Unlock()
		}()

		ch, err := provider.Query(ctx, input, aiHistory)
		if err != nil {
			_ = reply(ipc.Message{Type: "done", ID: id})
			return
		}

		for tok := range ch {
			if ctx.Err() != nil {
				_ = reply(ipc.Message{Type: "cancelled", ID: id})
				return
			}
			if tok.Error != nil {
				_ = reply(ipc.Message{Type: "done", ID: id})
				return
			}
			if tok.Done {
				break
			}
			_ = reply(ipc.Message{Type: "token", ID: id, Value: tok.Text})
		}
		_ = reply(ipc.Message{Type: "done", ID: id})
	}()
}
