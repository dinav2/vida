// vida-ui is the persistent Wayland layer-shell window for vida.
// It subscribes to IPC broadcasts from vida-daemon and shows/hides on command.
// On each keystroke it sends a query to the daemon and renders the result.
package main

/*
#cgo pkg-config: gtk4 gtk4-layer-shell-0 gio-unix-2.0
#include <gtk/gtk.h>

// Declarations for functions implemented in ui.c.
extern void       vida_on_activate(GtkApplication *app, gpointer data);
extern GtkWidget *vida_build_window(GtkApplication *app,
                                    GtkWidget **out_entry,
                                    GtkWidget **out_results);
extern void       vida_show(GtkWidget *w);
extern void       vida_hide(GtkWidget *w);
extern void       vida_entry_clear(GtkWidget *entry);
extern void       vida_entry_get_text(GtkWidget *entry, char *buf, int buflen);
extern void       vida_results_clear(GtkWidget *box);
extern void       vida_results_set_label(GtkWidget *box, const char *text);
extern void       vida_results_set_ai_text(GtkWidget *box, const char *text);
extern void       vida_results_append_text(GtkWidget *box, const char *text);
extern void       vida_results_set_url(GtkWidget *box, const char *url);
extern void       vida_results_set_apps(GtkWidget *box,
                                        const char **names, const char **icons, int n);
extern void       vida_grab_focus(GtkWidget *entry);
extern void       vida_select_row(GtkWidget *box, int idx);
extern int        vida_count_rows(GtkWidget *box);
extern void       vida_launch_app(const char *desktop_id);
extern void       vida_copy_to_clipboard(GtkWidget *widget, const char *text);
extern void       vida_open_url(const char *url);
*/
import "C"

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/dinav2/vida/internal/debounce"
	"github.com/dinav2/vida/internal/ipc"
)

// --- global UI state (only touched from GLib main thread or via gtkIdle) ---

var gWindow *C.GtkWidget
var gEntry *C.GtkWidget
var gResults *C.GtkWidget

// --- result selection state (only touched via gtkIdle) ---

var selectedIdx = -1
var currentKind string
var currentAppIDs []string
var currentAppExecs []string
var currentCalcValue string
var currentURL string
var currentAIText string

// --- query state (goroutine-safe) ---

var (
	querySeq     atomic.Uint64      // monotonically increasing, identifies current query
	inflightID   string             // IPC query ID for in-flight AI
	inflightMu   sync.Mutex
	aiDebounce   *debounce.Timer
	aiConn       *ipc.PersistentConn
	aiConnMu     sync.Mutex
)

func main() {
	cname := C.CString("io.vida.ui")
	defer C.free(unsafe.Pointer(cname))

	app := C.gtk_application_new(cname, C.G_APPLICATION_DEFAULT_FLAGS)
	defer C.g_object_unref(C.gpointer(app))

	activateStr := C.CString("activate")
	defer C.free(unsafe.Pointer(activateStr))

	C.g_signal_connect_data(
		C.gpointer(app),
		activateStr,
		C.GCallback(C.vida_on_activate),
		nil, nil, 0,
	)

	os.Exit(int(C.g_application_run((*C.GApplication)(unsafe.Pointer(app)), 0, nil)))
}

//export goOnActivate
func goOnActivate(app *C.GtkApplication, userData C.gpointer) {
	_ = userData
	var entry, results *C.GtkWidget
	win := C.vida_build_window(app, &entry, &results)
	gWindow = win
	gEntry = entry
	gResults = results

	aiDebounce = debounce.New(80*time.Millisecond, fireAIQuery)

	go subscribeLoop(sockFile())
}

