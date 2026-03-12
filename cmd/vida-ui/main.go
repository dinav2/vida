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
                                    GtkWidget **out_results,
                                    GtkWidget **out_answer);
extern void       vida_show(GtkWidget *w);
extern void       vida_hide(GtkWidget *w);
extern void       vida_entry_clear(GtkWidget *entry);
extern void       vida_entry_get_text(GtkWidget *entry, char *buf, int buflen);
extern void       vida_results_clear(GtkWidget *box);
extern void       vida_results_set_label(GtkWidget *box, const char *text);
extern void       vida_results_set_convert(GtkWidget *box, const char *text);
extern void       vida_answer_set(GtkWidget *answer, const char *value, const char *type);
extern void       vida_answer_clear(GtkWidget *answer);
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
extern void       vida_results_set_commands(GtkWidget *box,
                                            const char **names, const char **descs,
                                            const char **icons, int n);
extern void       vida_show_copied_hud(GtkWidget *box);
extern void       vida_entry_set_placeholder(GtkWidget *entry, const char *text);

// Chat view.
extern void vida_chat_show(const char *cmd_name);
extern void vida_chat_clear(void);
extern void vida_chat_append_message(const char *role, const char *text);
extern void vida_chat_update_last_ai(const char *text);
extern void vida_chat_set_entry_sensitive(gboolean sensitive);
extern void vida_chat_entry_get_text(char *buf, int buflen);
extern void vida_chat_entry_clear(void);

// Note form.
extern void vida_note_show(const char *prefill_title);
extern void vida_note_get_title(char *buf, int len);
extern void vida_note_get_body(char *buf, int len);
extern void vida_note_get_tags(char *buf, int len);

// Layer shell mode switching.
extern void vida_enter_chat_mode(GtkWidget *win);
extern void vida_enter_palette_mode(GtkWidget *win);
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/dinav2/vida/internal/config"
	"github.com/dinav2/vida/internal/debounce"
	"github.com/dinav2/vida/internal/ipc"
	"github.com/dinav2/vida/internal/notes"
)

var jsonUnmarshal = json.Unmarshal

// --- global UI state (only touched from GLib main thread or via gtkIdle) ---

var gWindow *C.GtkWidget
var gEntry *C.GtkWidget
var gResults *C.GtkWidget
var gAnswer *C.GtkWidget

// --- result selection state (only touched via gtkIdle) ---

var selectedIdx = -1
var currentKind string
var currentAppIDs []string
var currentAppExecs []string
var currentCalcValue string
var currentURL string
var currentAIText string
var currentResultText string // generic copyable text for Ctrl+C

// Command mode state
var currentCmdNames []string // names of displayed commands (parallel arrays)
var currentCmdKinds []string // kinds parallel to currentCmdNames
var currentCmdQuery string   // text after ":" used to run selected command

// Chat view state (only touched via gtkIdle)
type chatMsg struct {
	role    string // "user" | "assistant"
	content string
}

var chatHistory []chatMsg  // message history for multi-turn (FR-04b)
var chatCmdName string     // command name that started the session

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

	app := C.gtk_application_new(cname, C.G_APPLICATION_NON_UNIQUE)
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
	var entry, results, answer *C.GtkWidget
	win := C.vida_build_window(app, &entry, &results, &answer)
	gWindow = win
	gEntry = entry
	gResults = results
	gAnswer = answer

	aiDebounce = debounce.New(80*time.Millisecond, fireAIQuery)

	go subscribeLoop(sockFile())
}

