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
	// Determine config path
	configPath := os.Getenv("VIDA_CONFIG")
	if configPath == "" {
		home, _ := os.UserHomeDir()
		configPath = filepath.Join(home, ".config", "vida", "config.toml")
	}

	// Determine socket path
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
	database *db.DB
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
		var apiKey string
		if cfg != nil {
			apiKey = cfg.EffectiveOpenAIKey()
		}
		provider = ai.NewOpenAIProvider(ai.OpenAIConfig{APIKey: apiKey})
	default: // "claude"
		var apiKey string
		if cfg != nil {
			apiKey = cfg.EffectiveClaudeKey()
		}
		provider = ai.NewClaudeProvider(ai.ClaudeConfig{APIKey: apiKey})
	}

	var shortcutMap map[string]string
	if cfg != nil && len(cfg.Search.Shortcuts) > 0 {
		shortcutMap = cfg.Search.Shortcuts
	}

	aiFunc := func(ctx context.Context, input string) router.Result {
		ch, err := provider.Query(ctx, input, nil)
		if err != nil {
			return router.Result{Kind: router.KindAIStream}
		}
		_ = ch
		return router.Result{Kind: router.KindAIStream}
	}

	var opts []router.Option
	if shortcutMap != nil {
		opts = append(opts, router.WithShortcuts(shortcutMap))
	} else {
		opts = append(opts, router.WithShortcuts(shortcuts.DefaultShortcuts()))
	}
	opts = append(opts, router.WithAIFunc(aiFunc))

	d.mu.Lock()
	d.provider = provider
	d.rtr = router.New(opts...)
	d.mu.Unlock()
}

func (d *daemon) indexApps() {
	dirs := apps.DefaultDirs()
	idx, err := apps.BuildIndex(dirs)
	if err != nil {
		log.Printf("app indexing failed: %v", err)
		return
	}
	d.mu.Lock()
	// Rebuild router with app index
	d.mu.Unlock()
	log.Printf("vida-daemon: indexed apps from %v", dirs)
	_ = idx
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
	}
}

func (d *daemon) handleQuery(msg ipc.Message, reply ipc.ReplyFunc) {
	d.mu.RLock()
	rtr := d.rtr
	d.mu.RUnlock()

	ctx := context.Background()
	result := rtr.Route(ctx, msg.Input)

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
	}
	_ = reply(resp)
}