//export goOnKeyPressed
func goOnKeyPressed(ctrl *C.GtkEventControllerKey, keyval C.guint,
	keycode C.guint, state C.GdkModifierType, userData C.gpointer) C.gboolean {
	_, _, _ = ctrl, keycode, state
	if keyval == C.GDK_KEY_Escape {
		cancelInflight()
		C.vida_hide((*C.GtkWidget)(unsafe.Pointer(userData)))
		return C.TRUE
	}
	if keyval == C.GDK_KEY_Down {
		n := int(C.vida_count_rows(gResults))
		if n > 0 {
			selectedIdx = (selectedIdx + 1) % n
			C.vida_select_row(gResults, C.int(selectedIdx))
		}
		return C.TRUE
	}
	if keyval == C.GDK_KEY_Up {
		n := int(C.vida_count_rows(gResults))
		if n > 0 {
			if selectedIdx <= 0 {
				selectedIdx = n - 1
			} else {
				selectedIdx--
			}
			C.vida_select_row(gResults, C.int(selectedIdx))
		}
		return C.TRUE
	}
	if keyval == C.GDK_KEY_Return || keyval == C.GDK_KEY_KP_Enter {
		win := (*C.GtkWidget)(unsafe.Pointer(userData))
		switch currentKind {
		case "app_list":
			idx := selectedIdx
			if idx < 0 {
				idx = 0
			}
			if idx < len(currentAppIDs) {
				id := currentAppIDs[idx]
				cid := C.CString(id)
				C.vida_launch_app(cid)
				C.free(unsafe.Pointer(cid))
				C.vida_hide(win)
			}
		case "calc":
			cv := C.CString(currentCalcValue)
			C.vida_copy_to_clipboard(gEntry, cv)
			C.free(unsafe.Pointer(cv))
		case "shortcut":
			cu := C.CString(currentURL)
			C.vida_open_url(cu)
			C.free(unsafe.Pointer(cu))
		case "ai_stream":
			ca := C.CString(currentAIText)
			C.vida_copy_to_clipboard(gEntry, ca)
			C.free(unsafe.Pointer(ca))
		}
		return C.TRUE
	}
	return C.FALSE
}

//export goOnEntryChanged
func goOnEntryChanged(entry *C.GtkEntry, userData C.gpointer) {
	_ = userData
	var buf [512]C.char
	C.vida_entry_get_text((*C.GtkWidget)(unsafe.Pointer(entry)), &buf[0], 512)
	text := C.GoString(&buf[0])
	onInput(text)
}

// onInput is called on every keystroke. Routes immediately for non-AI kinds,
// debounces for AI (FR-01b–d).
func onInput(text string) {
	aiDebounce.Stop()
	cancelInflight()

	if text == "" {
		gtkIdle(func() {
			C.vida_results_clear(gResults)
			selectedIdx = -1
			currentKind = ""
		})
		return
	}

	sock := sockFile()
	seq := querySeq.Add(1)
	id := fmt.Sprintf("q-%d", seq)

	// Send query immediately (daemon routes it).
	go func() {
		conn, err := ipc.Connect(sock)
		if err != nil {
			return
		}
		defer conn.Close()

		resp, err := conn.Send(ipc.Message{Type: "query", ID: id, Input: text})
		if err != nil {
			return
		}

		switch resp.Kind {
		case "calc":
			val := resp.Value
			gtkIdle(func() {
				currentKind = "calc"
				currentCalcValue = val
				selectedIdx = -1
				C.vida_results_set_label(gResults, C.CString(val))
			})
		case "shortcut":
			url := resp.URL
			gtkIdle(func() {
				currentKind = "shortcut"
				currentURL = url
				selectedIdx = -1
				C.vida_results_set_url(gResults, C.CString(url))
			})
		case "app_list":
			names := strings.Split(resp.Message, "\n")
			ids := strings.Split(resp.IDs, "\n")
			execs := strings.Split(resp.Exec, "\n")
			Icons := strings.Split(resp.Icons, "\n")
			gtkIdle(func() {
				currentKind = "app_list"
				currentAppIDs = ids
				currentAppExecs = execs
				selectedIdx = -1
				cnames := make([]*C.char, len(names))
				cicons := make([]*C.char, len(names))
				for i, n := range names {
					cnames[i] = C.CString(n)
					defer C.free(unsafe.Pointer(cnames[i]))
					icon := ""
					if i < len(Icons) {
						icon = Icons[i]
					}
					cicons[i] = C.CString(icon)
					defer C.free(unsafe.Pointer(cicons[i]))
				}
				C.vida_results_set_apps(gResults, &cnames[0], &cicons[0], C.int(len(cnames)))
			})
		case "ai_stream":
			gtkIdle(func() {
				currentKind = "ai_stream"
				currentAIText = ""
				selectedIdx = -1
			})
			inflightMu.Lock()
			inflightID = id
			inflightMu.Unlock()
			aiDebounce = debounce.New(80*time.Millisecond, func() {
				go streamAI(id, text, sock)
			})
			aiDebounce.Trigger()
		case "empty", "cancelled", "":
			gtkIdle(func() {
				currentKind = ""
				selectedIdx = -1
				C.vida_results_clear(gResults)
			})
		}
	}()
}