//export goOnKeyPressed
func goOnKeyPressed(ctrl *C.GtkEventControllerKey, keyval C.guint,
	keycode C.guint, state C.GdkModifierType, userData C.gpointer) C.gboolean {
	_, _ = ctrl, keycode
	// Ctrl+C — copy current result text to clipboard (FR-07a).
	if state&C.GDK_CONTROL_MASK != 0 && keyval == C.GDK_KEY_c {
		if currentResultText != "" {
			ct := C.CString(currentResultText)
			C.vida_copy_to_clipboard(gEntry, ct)
			C.free(unsafe.Pointer(ct))
			C.vida_show_copied_hud(gResults)
			return C.TRUE
		}
		return C.FALSE
	}
	if keyval == C.GDK_KEY_Escape {
		cancelInflight()
		switch currentKind {
		case "chat":
			returnToPalette()
		case "note_form":
			returnToPalette()
		default:
			C.vida_hide((*C.GtkWidget)(unsafe.Pointer(userData)))
		}
		return C.TRUE
	}
	// Ctrl+B — back from chat/note view (FR-03b).
	if state&C.GDK_CONTROL_MASK != 0 && keyval == C.GDK_KEY_b {
		if currentKind == "chat" || currentKind == "note_form" {
			cancelInflight()
			returnToPalette()
			return C.TRUE
		}
	}
	// Ctrl+S — save note (FR-05d). Runs on main thread to safely read GTK widgets.
	if state&C.GDK_CONTROL_MASK != 0 && keyval == C.GDK_KEY_s {
		if currentKind == "note_form" {
			saveNote()
			return C.TRUE
		}
	}
	// In chat/note modes: only the special shortcuts above are handled by us;
	// everything else (Enter, arrows, typing) goes straight to the focused widget.
	if currentKind == "chat" || currentKind == "note_form" {
		return C.FALSE
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
		case "command_list":
			idx := selectedIdx
			if idx < 0 {
				idx = 0
			}
			if idx < len(currentCmdNames) {
				name := currentCmdNames[idx]
				// Extract input = text after ":<name> "
				input := ""
				parts := strings.SplitN(currentCmdQuery, " ", 2)
				if len(parts) == 2 {
					input = strings.TrimSpace(parts[1])
				}
				// AI and user commands need input; skip silently if none provided.
				kind := ""
				if idx < len(currentCmdKinds) {
					kind = currentCmdKinds[idx]
				}
				if (kind == "ai" || kind == "user") && input == "" {
					return C.TRUE
				}
				// Note command is handled entirely in the UI.
				if name == "note" {
					prefill := input
					gtkIdle(func() {
						currentKind = "note_form"
						cp := C.CString(prefill)
						C.vida_note_show(cp)
						C.free(unsafe.Pointer(cp))
						C.vida_enter_chat_mode(gWindow)
					})
					return C.TRUE
				}
				go runCommand(name, input, sockFile())
			}
			return C.TRUE
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
		case "calc", "convert":
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

// returnToPalette switches back from chat/note to palette view.
// Must only be called from the GTK main thread (key handler or button handler).
func returnToPalette() {
	currentKind = ""
	chatHistory = nil
	C.vida_chat_clear()
	C.vida_entry_clear(gEntry)
	C.vida_results_clear(gResults)
	C.vida_answer_clear(gAnswer)
	cp := C.CString(placeholderNormal)
	C.vida_entry_set_placeholder(gEntry, cp)
	C.free(unsafe.Pointer(cp))
	C.vida_enter_palette_mode(gWindow)
	C.vida_grab_focus(gEntry)
}

//export goOnChatBack
func goOnChatBack(btn *C.GtkButton, userData C.gpointer) {
	_, _ = btn, userData
	cancelInflight()
	// Direct execution — called from GTK main thread via button signal.
	returnToPalette()
}

//export goOnEntryChanged
func goOnEntryChanged(entry *C.GtkEntry, userData C.gpointer) {
	_ = userData
	var buf [512]C.char
	C.vida_entry_get_text((*C.GtkWidget)(unsafe.Pointer(entry)), &buf[0], 512)
	text := C.GoString(&buf[0])
	onInput(text)
}

const placeholderNormal = "Search apps, calculate, or ask AI\xe2\x80\xa6"
const placeholderCommand = "Type a command\xe2\x80\xa6"

// onInput is called on every keystroke. Routes immediately for non-AI kinds,
// debounces for AI (FR-01b–d).
func onInput(text string) {
	aiDebounce.Stop()
	cancelInflight()

	// Switch placeholder based on mode (FR-01e).
	// Also clear the answer bar immediately so it doesn't linger between keystrokes.
	isCmd := strings.HasPrefix(text, ":")
	gtkIdle(func() {
		C.vida_answer_clear(gAnswer)
		if isCmd {
			cp := C.CString(placeholderCommand)
			C.vida_entry_set_placeholder(gEntry, cp)
			C.free(unsafe.Pointer(cp))
		} else {
			cp := C.CString(placeholderNormal)
			C.vida_entry_set_placeholder(gEntry, cp)
			C.free(unsafe.Pointer(cp))
		}
	})

	if text == "" {
		gtkIdle(func() {
			C.vida_results_clear(gResults)
			C.vida_answer_clear(gAnswer)
			selectedIdx = -1
			currentKind = ""
			currentResultText = ""
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
		// Discard stale responses — a newer query has already been issued.
		if querySeq.Load() != seq {
			return
		}

		switch resp.Kind {
		case "command_list":
			cmds := parseCommandList(resp.Message)
			query := strings.TrimPrefix(text, ":")
			gtkIdle(func() {
				C.vida_answer_clear(gAnswer)
				currentKind = "command_list"
				currentCmdQuery = query
				currentResultText = ""
				selectedIdx = -1
				n := len(cmds)
				currentCmdNames = make([]string, n)
				currentCmdKinds = make([]string, n)
				cnames := make([]*C.char, n)
				cdescs := make([]*C.char, n)
				cicons := make([]*C.char, n)
				for i, c := range cmds {
					currentCmdNames[i] = c.name
					currentCmdKinds[i] = c.kind
					cnames[i] = C.CString(c.name)
					defer C.free(unsafe.Pointer(cnames[i]))
					cdescs[i] = C.CString(c.desc)
					defer C.free(unsafe.Pointer(cdescs[i]))
					cicons[i] = C.CString(c.icon)
					defer C.free(unsafe.Pointer(cicons[i]))
				}
				if n == 0 {
					C.vida_results_set_commands(gResults, nil, nil, nil, 0)
				} else {
					C.vida_results_set_commands(gResults, &cnames[0], &cdescs[0], &cicons[0], C.int(n))
				}
			})
		case "calc":
			val := resp.Value
			gtkIdle(func() {
				currentKind = "calc"
				currentCalcValue = val
				currentResultText = val
				selectedIdx = -1
				C.vida_results_clear(gResults)
				cv := C.CString(val)
				ct := C.CString("CALC")
				C.vida_answer_set(gAnswer, cv, ct)
				C.free(unsafe.Pointer(cv))
				C.free(unsafe.Pointer(ct))
			})
		case "convert":
			val := resp.Value
			gtkIdle(func() {
				currentKind = "convert"
				currentCalcValue = val
				currentResultText = val
				selectedIdx = -1
				C.vida_results_clear(gResults)
				cv := C.CString(val)
				ct := C.CString("CONVERT")
				C.vida_answer_set(gAnswer, cv, ct)
				C.free(unsafe.Pointer(cv))
				C.free(unsafe.Pointer(ct))
			})
		case "shortcut":
			url := resp.URL
			gtkIdle(func() {
				C.vida_answer_clear(gAnswer)
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
				C.vida_answer_clear(gAnswer)
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
				C.vida_answer_clear(gAnswer)
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
				C.vida_answer_clear(gAnswer)
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
				currentResultText = text
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
				C.vida_answer_clear(gAnswer)
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
var idleQueue = make(chan func(), 512)

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

// cmdEntry is a minimal command descriptor parsed from the IPC JSON response.
type cmdEntry struct {
	name string
	desc string
	icon string
	kind string
}

// parseCommandList decodes the JSON array of commands from a command_list IPC message.
func parseCommandList(raw string) []cmdEntry {
	// Simple JSON parse without importing encoding/json into CGo file.
	// The format is: [{"name":"lock","desc":"Lock screen","icon":"...","kind":"system"}, ...]
	// We'll use encoding/json via a local import-free approach.
	type item struct {
		Name string `json:"name"`
		Desc string `json:"desc"`
		Icon string `json:"icon"`
		Kind string `json:"kind"`
	}
	// Use the standard library via a package-level import.
	var items []item
	if err := jsonUnmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	out := make([]cmdEntry, len(items))
	for i, it := range items {
		out[i] = cmdEntry{name: it.Name, desc: it.Desc, icon: it.Icon, kind: it.Kind}
	}
	return out
}

// runCommand sends a run_command IPC message and renders the result.
func runCommand(name, input, sock string) {
	conn, err := ipc.ConnectPersistent(sock)
	if err != nil {
		return
	}
	defer conn.Close()

	seq := querySeq.Add(1)
	id := fmt.Sprintf("cmd-%d", seq)

	if err := conn.SendNoReply(ipc.Message{Type: "run_command", ID: id, Name: name, Input: input}); err != nil {
		return
	}

	msg, err := conn.Recv(15 * time.Second)
	if err != nil {
		return
	}

	switch msg.Type {
	case "command_done":
		// fire-and-forget: close palette.
		gtkIdle(func() {
			C.vida_hide(gWindow)
			C.vida_entry_clear(gEntry)
			C.vida_results_clear(gResults)
			cp := C.CString(placeholderNormal)
			C.vida_entry_set_placeholder(gEntry, cp)
			C.free(unsafe.Pointer(cp))
			currentKind = ""
			currentResultText = ""
		})
	case "command_result":
		val := msg.Value
		gtkIdle(func() {
			currentKind = "command_result"
			currentResultText = val
			selectedIdx = -1
			cv := C.CString(val)
			C.vida_results_set_label(gResults, cv)
			C.free(unsafe.Pointer(cv))
		})
	case "command_error":
		errMsg := msg.Message
		gtkIdle(func() {
			currentKind = ""
			currentResultText = ""
			ce := C.CString("Error: " + errMsg)
			C.vida_results_set_label(gResults, ce)
			C.free(unsafe.Pointer(ce))
		})
	// AI command — stream tokens into chat view.
	case "token":
		// Transition to chat view before first token (FR-01a).
		cmdName := name
		userInput := input
		gtkIdle(func() {
			currentKind = "chat"
			chatCmdName = cmdName
			currentResultText = ""
			cn := C.CString(cmdName)
			cu := C.CString(userInput)
			cr := C.CString("user")
			ca := C.CString("ai")
			ce := C.CString("")
			C.vida_chat_show(cn)
			C.vida_chat_append_message(cr, cu)
			C.vida_chat_append_message(ca, ce)
			C.vida_chat_set_entry_sensitive(C.FALSE)
			C.vida_enter_chat_mode(gWindow)
			C.free(unsafe.Pointer(cn))
			C.free(unsafe.Pointer(cu))
			C.free(unsafe.Pointer(cr))
			C.free(unsafe.Pointer(ca))
			C.free(unsafe.Pointer(ce))
		})

		// Store user turn in history.
		chatHistory = append(chatHistory, chatMsg{role: "user", content: userInput})

		var accumulated strings.Builder
		accumulated.WriteString(msg.Value)
		for {
			next, err := conn.Recv(30 * time.Second)
			if err != nil {
				break
			}
			if next.Type == "done" || next.Type == "cancelled" {
				break
			}
			if next.Type == "token" {
				accumulated.WriteString(next.Value)
				text := accumulated.String()
				gtkIdle(func() {
					currentAIText = text
					currentResultText = text
					ct := C.CString(text)
					C.vida_chat_update_last_ai(ct)
					C.free(unsafe.Pointer(ct))
				})
			}
		}
		// Stream complete — store AI response, re-enable entry.
		aiText := accumulated.String()
		chatHistory = append(chatHistory, chatMsg{role: "assistant", content: aiText})
		gtkIdle(func() {
			C.vida_chat_set_entry_sensitive(C.TRUE)
		})
	}
}

//export goOnChatEntryActivate
func goOnChatEntryActivate(entry *C.GtkEntry, userData C.gpointer) {
	_ = userData
	var buf [2048]C.char
	C.vida_chat_entry_get_text(&buf[0], 2048)
	text := C.GoString(&buf[0])
	if text == "" {
		return
	}
	C.vida_chat_entry_clear()

	// Snapshot history and build follow-up query.
	history := make([]chatMsg, len(chatHistory))
	copy(history, chatHistory)
	name := chatCmdName
	input := text

	go func() {
		conn, err := ipc.ConnectPersistent(sockFile())
		if err != nil {
			return
		}
		defer conn.Close()

		seq := querySeq.Add(1)
		id := fmt.Sprintf("chat-%d", seq)

		// Build IPC history.
		ipcHistory := make([]ipc.HistoryEntry, len(history))
		for i, h := range history {
			ipcHistory[i] = ipc.HistoryEntry{Role: h.role, Content: h.content}
		}

		if err := conn.SendNoReply(ipc.Message{
			Type:    "run_command",
			ID:      id,
			Name:    name,
			Input:   input,
			History: ipcHistory,
		}); err != nil {
			return
		}

		// Add user bubble.
		userText := input
		gtkIdle(func() {
			cu := C.CString(userText)
			C.vida_chat_append_message(C.CString("user"), cu)
			C.free(unsafe.Pointer(cu))
			C.vida_chat_append_message(C.CString("ai"), C.CString(""))
			C.vida_chat_set_entry_sensitive(C.FALSE)
		})
		chatHistory = append(chatHistory, chatMsg{role: "user", content: input})

		var accumulated strings.Builder
		for {
			msg, err := conn.Recv(30 * time.Second)
			if err != nil {
				break
			}
			if msg.Type == "done" || msg.Type == "cancelled" {
				break
			}
			if msg.Type == "token" {
				accumulated.WriteString(msg.Value)
				text := accumulated.String()
				gtkIdle(func() {
					currentAIText = text
					currentResultText = text
					ct := C.CString(text)
					C.vida_chat_update_last_ai(ct)
					C.free(unsafe.Pointer(ct))
				})
			}
		}
		aiReply := accumulated.String()
		chatHistory = append(chatHistory, chatMsg{role: "assistant", content: aiReply})
		gtkIdle(func() { C.vida_chat_set_entry_sensitive(C.TRUE) })
	}()
}

// saveNote reads the note form fields and writes the note file.
func saveNote() {
	var titleBuf, bodyBuf, tagsBuf [2048]C.char
	// These reads happen from a goroutine — snapshot via gtkIdle would be safer,
	// but since this is triggered synchronously by Ctrl+S on the main thread,
	// the form fields are stable at this point.
	C.vida_note_get_title(&titleBuf[0], 2048)
	C.vida_note_get_body(&bodyBuf[0], 2048)
	C.vida_note_get_tags(&tagsBuf[0], 2048)
	title := C.GoString(&titleBuf[0])
	body := C.GoString(&bodyBuf[0])
	tags := C.GoString(&tagsBuf[0])

	cfg, err := config.Load("")
	if err != nil {
		return
	}
	noteCfg := notes.Config{
		Dir:         cfg.Notes.Dir,
		DailySubdir: cfg.Notes.DailySubdir,
		InboxSubdir: cfg.Notes.InboxSubdir,
		Template:    cfg.Notes.Template,
	}
	_ = notes.Save(noteCfg, title, body, tags)

	gtkIdle(func() {
		currentKind = ""
		C.vida_chat_clear()
		C.vida_hide(gWindow)
	})
}

// Suppress unused import warnings for packages only used via CGo indirectly.
var _ = context.Background
