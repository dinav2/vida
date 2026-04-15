// vida is the CLI management tool for vida-daemon.
// Commands: show, hide, reload, clear-history, ping, status
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dinav2/vida/internal/ipc"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "vida: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: vida <command>\nCommands: show, hide, reload, clear-history, clipboard, ping, status")
	}

	sockPath := sockFile()

	cmd := args[0]
	switch cmd {
	case "show", "hide":
		return sendAndExpect(sockPath, ipc.Message{Type: cmd}, "ok")
	case "reload":
		return sendAndExpect(sockPath, ipc.Message{Type: "reload"}, "ok")
	case "clear-history":
		resp, err := sendMsg(sockPath, ipc.Message{Type: "clear_history"})
		if err != nil {
			return err
		}
		if resp.Type == "error" {
			return fmt.Errorf("daemon error: %s", resp.Message)
		}
		fmt.Println("History cleared.")
		return nil
	case "ping":
		resp, err := sendMsg(sockPath, ipc.Message{Type: "ping"})
		if err != nil {
			if ipc.IsDaemonNotRunning(err) {
				return err
			}
			return fmt.Errorf("ping failed: %w", err)
		}
		if resp.Type == "pong" {
			fmt.Println("pong")
			return nil
		}
		return fmt.Errorf("unexpected response: %s", resp.Type)
	case "clipboard":
		return sendAndExpect(sockPath, ipc.Message{Type: "show_clipboard"}, "ok")
	case "status":
		resp, err := sendMsg(sockPath, ipc.Message{Type: "status"})
		if err != nil {
			return err
		}
		fmt.Printf("pid:      %d\n", resp.PID)
		fmt.Printf("provider: %s\n", resp.Provider)
		return nil
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func sockFile() string {
	if s := os.Getenv("VIDA_SOCKET"); s != "" {
		return s
	}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	return filepath.Join(runtimeDir, "vida.sock")
}

func sendMsg(sockPath string, msg ipc.Message) (ipc.Message, error) {
	conn, err := ipc.Connect(sockPath)
	if err != nil {
		return ipc.Message{}, err
	}
	defer conn.Close()
	return conn.Send(msg)
}

func sendAndExpect(sockPath string, msg ipc.Message, wantType string) error {
	resp, err := sendMsg(sockPath, msg)
	if err != nil {
		return err
	}
	if resp.Type == "error" {
		return fmt.Errorf("daemon error: %s", resp.Message)
	}
	if resp.Type != wantType {
		return fmt.Errorf("unexpected response type %q, want %q", resp.Type, wantType)
	}
	return nil
}