// fireAIQuery is called by the debounce timer after 80 ms idle.
func fireAIQuery() {
	// The actual AI streaming is started in onInput when kind==ai_stream.
}

// streamAI opens a persistent connection and streams AI tokens into the results.
func streamAI(id, input, sock string) {
	conn, err := ipc.ConnectPersistent(sock)
	if err != nil {
		return
	}
	aiConnMu.Lock()
	aiConn = conn
	aiConnMu.Unlock()
	defer func() {
		conn.Close()
		aiConnMu.Lock()
		if aiConn == conn {
			aiConn = nil
		}
		aiConnMu.Unlock()
	}()

	if err := conn.SendNoReply(ipc.Message{Type: "query", ID: id, Input: input}); err != nil {
		return
	}

	var accumulated strings.Builder
	for {
		msg, err := conn.Recv(30 * time.Second)
		if err != nil {
			return
		}
		inflightMu.Lock()
		current := inflightID
		inflightMu.Unlock()
		if current != id {
			return // superseded
		}
		switch msg.Type {
		case "token":
			accumulated.WriteString(msg.Value)
			text := accumulated.String()
			gtkIdle(func() {
				currentAIText = text
				C.vida_results_set_ai_text(gResults, C.CString(text))
			})
		case "done", "cancelled":
			return
		}
	}
}

// cancelInflight sends a cancel message for the current in-flight AI query.
func cancelInflight() {
	inflightMu.Lock()
	id := inflightID
	inflightID = ""
	inflightMu.Unlock()
	if id == "" {
		return
	}
	go func() {
		conn, err := ipc.Connect(sockFile())
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Send(ipc.Message{Type: "cancel", ID: id})
	}()
	aiConnMu.Lock()
	c := aiConn
	aiConnMu.Unlock()
	if c != nil {
		c.Close()
	}
}

// subscribeLoop maintains a persistent IPC connection for show/hide broadcasts.
func subscribeLoop(sockPath string) {
	for {
		if err := subscribe(sockPath); err != nil {
			if !ipc.IsDaemonNotRunning(err) {
				log.Printf("vida-ui: %v", err)
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func subscribe(sockPath string) error {
	conn, err := ipc.ConnectPersistent(sockPath)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := conn.SendNoReply(ipc.Message{Type: "subscribe"}); err != nil {
		return err
	}

	for {
		msg, err := conn.Recv(30 * time.Second)
		if err != nil {
			return err
		}
		switch msg.Type {
		case "show":
			cancelInflight()
			gtkIdle(func() {
				C.vida_entry_clear(gEntry)
				C.vida_results_clear(gResults)
				selectedIdx = -1
				currentKind = ""
				C.vida_show(gWindow)
				C.vida_grab_focus(gEntry)
			})
		case "hide":
			cancelInflight()
			gtkIdle(func() { C.vida_hide(gWindow) })
		}
	}
}

// gtkIdle schedules fn to run on the GLib main thread.
var idleQueue = make(chan func(), 64)

func gtkIdle(fn func()) {
	select {
	case idleQueue <- fn:
	default:
	}
}

// goProcessIdle is called periodically by a GLib timeout to drain idleQueue.
// Exported so ui.c can call it from a g_timeout_add callback.
//
//export goProcessIdle
func goProcessIdle(_ C.gpointer) C.gboolean {
	for {
		select {
		case fn := <-idleQueue:
			fn()
		default:
			return C.TRUE // keep the timeout running
		}
	}
}

func sockFile() string {
	if s := os.Getenv("VIDA_SOCKET"); s != "" {
		return s
	}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = filepath.Join("/run/user", "1000")
	}
	return filepath.Join(runtimeDir, "vida.sock")
}

// Suppress unused import warnings for packages only used via CGo indirectly.
var _ = context.Background
