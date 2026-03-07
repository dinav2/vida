// vida-ui is the persistent Wayland layer-shell window for vida.
// It subscribes to IPC broadcasts from vida-daemon and shows/hides on command.
package main

/*
#cgo pkg-config: gtk4 gtk4-layer-shell-0
#include <gtk/gtk.h>

// Declarations for functions implemented in ui.c.
extern void       vida_on_activate(GtkApplication *app, gpointer data);
extern GtkWidget *vida_build_window(GtkApplication *app);
extern void       vida_show(GtkWidget *w);
extern void       vida_hide(GtkWidget *w);
*/
import "C"

import (
	"log"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"github.com/dinav2/vida/internal/ipc"
)

// gWindow is set once in goOnActivate.
var gWindow *C.GtkWidget

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
	win := C.vida_build_window(app)
	gWindow = win
	go subscribeLoop(sockFile())
}

//export goOnKeyPressed
func goOnKeyPressed(ctrl *C.GtkEventControllerKey, keyval C.guint,
	keycode C.guint, state C.GdkModifierType, userData C.gpointer) C.gboolean {
	_, _, _ = ctrl, keycode, state
	if keyval == C.GDK_KEY_Escape {
		C.vida_hide((*C.GtkWidget)(unsafe.Pointer(userData)))
		return C.TRUE
	}
	return C.FALSE
}

// subscribeLoop maintains a persistent IPC connection, reconnecting on error.
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
			if gWindow != nil {
				C.vida_show(gWindow)
			}
		case "hide":
			if gWindow != nil {
				C.vida_hide(gWindow)
			}
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
