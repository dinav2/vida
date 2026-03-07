/*
 * ui.c — GTK4 + layer-shell implementation for vida-ui.
 * SPEC-20260307-003: Mac-style launcher aesthetic.
 */

#include <gtk/gtk.h>
#include <gtk4-layer-shell/gtk4-layer-shell.h>
#include <gio/gdesktopappinfo.h>
#include <string.h>
#include <stdlib.h>

/* Go-exported callbacks */
extern void     goOnActivate(GtkApplication *app, gpointer user_data);
extern gboolean goOnKeyPressed(GtkEventControllerKey *ctrl, guint keyval,
                               guint keycode, GdkModifierType state,
                               gpointer user_data);
extern void     goOnEntryChanged(GtkEntry *entry, gpointer user_data);
extern gboolean goProcessIdle(gpointer user_data);
extern void     goOnRowActivated(GtkButton *btn, gpointer user_data);

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

/* ---------- CSS ---------- */

static const char *VIDA_CSS =
    /* Transparent window so compositor renders rounded corners against desktop.
     * Also override the default .background class GTK adds to windows. */
    "window, window.background {"
    "  background: transparent;"
    "  box-shadow: none;"
    "}"

    /* Inner panel: dark frosted glass, rounded corners, subtle border.
     * Width is set via gtk_widget_set_size_request — GTK4 CSS does not
     * support max-width. */
    ".vida-panel {"
    "  background: rgba(20, 20, 25, 0.92);"
    "  border-radius: 16px;"
    "  border: 1px solid rgba(255, 255, 255, 0.08);"
    "}"

    /* Search entry blends into panel */
    ".vida-entry {"
    "  font-family: 'Inter', system-ui, sans-serif;"
    "  font-size: 20px;"
    "  color: #ffffff;"
    "  background: transparent;"
    "  border: none;"
    "  box-shadow: none;"
    "  padding: 16px 20px;"
    "  caret-color: #ffffff;"
    "}"
    /* Kill GTK4/Adwaita focus ring — it uses :focus-within and box-shadow */
    ".vida-entry:focus,"
    ".vida-entry:focus-within,"
    ".vida-entry:focus-visible {"
    "  outline: none;"
    "  box-shadow: none;"
    "}"
    "entry.vida-entry:focus,"
    "entry.vida-entry:focus-within,"
    "entry.vida-entry:focus-visible {"
    "  outline: none;"
    "  box-shadow: none;"
    "}"
    "entry.vida-entry > text:focus,"
    "entry.vida-entry > text:focus-visible {"
    "  outline: none;"
    "  box-shadow: none;"
    "}"
    ".vida-entry text {"
    "  background: transparent;"
    "}"
    ".vida-entry placeholder {"
    "  color: rgba(255, 255, 255, 0.3);"
    "  font-size: 20px;"
    "}"

    /* Search icon */
    ".vida-search-icon {"
    "  color: rgba(255, 255, 255, 0.3);"
    "  margin-left: 20px;"
    "}"

    /* Separator between entry and results */
    ".vida-separator {"
    "  background: rgba(255, 255, 255, 0.08);"
    "  min-height: 1px;"
    "  margin: 0;"
    "}"

    /* Results container */
    ".vida-results {"
    "  padding: 8px;"
    "}"

    /* Individual result row — flat button for hover */
    ".vida-row {"
    "  background: transparent;"
    "  border: none;"
    "  border-radius: 8px;"
    "  padding: 0;"
    "  min-height: 44px;"
    "}"
    ".vida-row:hover {"
    "  background: rgba(255, 255, 255, 0.07);"
    "}"
    ".vida-row:active {"
    "  background: rgba(255, 255, 255, 0.12);"
    "}"
    ".vida-row:focus,"
    ".vida-row:focus-visible {"
    "  outline: 1px solid rgba(255, 255, 255, 0.25);"
    "  box-shadow: none;"
    "}"
    /* Keyboard-selected row — brighter than hover */
    ".vida-row-selected {"
    "  background: rgba(255, 255, 255, 0.13);"
    "  outline: 1px solid rgba(255, 255, 255, 0.2);"
    "}"

    /* Row label: app/calc name */
    ".vida-row-label {"
    "  font-family: 'Inter', system-ui, sans-serif;"
    "  font-size: 15px;"
    "  color: #ffffff;"
    "  padding: 0 12px;"
    "}"

    /* Row type badge: "Calculator", "App", "Web" */
    ".vida-row-type {"
    "  font-family: 'Inter', system-ui, sans-serif;"
    "  font-size: 12px;"
    "  color: rgba(255, 255, 255, 0.4);"
    "  padding: 0 12px;"
    "}"

    /* Open URL button */
    ".vida-open-btn {"
    "  font-family: 'Inter', system-ui, sans-serif;"
    "  font-size: 12px;"
    "  color: rgba(255, 255, 255, 0.6);"
    "  background: rgba(255, 255, 255, 0.1);"
    "  border: none;"
    "  border-radius: 6px;"
    "  padding: 4px 10px;"
    "  margin-right: 12px;"
    "}"
    ".vida-open-btn:hover {"
    "  background: rgba(255, 255, 255, 0.18);"
    "}";

