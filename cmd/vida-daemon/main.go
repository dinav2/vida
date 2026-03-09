// vida-daemon is the always-running background process for vida.
// It manages IPC, app indexing, query routing, and AI provider calls.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/dinav2/vida/internal/ai"
	"github.com/dinav2/vida/internal/apps"
	"github.com/dinav2/vida/internal/config"
	"github.com/dinav2/vida/internal/db"
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

	d.rebuildRouter()
	go d.indexApps()

	srv, err := ipc.Listen(sockPath)
	if err != nil {
		return fmt.Errorf("ipc listen: %w", err)
	}
	defer srv.Close()

	srv.Handle(d.handleMessage)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go srv.Serve(ctx)

	log.Printf("vida-daemon: socket ready at %s (pid %d)", sockPath, d.pid)
	<-ctx.Done()
	log.Println("vida-daemon: shutting down")
	return nil
}

type daemon struct {
	configPath string
	sockPath   string
	pid        int

	mu       sync.RWMutex
	cfg      *config.Config
	provider ai.AIProvider
	rtr      *router.Router
	appIndex *apps.Index
	database *db.DB

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
	return nil
}

func (d *daemon) rebuildRouter() {
	d.mu.RLock()
	cfg := d.cfg
	appIndex := d.appIndex
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
		_ = reply(ipc.Message{Type: "ok"})
	case "query":
		d.handleQuery(msg, reply)
	case "cancel":
		d.cancelInflight(msg.ID)
		_ = reply(ipc.Message{Type: "ok", ID: msg.ID})
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
	default:
		resp := ipc.Message{
			Type: "result",
			ID:   msg.ID,
			Kind: string(result.Kind),
		}
		switch result.Kind {
		case router.KindCalc:
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
		}
		_ = reply(resp)
	}
}

// streamAI calls the AI provider and sends token+done messages over reply (TR-02c).
func (d *daemon) streamAI(id, input string, provider ai.AIProvider, reply ipc.ReplyFunc) {
	if provider == nil {
		_ = reply(ipc.Message{Type: "done", ID: id})
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Register for external cancellation (TR-05a).
	d.inflightMu.Lock()
	d.inflight[id] = cancel
	d.inflightMu.Unlock()

	go func() {
		defer func() {
			cancel()
			d.inflightMu.Lock()
			delete(d.inflight, id)
			d.inflightMu.Unlock()
		}()

		ch, err := provider.Query(ctx, input, nil)
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
