/*
 * ui.c — GTK4 + layer-shell implementation for vida-ui.
 */

#include <gtk/gtk.h>
#include <gtk4-layer-shell/gtk4-layer-shell.h>
#include <string.h>
#include <stdlib.h>

/* Go-exported callbacks */
extern void     goOnActivate(GtkApplication *app, gpointer user_data);
extern gboolean goOnKeyPressed(GtkEventControllerKey *ctrl, guint keyval,
                               guint keycode, GdkModifierType state,
                               gpointer user_data);
extern void     goOnEntryChanged(GtkEntry *entry, gpointer user_data);
extern gboolean goProcessIdle(gpointer user_data);

/* C-side wrapper callbacks */
void vida_on_activate(GtkApplication *app, gpointer data) {
    goOnActivate(app, data);
}

static gboolean vida_on_key_pressed(GtkEventControllerKey *ctrl, guint keyval,
                                     guint keycode, GdkModifierType state,
                                     gpointer data) {
    return goOnKeyPressed(ctrl, keyval, keycode, state, data);
}

static void vida_on_entry_changed(GtkEntry *entry, gpointer data) {
    goOnEntryChanged(entry, data);
}

/*
 * vida_build_window — create the main window with entry + results box.
 * out_entry and out_results are set to the created widgets.
 */
GtkWidget *vida_build_window(GtkApplication *app,
                              GtkWidget **out_entry,
                              GtkWidget **out_results) {
    /* Window */
    GtkWidget *win = gtk_application_window_new(app);
    gtk_window_set_title(GTK_WINDOW(win), "vida");
    gtk_window_set_decorated(GTK_WINDOW(win), FALSE);

    /* Layer shell */
    gtk_layer_init_for_window(GTK_WINDOW(win));
    gtk_layer_set_layer(GTK_WINDOW(win), GTK_LAYER_SHELL_LAYER_OVERLAY);
    gtk_layer_set_keyboard_mode(GTK_WINDOW(win),
                                GTK_LAYER_SHELL_KEYBOARD_MODE_EXCLUSIVE);
    gtk_layer_set_anchor(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_TOP,   TRUE);
    gtk_layer_set_anchor(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_LEFT,  TRUE);
    gtk_layer_set_anchor(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_RIGHT, TRUE);
    gtk_layer_set_exclusive_zone(GTK_WINDOW(win), -1);
    gtk_layer_set_margin(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_TOP, 80);

    /* Outer vertical box */
    GtkWidget *vbox = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
    gtk_window_set_child(GTK_WINDOW(win), vbox);

    /* Search entry */
    GtkWidget *entry = gtk_entry_new();
    gtk_entry_set_placeholder_text(GTK_ENTRY(entry),
        "Search apps, calculate, or ask AI\xe2\x80\xa6");
    gtk_widget_set_hexpand(entry, TRUE);
    gtk_box_append(GTK_BOX(vbox), entry);

    /* Results container */
    GtkWidget *results = gtk_box_new(GTK_ORIENTATION_VERTICAL, 4);
    gtk_widget_set_hexpand(results, TRUE);
    gtk_box_append(GTK_BOX(vbox), results);

    /* Key controller (window-wide: Escape, Enter) */
    GtkEventController *key_ctrl = gtk_event_controller_key_new();
    g_signal_connect(key_ctrl, "key-pressed",
                     G_CALLBACK(vida_on_key_pressed), win);
    gtk_widget_add_controller(win, key_ctrl);

    /* Entry changed → query */
    g_signal_connect(entry, "changed",
                     G_CALLBACK(vida_on_entry_changed), NULL);

    /* GLib timeout to drain the Go idle queue every 16 ms (~60 fps). */
    g_timeout_add(16, G_SOURCE_FUNC(goProcessIdle), NULL);

    gtk_widget_set_visible(win, FALSE);
    gtk_window_present(GTK_WINDOW(win));

    *out_entry   = entry;
    *out_results = results;
    return win;
}

/* --- helpers called from Go --- */

void vida_show(GtkWidget *w)  { gtk_widget_set_visible(w, TRUE);  }
void vida_hide(GtkWidget *w)  { gtk_widget_set_visible(w, FALSE); }

void vida_entry_clear(GtkWidget *entry) {
    gtk_editable_set_text(GTK_EDITABLE(entry), "");
}

void vida_entry_get_text(GtkWidget *entry, char *buf, int buflen) {
    const char *text = gtk_editable_get_text(GTK_EDITABLE(entry));
    strncpy(buf, text ? text : "", buflen - 1);
    buf[buflen - 1] = '\0';
}

void vida_grab_focus(GtkWidget *entry) {
    gtk_widget_grab_focus(entry);
}

/* Remove all children from the results box. */
void vida_results_clear(GtkWidget *box) {
    GtkWidget *child;
    while ((child = gtk_widget_get_first_child(box)) != NULL)
        gtk_box_remove(GTK_BOX(box), child);
}

/* Show a single text label in the results box (calc result or AI text). */
void vida_results_set_label(GtkWidget *box, const char *text) {
    vida_results_clear(box);
    if (!text || !*text) return;
    GtkWidget *label = gtk_label_new(text);
    gtk_label_set_selectable(GTK_LABEL(label), TRUE);
    gtk_label_set_wrap(GTK_LABEL(label), TRUE);
    gtk_label_set_xalign(GTK_LABEL(label), 0.0f);
    gtk_widget_set_hexpand(label, TRUE);
    gtk_box_append(GTK_BOX(box), label);
}

/* Append text to the last label in the box (streaming update). */
void vida_results_append_text(GtkWidget *box, const char *text) {
    /* For simplicity just replace the whole label (called with full accumulated text). */
    vida_results_set_label(box, text);
}

static void open_url_cb(GtkButton *btn, gpointer url) {
    GtkUriLauncher *launcher = gtk_uri_launcher_new((const char *)url);
    gtk_uri_launcher_launch(launcher, NULL, NULL, NULL, NULL);
    g_object_unref(launcher);
}

/* Show a URL row with an "Open" button. */
void vida_results_set_url(GtkWidget *box, const char *url) {
    vida_results_clear(box);
    if (!url || !*url) return;

    GtkWidget *hbox  = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 8);
    GtkWidget *label = gtk_label_new(url);
    gtk_label_set_ellipsize(GTK_LABEL(label), PANGO_ELLIPSIZE_MIDDLE);
    gtk_label_set_xalign(GTK_LABEL(label), 0.0f);
    gtk_widget_set_hexpand(label, TRUE);

    /* Open URL via GtkUriLauncher on button click. */
    GtkWidget *btn = gtk_button_new_with_label("Open");
    char *url_copy = g_strdup(url);
    g_signal_connect_data(btn, "clicked", G_CALLBACK(open_url_cb), url_copy,
                          (GClosureNotify)g_free, 0);

    gtk_box_append(GTK_BOX(hbox), label);
    gtk_box_append(GTK_BOX(hbox), btn);
    gtk_box_append(GTK_BOX(box), hbox);
}

/* Show a list of app name rows. */
void vida_results_set_apps(GtkWidget *box, const char **names, int n) {
    vida_results_clear(box);
    for (int i = 0; i < n && i < 8; i++) {
        if (!names[i] || !*names[i]) continue;
        GtkWidget *row = gtk_label_new(names[i]);
        gtk_label_set_xalign(GTK_LABEL(row), 0.0f);
        gtk_widget_set_hexpand(row, TRUE);
        gtk_box_append(GTK_BOX(box), row);
    }
}