static void load_css(void) {
    GtkCssProvider *provider = gtk_css_provider_new();
    gtk_css_provider_load_from_string(provider, VIDA_CSS);
    gtk_style_context_add_provider_for_display(
        gdk_display_get_default(),
        GTK_STYLE_PROVIDER(provider),
        GTK_STYLE_PROVIDER_PRIORITY_USER);
    g_object_unref(provider);
}

/* ---------- Window builder ---------- */

GtkWidget *vida_build_window(GtkApplication *app,
                              GtkWidget **out_entry,
                              GtkWidget **out_results) {
    load_css();

    /* Transparent window */
    GtkWidget *win = gtk_application_window_new(app);
    gtk_window_set_title(GTK_WINDOW(win), "vida");
    gtk_window_set_decorated(GTK_WINDOW(win), FALSE);
    gtk_widget_set_name(win, "vida-window");

    /* Layer shell: anchor TOP+LEFT+RIGHT so the window spans full screen width
     * (transparent). The inner panel is centered via halign + size_request. */
    gtk_layer_init_for_window(GTK_WINDOW(win));
    gtk_layer_set_layer(GTK_WINDOW(win), GTK_LAYER_SHELL_LAYER_OVERLAY);
    gtk_layer_set_keyboard_mode(GTK_WINDOW(win),
                                GTK_LAYER_SHELL_KEYBOARD_MODE_EXCLUSIVE);
    gtk_layer_set_anchor(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_TOP,   TRUE);
    gtk_layer_set_anchor(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_LEFT,  TRUE);
    gtk_layer_set_anchor(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_RIGHT, TRUE);
    gtk_layer_set_exclusive_zone(GTK_WINDOW(win), -1);
    gtk_layer_set_margin(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_TOP, 200);

    /* Full-width transparent container — just holds the centered panel */
    GtkWidget *root = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
    gtk_window_set_child(GTK_WINDOW(win), root);

    /* Inner panel — dark background + rounded corners, fixed 640px width */
    GtkWidget *panel = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
    gtk_widget_add_css_class(panel, "vida-panel");
    gtk_widget_set_halign(panel, GTK_ALIGN_CENTER);
    gtk_widget_set_size_request(panel, 640, -1);
    gtk_widget_set_overflow(panel, GTK_OVERFLOW_HIDDEN); /* clip children to border-radius */
    gtk_box_append(GTK_BOX(root), panel);

    /* Entry row: search icon + text entry */
    GtkWidget *entry_row = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
    gtk_widget_set_valign(entry_row, GTK_ALIGN_CENTER);

    GtkWidget *icon = gtk_image_new_from_icon_name("system-search-symbolic");
    gtk_widget_add_css_class(icon, "vida-search-icon");
    gtk_box_append(GTK_BOX(entry_row), icon);

    GtkWidget *entry = gtk_entry_new();
    gtk_entry_set_placeholder_text(GTK_ENTRY(entry),
        "Search apps, calculate, or ask AI\xe2\x80\xa6");
    gtk_widget_add_css_class(entry, "vida-entry");
    gtk_widget_set_hexpand(entry, TRUE);
    gtk_box_append(GTK_BOX(entry_row), entry);

    gtk_box_append(GTK_BOX(panel), entry_row);

    /* Separator */
    GtkWidget *sep = gtk_separator_new(GTK_ORIENTATION_HORIZONTAL);
    gtk_widget_add_css_class(sep, "vida-separator");
    /* Hidden by default; shown when results are present */
    gtk_widget_set_visible(sep, FALSE);
    gtk_box_append(GTK_BOX(panel), sep);

    /* Results container */
    GtkWidget *results = gtk_box_new(GTK_ORIENTATION_VERTICAL, 4);
    gtk_widget_add_css_class(results, "vida-results");
    gtk_widget_set_hexpand(results, TRUE);
    gtk_box_append(GTK_BOX(panel), results);

    /* Key controller — CAPTURE phase so we intercept keys before GtkEntry
     * consumes Enter/Escape/arrows. */
    GtkEventController *key_ctrl = gtk_event_controller_key_new();
    gtk_event_controller_set_propagation_phase(key_ctrl, GTK_PHASE_CAPTURE);
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

/* ---------- Helpers called from Go ---------- */

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

/* Show/hide the separator that sits between entry and results. */
static void set_separator_visible(GtkWidget *results, gboolean visible) {
    /* Walk up to panel, then find separator (second child of panel). */
    GtkWidget *panel = gtk_widget_get_parent(results);
    if (!panel) return;
    GtkWidget *child = gtk_widget_get_first_child(panel);
    /* panel children: entry_row, separator, results */
    if (child) child = gtk_widget_get_next_sibling(child); /* separator */
    if (child) gtk_widget_set_visible(child, visible);
}

/* Remove all children from the results box. */
void vida_results_clear(GtkWidget *box) {
    GtkWidget *child;
    while ((child = gtk_widget_get_first_child(box)) != NULL)
        gtk_box_remove(GTK_BOX(box), child);
    set_separator_visible(box, FALSE);
}

/* ---------- Row builder ---------- */

/* make_row creates a styled flat-button row with a left label and right type badge. */
static GtkWidget *make_row(const char *text, const char *type_label) {
    GtkWidget *btn = gtk_button_new();
    gtk_widget_add_css_class(btn, "vida-row");
    gtk_button_set_has_frame(GTK_BUTTON(btn), FALSE);

    GtkWidget *hbox = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
    gtk_widget_set_hexpand(hbox, TRUE);

    GtkWidget *lbl = gtk_label_new(text);
    gtk_widget_add_css_class(lbl, "vida-row-label");
    gtk_label_set_ellipsize(GTK_LABEL(lbl), PANGO_ELLIPSIZE_END);
    gtk_label_set_xalign(GTK_LABEL(lbl), 0.0f);
    gtk_widget_set_hexpand(lbl, TRUE);

    GtkWidget *type_lbl = gtk_label_new(type_label);
    gtk_widget_add_css_class(type_lbl, "vida-row-type");
    gtk_label_set_xalign(GTK_LABEL(type_lbl), 1.0f);

    gtk_box_append(GTK_BOX(hbox), lbl);
    gtk_box_append(GTK_BOX(hbox), type_lbl);
    gtk_button_set_child(GTK_BUTTON(btn), hbox);
    return btn;
}

/* Show a single text label (calc result or AI streaming text). */
void vida_results_set_label(GtkWidget *box, const char *text) {
    vida_results_clear(box);
    if (!text || !*text) return;

    GtkWidget *btn = make_row(text, "Calculator");
    /* Make AI/calc rows selectable — override with plain label for AI */
    gtk_box_append(GTK_BOX(box), btn);
    set_separator_visible(box, TRUE);
}

/* Show AI streaming text with "AI" type label. */
void vida_results_set_ai_text(GtkWidget *box, const char *text) {
    vida_results_clear(box);
    if (!text || !*text) return;
    GtkWidget *btn = make_row(text, "AI");
    gtk_box_append(GTK_BOX(box), btn);
    set_separator_visible(box, TRUE);
}

/* Append text to results (streaming update — replaces whole label). */
void vida_results_append_text(GtkWidget *box, const char *text) {
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

    GtkWidget *btn = gtk_button_new();
    gtk_widget_add_css_class(btn, "vida-row");
    gtk_button_set_has_frame(GTK_BUTTON(btn), FALSE);

    GtkWidget *hbox = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
    gtk_widget_set_hexpand(hbox, TRUE);

    GtkWidget *lbl = gtk_label_new(url);
    gtk_widget_add_css_class(lbl, "vida-row-label");
    gtk_label_set_ellipsize(GTK_LABEL(lbl), PANGO_ELLIPSIZE_MIDDLE);
    gtk_label_set_xalign(GTK_LABEL(lbl), 0.0f);
    gtk_widget_set_hexpand(lbl, TRUE);

    GtkWidget *type_lbl = gtk_label_new("Web");
    gtk_widget_add_css_class(type_lbl, "vida-row-type");

    GtkWidget *open_btn = gtk_button_new_with_label("Open");
    gtk_widget_add_css_class(open_btn, "vida-open-btn");
    char *url_copy = g_strdup(url);
    g_signal_connect_data(open_btn, "clicked", G_CALLBACK(open_url_cb), url_copy,
                          (GClosureNotify)g_free, 0);

    gtk_box_append(GTK_BOX(hbox), lbl);
    gtk_box_append(GTK_BOX(hbox), type_lbl);
    gtk_box_append(GTK_BOX(hbox), open_btn);
    gtk_button_set_child(GTK_BUTTON(btn), hbox);
    gtk_box_append(GTK_BOX(box), btn);
    set_separator_visible(box, TRUE);
}

/* Show a list of app name rows. */
void vida_results_set_apps(GtkWidget *box, const char **names, int n) {
    vida_results_clear(box);
    for (int i = 0; i < n && i < 6; i++) {
        if (!names[i] || !*names[i]) continue;
        GtkWidget *row = make_row(names[i], "App");
        gtk_box_append(GTK_BOX(box), row);
    }
    if (n > 0) set_separator_visible(box, TRUE);
}

/* Add/remove the selected highlight from a specific row by index. */
void vida_select_row(GtkWidget *box, int idx) {
    int i = 0;
    GtkWidget *child = gtk_widget_get_first_child(box);
    while (child) {
        if (i == idx) {
            gtk_widget_add_css_class(child, "vida-row-selected");
        } else {
            gtk_widget_remove_css_class(child, "vida-row-selected");
        }
        child = gtk_widget_get_next_sibling(child);
        i++;
    }
}

/* Return the number of children in the results box. */
int vida_count_rows(GtkWidget *box) {
    int n = 0;
    GtkWidget *child = gtk_widget_get_first_child(box);
    while (child) { n++; child = gtk_widget_get_next_sibling(child); }
    return n;
}

/* Launch a .desktop app by its ID (e.g. "firefox.desktop").
 * Passes the GDK display launch context so the app gets the correct
 * Wayland display/compositor connection. */
void vida_launch_app(const char *desktop_id) {
    if (!desktop_id || !*desktop_id) return;
    GDesktopAppInfo *info = g_desktop_app_info_new(desktop_id);
    if (!info) return;
    GdkDisplay *display = gdk_display_get_default();
    GAppLaunchContext *ctx = G_APP_LAUNCH_CONTEXT(gdk_display_get_app_launch_context(display));
    GError *err = NULL;
    g_app_info_launch(G_APP_INFO(info), NULL, ctx, &err);
    if (err) g_error_free(err);
    g_object_unref(ctx);
    g_object_unref(info);
}

/* Copy text to the system clipboard. */
void vida_copy_to_clipboard(GtkWidget *widget, const char *text) {
    if (!text || !*text) return;
    GdkClipboard *cb = gtk_widget_get_clipboard(widget);
    gdk_clipboard_set_text(cb, text);
}

/* Open a URL via GtkUriLauncher (reusable from Enter key). */
void vida_open_url(const char *url) {
    if (!url || !*url) return;
    GtkUriLauncher *launcher = gtk_uri_launcher_new(url);
    gtk_uri_launcher_launch(launcher, NULL, NULL, NULL, NULL);
    g_object_unref(launcher);
}
