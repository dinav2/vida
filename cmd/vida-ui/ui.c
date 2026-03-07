/*
 * ui.c — GTK4 + layer-shell implementation for vida-ui.
 * Compiled as a separate CGo C file so callback symbols are non-static
 * and visible to the Go linker.
 */

#include <gtk/gtk.h>
#include <gtk4-layer-shell/gtk4-layer-shell.h>

/* Go-exported callbacks declared via //export in main.go */
extern void     goOnActivate(GtkApplication *app, gpointer user_data);
extern gboolean goOnKeyPressed(GtkEventControllerKey *ctrl, guint keyval,
                               guint keycode, GdkModifierType state,
                               gpointer user_data);

/* C wrappers — these are the actual GCallback function pointers. */
void vida_on_activate(GtkApplication *app, gpointer data) {
    goOnActivate(app, data);
}

gboolean vida_on_key_pressed(GtkEventControllerKey *ctrl, guint keyval,
                              guint keycode, GdkModifierType state,
                              gpointer data) {
    return goOnKeyPressed(ctrl, keyval, keycode, state, data);
}

/* vida_build_window: creates the GTK window and returns it. */
GtkWidget *vida_build_window(GtkApplication *app) {
    GtkWidget *win = gtk_application_window_new(app);
    gtk_window_set_title(GTK_WINDOW(win), "vida");
    gtk_window_set_default_size(GTK_WINDOW(win), 640, 56);
    gtk_window_set_decorated(GTK_WINDOW(win), FALSE);

    /* Configure as a wlr-layer-shell overlay surface. */
    gtk_layer_init_for_window(GTK_WINDOW(win));
    gtk_layer_set_layer(GTK_WINDOW(win), GTK_LAYER_SHELL_LAYER_OVERLAY);
    gtk_layer_set_keyboard_mode(GTK_WINDOW(win),
                                GTK_LAYER_SHELL_KEYBOARD_MODE_EXCLUSIVE);
    gtk_layer_set_anchor(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_TOP,   TRUE);
    gtk_layer_set_anchor(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_LEFT,  TRUE);
    gtk_layer_set_anchor(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_RIGHT, TRUE);
    gtk_layer_set_exclusive_zone(GTK_WINDOW(win), -1);
    gtk_layer_set_margin(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_TOP, 80);

    /* Search entry. */
    GtkWidget *entry = gtk_entry_new();
    gtk_entry_set_placeholder_text(GTK_ENTRY(entry),
        "Search apps, calculate, or ask AI\xe2\x80\xa6");
    gtk_window_set_child(GTK_WINDOW(win), entry);

    /* Key controller for Escape → hide. */
    GtkEventController *ctrl = gtk_event_controller_key_new();
    g_signal_connect(ctrl, "key-pressed",
                     G_CALLBACK(vida_on_key_pressed), win);
    gtk_widget_add_controller(win, ctrl);

    /* Start hidden; present so the window object is fully initialised. */
    gtk_widget_set_visible(win, FALSE);
    gtk_window_present(GTK_WINDOW(win));
    return win;
}

void vida_show(GtkWidget *w) { gtk_widget_set_visible(w, TRUE); }
void vida_hide(GtkWidget *w) { gtk_widget_set_visible(w, FALSE); }
