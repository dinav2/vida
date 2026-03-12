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
extern void     goOnChatEntryActivate(GtkEntry *entry, gpointer user_data);
extern void     goOnChatBack(GtkButton *btn, gpointer user_data);

/* Chat / note view static widget references */
static GtkWidget *s_stack         = NULL;
static GtkWidget *s_win           = NULL;
static GtkWidget *s_chat_scroll   = NULL;
static GtkWidget *s_chat_history  = NULL;
static GtkWidget *s_chat_header   = NULL;
static GtkWidget *s_chat_entry    = NULL;
static GtkWidget *s_note_title    = NULL;
static GtkWidget *s_note_body_tv  = NULL;
static GtkWidget *s_note_tags     = NULL;

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
    "  min-height: 56px;"
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

    /* Command rows */
    ".vida-cmd-name {"
    "  font-family: 'Inter', system-ui, sans-serif;"
    "  font-size: 14px;"
    "  font-weight: 600;"
    "  color: #ffffff;"
    "  padding: 0 4px;"
    "}"
    ".vida-cmd-desc {"
    "  font-family: 'Inter', system-ui, sans-serif;"
    "  font-size: 12px;"
    "  color: rgba(255, 255, 255, 0.45);"
    "  padding: 0 4px;"
    "}"

    /* HUD "Copied" indicator */
    ".vida-hud {"
    "  font-family: 'Inter', system-ui, sans-serif;"
    "  font-size: 13px;"
    "  color: rgba(255, 255, 255, 0.7);"
    "  background: rgba(255, 255, 255, 0.1);"
    "  border-radius: 6px;"
    "  padding: 4px 12px;"
    "  margin: 4px 8px;"
    "}"

    /* App icon in launcher rows */
    ".vida-app-icon {"
    "  border-radius: 6px;"
    "  margin: 0 8px 0 12px;"
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
    "}"

    /* Answer bar — dedicated display for calc / convert results */
    ".vida-answer {"
    "  padding: 14px 20px 12px 20px;"
    "  border-top: 1px solid rgba(255, 255, 255, 0.06);"
    "  border-bottom: 1px solid rgba(255, 255, 255, 0.06);"
    "}"
    ".vida-answer-value {"
    "  font-family: 'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace;"
    "  font-size: 24px;"
    "  font-weight: 600;"
    "  color: #ffffff;"
    "}"
    ".vida-answer-type {"
    "  font-family: 'Inter', system-ui, sans-serif;"
    "  font-size: 11px;"
    "  color: rgba(255, 255, 255, 0.28);"
    "  letter-spacing: 1px;"
    "  margin-top: 2px;"
    "}"

    /* ── Chat view ──────────────────────────────────────────── */

    /* Header: command name + back button */
    ".vida-chat-header {"
    "  padding: 14px 20px 12px 20px;"
    "  border-bottom: 1px solid rgba(255, 255, 255, 0.07);"
    "}"
    ".vida-chat-header-label {"
    "  font-family: 'Inter', system-ui, sans-serif;"
    "  font-size: 12px;"
    "  font-weight: 700;"
    "  color: rgba(255, 255, 255, 0.4);"
    "  letter-spacing: 1.5px;"
    "  text-transform: uppercase;"
    "}"
    ".vida-chat-back {"
    "  font-size: 18px;"
    "  color: rgba(255, 255, 255, 0.3);"
    "  background: transparent;"
    "  border: none;"
    "  border-radius: 6px;"
    "  padding: 2px 8px;"
    "  min-height: 0;"
    "}"
    ".vida-chat-back:hover {"
    "  color: rgba(255,255,255,0.75);"
    "  background: rgba(255,255,255,0.07);"
    "}"

    /* Scrollable message history */
    ".vida-chat-history {"
    "  padding: 16px 20px 8px 20px;"
    "}"

    /* User bubble — right side, pill shape, subtle fill */
    ".vida-msg-user {"
    "  font-family: 'Inter', system-ui, sans-serif;"
    "  font-size: 14px;"
    "  line-height: 1.5;"
    "  color: #ffffff;"
    "  background: rgba(99, 102, 241, 0.35);"
    "  border-radius: 18px 18px 4px 18px;"
    "  padding: 10px 16px;"
    "  margin: 3px 0 3px 80px;"
    "}"

    /* AI bubble — left side, no fill, lighter text */
    ".vida-msg-ai {"
    "  font-family: 'Inter', system-ui, sans-serif;"
    "  font-size: 14px;"
    "  line-height: 1.6;"
    "  color: rgba(255, 255, 255, 0.88);"
    "  background: rgba(255, 255, 255, 0.05);"
    "  border-radius: 4px 18px 18px 18px;"
    "  padding: 10px 16px;"
    "  margin: 3px 80px 3px 0;"
    "}"

    /* Chat input row */
    ".vida-chat-input-row {"
    "  padding: 12px 16px 14px 16px;"
    "  border-top: 1px solid rgba(255, 255, 255, 0.07);"
    "}"
    ".vida-chat-entry {"
    "  font-family: 'Inter', system-ui, sans-serif;"
    "  font-size: 14px;"
    "  color: #ffffff;"
    "  background: rgba(255, 255, 255, 0.07);"
    "  border: 1px solid rgba(255, 255, 255, 0.1);"
    "  border-radius: 20px;"
    "  padding: 9px 16px;"
    "  caret-color: #ffffff;"
    "}"
    ".vida-chat-entry:focus {"
    "  border-color: rgba(99, 102, 241, 0.6);"
    "  background: rgba(255, 255, 255, 0.09);"
    "  outline: none;"
    "  box-shadow: none;"
    "}"
    "entry.vida-chat-entry > text { background: transparent; }"
    "entry.vida-chat-entry:focus, entry.vida-chat-entry:focus-within,"
    "entry.vida-chat-entry:focus-visible { outline: none; box-shadow: none; }"

    /* ── Note form ──────────────────────────────────────────── */

    ".vida-note-title {"
    "  font-family: 'Inter', system-ui, sans-serif;"
    "  font-size: 18px;"
    "  font-weight: 600;"
    "  color: #ffffff;"
    "  background: transparent;"
    "  border: none;"
    "  border-radius: 0;"
    "  padding: 18px 22px 14px 22px;"
    "  box-shadow: none;"
    "  caret-color: #ffffff;"
    "}"
    ".vida-note-title:focus { outline: none; box-shadow: none; }"
    "entry.vida-note-title > text { background: transparent; }"
    "entry.vida-note-title:focus, entry.vida-note-title:focus-within,"
    "entry.vida-note-title:focus-visible { outline: none; box-shadow: none; }"

    /* Textarea for note body */
    ".vida-note-body {"
    "  font-family: 'Inter', system-ui, sans-serif;"
    "  font-size: 14px;"
    "  line-height: 1.65;"
    "  background: transparent;"
    "  border: none;"
    "  padding: 0 22px 8px 22px;"
    "}"
    "textview.vida-note-body > text {"
    "  background: transparent;"
    "  color: rgba(255, 255, 255, 0.8);"
    "  caret-color: #ffffff;"
    "}"
    "textview.vida-note-body:focus, textview.vida-note-body:focus-visible {"
    "  outline: none;"
    "  box-shadow: none;"
    "}"

    /* Divider between body and tags */
    ".vida-note-divider {"
    "  background: rgba(255, 255, 255, 0.07);"
    "  min-height: 1px;"
    "  margin: 4px 0;"
    "}"

    ".vida-note-tags {"
    "  font-family: 'Inter', system-ui, sans-serif;"
    "  font-size: 13px;"
    "  color: rgba(255, 255, 255, 0.4);"
    "  background: transparent;"
    "  border: none;"
    "  border-radius: 0;"
    "  padding: 10px 22px 6px 22px;"
    "  box-shadow: none;"
    "}"
    ".vida-note-tags:focus { outline: none; box-shadow: none; }"
    "entry.vida-note-tags > text { background: transparent; }"
    "entry.vida-note-tags:focus, entry.vida-note-tags:focus-within,"
    "entry.vida-note-tags:focus-visible { outline: none; box-shadow: none; }"

    ".vida-note-hint {"
    "  font-family: 'Inter', system-ui, sans-serif;"
    "  font-size: 11px;"
    "  color: rgba(255, 255, 255, 0.2);"
    "  padding: 2px 22px 12px 22px;"
    "  letter-spacing: 0.3px;"
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
                              GtkWidget **out_results,
                              GtkWidget **out_answer) {
    load_css();

    /* Transparent window */
    GtkWidget *win = gtk_application_window_new(app);
    gtk_window_set_title(GTK_WINDOW(win), "vida");
    gtk_window_set_decorated(GTK_WINDOW(win), FALSE);
    gtk_widget_set_name(win, "vida-window");
    s_win = win;

    /* Layer shell */
    gtk_layer_init_for_window(GTK_WINDOW(win));
    gtk_layer_set_layer(GTK_WINDOW(win), GTK_LAYER_SHELL_LAYER_OVERLAY);
    gtk_layer_set_keyboard_mode(GTK_WINDOW(win),
                                GTK_LAYER_SHELL_KEYBOARD_MODE_EXCLUSIVE);
    gtk_layer_set_anchor(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_TOP,   TRUE);
    gtk_layer_set_anchor(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_LEFT,  TRUE);
    gtk_layer_set_anchor(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_RIGHT, TRUE);
    gtk_layer_set_exclusive_zone(GTK_WINDOW(win), -1);
    gtk_layer_set_margin(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_TOP, 200);

    /* Full-width transparent container */
    GtkWidget *root = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
    gtk_window_set_child(GTK_WINDOW(win), root);

    /* GtkStack — "palette" | "chat" | "note" */
    GtkWidget *stack = gtk_stack_new();
    gtk_widget_add_css_class(stack, "vida-root-stack");
    gtk_widget_set_halign(stack, GTK_ALIGN_CENTER);
    gtk_widget_set_size_request(stack, 640, -1);
    gtk_stack_set_transition_type(GTK_STACK(stack), GTK_STACK_TRANSITION_TYPE_NONE);
    gtk_box_append(GTK_BOX(root), stack);
    s_stack = stack;

    /* ── PALETTE PAGE ─────────────────────────────────────────── */
    GtkWidget *panel = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
    gtk_widget_add_css_class(panel, "vida-panel");
    gtk_widget_set_overflow(panel, GTK_OVERFLOW_HIDDEN);
    gtk_stack_add_named(GTK_STACK(stack), panel, "palette");

    /* Entry row */
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

    /* Answer bar */
    GtkWidget *answer = gtk_box_new(GTK_ORIENTATION_VERTICAL, 2);
    gtk_widget_add_css_class(answer, "vida-answer");
    gtk_widget_set_hexpand(answer, TRUE);
    gtk_widget_set_visible(answer, FALSE);

    GtkWidget *answer_value = gtk_label_new("");
    gtk_widget_add_css_class(answer_value, "vida-answer-value");
    gtk_label_set_xalign(GTK_LABEL(answer_value), 0.0f);
    gtk_label_set_wrap(GTK_LABEL(answer_value), TRUE);
    gtk_label_set_wrap_mode(GTK_LABEL(answer_value), PANGO_WRAP_WORD_CHAR);
    gtk_label_set_max_width_chars(GTK_LABEL(answer_value), 1);
    gtk_widget_set_hexpand(answer_value, TRUE);
    gtk_box_append(GTK_BOX(answer), answer_value);

    GtkWidget *answer_type = gtk_label_new("");
    gtk_widget_add_css_class(answer_type, "vida-answer-type");
    gtk_label_set_xalign(GTK_LABEL(answer_type), 0.0f);
    gtk_box_append(GTK_BOX(answer), answer_type);

    gtk_box_append(GTK_BOX(panel), answer);
    g_object_set_data(G_OBJECT(answer), "value-label", answer_value);
    g_object_set_data(G_OBJECT(answer), "type-label", answer_type);

    /* Separator */
    GtkWidget *sep = gtk_separator_new(GTK_ORIENTATION_HORIZONTAL);
    gtk_widget_add_css_class(sep, "vida-separator");
    gtk_widget_set_visible(sep, FALSE);
    gtk_box_append(GTK_BOX(panel), sep);

    /* Results */
    GtkWidget *results = gtk_box_new(GTK_ORIENTATION_VERTICAL, 4);
    gtk_widget_add_css_class(results, "vida-results");
    gtk_widget_set_hexpand(results, TRUE);
    gtk_box_append(GTK_BOX(panel), results);

    /* ── CHAT PAGE ────────────────────────────────────────────── */
    GtkWidget *chat_panel = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
    gtk_widget_add_css_class(chat_panel, "vida-panel");
    gtk_widget_set_overflow(chat_panel, GTK_OVERFLOW_HIDDEN);
    gtk_stack_add_named(GTK_STACK(stack), chat_panel, "chat");

    /* Header */
    GtkWidget *chat_header = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 8);
    gtk_widget_add_css_class(chat_header, "vida-chat-header");
    gtk_box_append(GTK_BOX(chat_panel), chat_header);
    s_chat_header = chat_header;

    GtkWidget *chat_title = gtk_label_new("");
    gtk_widget_add_css_class(chat_title, "vida-chat-header-label");
    gtk_label_set_xalign(GTK_LABEL(chat_title), 0.0f);
    gtk_widget_set_hexpand(chat_title, TRUE);
    gtk_box_append(GTK_BOX(chat_header), chat_title);
    g_object_set_data(G_OBJECT(chat_header), "title-label", chat_title);

    GtkWidget *back_btn = gtk_button_new_with_label("\xc3\x97"); /* × */
    gtk_widget_add_css_class(back_btn, "vida-chat-back");
    gtk_button_set_has_frame(GTK_BUTTON(back_btn), FALSE);
    gtk_box_append(GTK_BOX(chat_header), back_btn);
    g_signal_connect(back_btn, "clicked", G_CALLBACK(goOnChatBack), NULL);

    /* Scrolled message history */
    GtkWidget *chat_scroll = gtk_scrolled_window_new();
    gtk_scrolled_window_set_policy(GTK_SCROLLED_WINDOW(chat_scroll),
                                   GTK_POLICY_NEVER, GTK_POLICY_AUTOMATIC);
    gtk_widget_set_vexpand(chat_scroll, TRUE);
    gtk_widget_set_size_request(chat_scroll, -1, 100);
    gtk_box_append(GTK_BOX(chat_panel), chat_scroll);
    s_chat_scroll = chat_scroll;

    GtkWidget *chat_history = gtk_box_new(GTK_ORIENTATION_VERTICAL, 4);
    gtk_widget_add_css_class(chat_history, "vida-chat-history");
    gtk_widget_set_vexpand(chat_history, FALSE);
    gtk_scrolled_window_set_child(GTK_SCROLLED_WINDOW(chat_scroll), chat_history);
    s_chat_history = chat_history;

    /* Bottom input row */
    GtkWidget *chat_input_row = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 8);
    gtk_widget_add_css_class(chat_input_row, "vida-chat-input-row");
    gtk_box_append(GTK_BOX(chat_panel), chat_input_row);

    GtkWidget *chat_entry = gtk_entry_new();
    gtk_entry_set_placeholder_text(GTK_ENTRY(chat_entry), "Follow-up\xe2\x80\xa6");
    gtk_widget_add_css_class(chat_entry, "vida-chat-entry");
    gtk_widget_set_hexpand(chat_entry, TRUE);
    gtk_box_append(GTK_BOX(chat_input_row), chat_entry);
    s_chat_entry = chat_entry;
    g_signal_connect(chat_entry, "activate",
                     G_CALLBACK(goOnChatEntryActivate), NULL);

    /* ── NOTE PAGE ────────────────────────────────────────────── */
    GtkWidget *note_panel = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
    gtk_widget_add_css_class(note_panel, "vida-panel");
    gtk_widget_set_overflow(note_panel, GTK_OVERFLOW_HIDDEN);
    gtk_stack_add_named(GTK_STACK(stack), note_panel, "note");

    /* Note header */
    GtkWidget *note_header = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 8);
    gtk_widget_add_css_class(note_header, "vida-chat-header");
    gtk_box_append(GTK_BOX(note_panel), note_header);

    GtkWidget *note_header_lbl = gtk_label_new("New note");
    gtk_widget_add_css_class(note_header_lbl, "vida-chat-header-label");
    gtk_label_set_xalign(GTK_LABEL(note_header_lbl), 0.0f);
    gtk_widget_set_hexpand(note_header_lbl, TRUE);
    gtk_box_append(GTK_BOX(note_header), note_header_lbl);

    GtkWidget *note_close = gtk_button_new_with_label("\xc3\x97");
    gtk_widget_add_css_class(note_close, "vida-chat-back");
    gtk_button_set_has_frame(GTK_BUTTON(note_close), FALSE);
    gtk_box_append(GTK_BOX(note_header), note_close);
    g_signal_connect(note_close, "clicked", G_CALLBACK(goOnChatBack), NULL);

    /* Title entry */
    GtkWidget *note_title = gtk_entry_new();
    gtk_entry_set_placeholder_text(GTK_ENTRY(note_title), "Title (leave blank for daily note)");
    gtk_widget_add_css_class(note_title, "vida-note-title");
    gtk_widget_set_hexpand(note_title, TRUE);
    gtk_box_append(GTK_BOX(note_panel), note_title);
    s_note_title = note_title;

    /* Body textarea */
    /* Body in a scrolled window so the panel never grows beyond its natural size. */
    GtkWidget *note_body_scroll = gtk_scrolled_window_new();
    gtk_scrolled_window_set_policy(GTK_SCROLLED_WINDOW(note_body_scroll),
                                   GTK_POLICY_NEVER, GTK_POLICY_AUTOMATIC);
    gtk_widget_set_hexpand(note_body_scroll, TRUE);
    gtk_widget_set_size_request(note_body_scroll, -1, 200);
    gtk_box_append(GTK_BOX(note_panel), note_body_scroll);

    GtkWidget *note_body_tv = gtk_text_view_new();
    gtk_widget_add_css_class(note_body_tv, "vida-note-body");
    gtk_text_view_set_wrap_mode(GTK_TEXT_VIEW(note_body_tv), GTK_WRAP_WORD_CHAR);
    gtk_widget_set_hexpand(note_body_tv, TRUE);
    gtk_scrolled_window_set_child(GTK_SCROLLED_WINDOW(note_body_scroll), note_body_tv);
    s_note_body_tv = note_body_tv;

    /* Divider */
    GtkWidget *note_div = gtk_separator_new(GTK_ORIENTATION_HORIZONTAL);
    gtk_widget_add_css_class(note_div, "vida-note-divider");
    gtk_box_append(GTK_BOX(note_panel), note_div);

    /* Tags entry */
    GtkWidget *note_tags = gtk_entry_new();
    gtk_entry_set_placeholder_text(GTK_ENTRY(note_tags), "tags, comma, separated");
    gtk_widget_add_css_class(note_tags, "vida-note-tags");
    gtk_widget_set_hexpand(note_tags, TRUE);
    gtk_box_append(GTK_BOX(note_panel), note_tags);
    s_note_tags = note_tags;

    GtkWidget *note_hint = gtk_label_new("Ctrl+S  save   \xc2\xb7\xc2\xb7\xc2\xb7   Esc  discard");
    gtk_widget_add_css_class(note_hint, "vida-note-hint");
    gtk_label_set_xalign(GTK_LABEL(note_hint), 0.0f);
    gtk_box_append(GTK_BOX(note_panel), note_hint);

    /* ── KEY CONTROLLER ──────────────────────────────────────── */
    GtkEventController *key_ctrl = gtk_event_controller_key_new();
    gtk_event_controller_set_propagation_phase(key_ctrl, GTK_PHASE_CAPTURE);
    g_signal_connect(key_ctrl, "key-pressed",
                     G_CALLBACK(vida_on_key_pressed), win);
    gtk_widget_add_controller(win, key_ctrl);

    /* Entry changed → query (palette only) */
    g_signal_connect(entry, "changed",
                     G_CALLBACK(vida_on_entry_changed), NULL);

    /* GLib timeout to drain Go idle queue */
    g_timeout_add(16, G_SOURCE_FUNC(goProcessIdle), NULL);

    gtk_widget_set_visible(win, FALSE);
    gtk_window_present(GTK_WINDOW(win));

    *out_entry   = entry;
    *out_results = results;
    *out_answer  = answer;
    return win;
}

/* ---------- Helpers called from Go ---------- */

void vida_show(GtkWidget *w)  { gtk_widget_set_visible(w, TRUE);  }
void vida_hide(GtkWidget *w)  { gtk_widget_set_visible(w, FALSE); }

/* Show the answer bar with a computed value and type label (e.g. "CALC"). */
void vida_answer_set(GtkWidget *answer, const char *value, const char *type) {
    GtkWidget *val_lbl  = GTK_WIDGET(g_object_get_data(G_OBJECT(answer), "value-label"));
    GtkWidget *type_lbl = GTK_WIDGET(g_object_get_data(G_OBJECT(answer), "type-label"));
    if (val_lbl)  gtk_label_set_text(GTK_LABEL(val_lbl),  value ? value : "");
    if (type_lbl) gtk_label_set_text(GTK_LABEL(type_lbl), type  ? type  : "");
    gtk_widget_set_visible(answer, TRUE);
}

/* Hide the answer bar. */
void vida_answer_clear(GtkWidget *answer) {
    gtk_widget_set_visible(answer, FALSE);
}

/* ---------- Chat view ---------- */

/* Switch to chat view, set the header command name. Clears prior history. */
void vida_chat_show(const char *cmd_name) {
    if (!s_stack || !s_chat_history || !s_chat_header) return;

    /* Clear old history */
    GtkWidget *child;
    while ((child = gtk_widget_get_first_child(s_chat_history)) != NULL)
        gtk_box_remove(GTK_BOX(s_chat_history), child);

    /* Update header label */
    GtkWidget *lbl = GTK_WIDGET(g_object_get_data(G_OBJECT(s_chat_header), "title-label"));
    if (lbl) gtk_label_set_text(GTK_LABEL(lbl), cmd_name ? cmd_name : "");

    gtk_stack_set_visible_child_name(GTK_STACK(s_stack), "chat");
    if (s_chat_entry) {
        gtk_widget_set_sensitive(s_chat_entry, TRUE);
        gtk_widget_grab_focus(s_chat_entry);
    }
}

/* Switch back to palette view. */
void vida_chat_clear(void) {
    if (!s_stack) return;
    gtk_stack_set_visible_child_name(GTK_STACK(s_stack), "palette");
}

/* Append a message bubble to the chat history.
 * role: "user" or "ai" */
void vida_chat_append_message(const char *role, const char *text) {
    if (!s_chat_history || !role || !text) return;

    int is_user = (strcmp(role, "user") == 0);
    GtkWidget *lbl = gtk_label_new(text);
    gtk_label_set_wrap(GTK_LABEL(lbl), TRUE);
    gtk_label_set_wrap_mode(GTK_LABEL(lbl), PANGO_WRAP_WORD_CHAR);
    gtk_label_set_max_width_chars(GTK_LABEL(lbl), 1);
    gtk_widget_set_hexpand(lbl, TRUE);
    gtk_label_set_xalign(GTK_LABEL(lbl), is_user ? 1.0f : 0.0f);
    gtk_widget_add_css_class(lbl, is_user ? "vida-msg-user" : "vida-msg-ai");
    gtk_box_append(GTK_BOX(s_chat_history), lbl);

    /* Scroll to bottom */
    GtkAdjustment *adj = gtk_scrolled_window_get_vadjustment(
        GTK_SCROLLED_WINDOW(s_chat_scroll));
    gtk_adjustment_set_value(adj, gtk_adjustment_get_upper(adj) -
                                   gtk_adjustment_get_page_size(adj));
}

/* Update the last AI bubble text (streaming update). */
void vida_chat_update_last_ai(const char *text) {
    if (!s_chat_history || !text) return;
    /* Find last child */
    GtkWidget *last = NULL;
    GtkWidget *child = gtk_widget_get_first_child(s_chat_history);
    while (child) { last = child; child = gtk_widget_get_next_sibling(child); }
    if (!last) return;
    /* Only update if it's an AI bubble */
    if (gtk_widget_has_css_class(last, "vida-msg-ai")) {
        gtk_label_set_text(GTK_LABEL(last), text);
        /* Scroll to bottom */
        GtkAdjustment *adj = gtk_scrolled_window_get_vadjustment(
            GTK_SCROLLED_WINDOW(s_chat_scroll));
        gtk_adjustment_set_value(adj, gtk_adjustment_get_upper(adj) -
                                       gtk_adjustment_get_page_size(adj));
    }
}

/* Enable or disable the chat follow-up entry. */
void vida_chat_set_entry_sensitive(gboolean sensitive) {
    if (!s_chat_entry) return;
    gtk_widget_set_sensitive(s_chat_entry, sensitive);
    /* Re-focus the entry when re-enabling so Enter sends the next message. */
    if (sensitive) gtk_widget_grab_focus(s_chat_entry);
}

/* Enter chat/note mode: remove L/R anchors so the window shrinks to panel width
 * (640px, centered by compositor) and allows pointer events to pass through to
 * other apps in the transparent areas. Switch to ON_DEMAND keyboard mode so the
 * user can click other windows freely, then click back to resume typing. */
void vida_enter_chat_mode(GtkWidget *win) {
    gtk_layer_set_anchor(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_LEFT,  FALSE);
    gtk_layer_set_anchor(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_RIGHT, FALSE);
    gtk_layer_set_keyboard_mode(GTK_WINDOW(win),
                                GTK_LAYER_SHELL_KEYBOARD_MODE_ON_DEMAND);
    /* Request compositor-level keyboard focus so typing works immediately. */
    gtk_window_present(GTK_WINDOW(win));
}

/* Return to palette mode: restore full-width overlay and exclusive keyboard. */
void vida_enter_palette_mode(GtkWidget *win) {
    gtk_layer_set_anchor(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_LEFT,  TRUE);
    gtk_layer_set_anchor(GTK_WINDOW(win), GTK_LAYER_SHELL_EDGE_RIGHT, TRUE);
    gtk_layer_set_keyboard_mode(GTK_WINDOW(win),
                                GTK_LAYER_SHELL_KEYBOARD_MODE_EXCLUSIVE);
    gtk_window_present(GTK_WINDOW(win));
}

/* Get chat entry text. */
void vida_chat_entry_get_text(char *buf, int buflen) {
    if (!s_chat_entry) { if (buflen > 0) buf[0] = '\0'; return; }
    const char *text = gtk_editable_get_text(GTK_EDITABLE(s_chat_entry));
    strncpy(buf, text ? text : "", buflen - 1);
    buf[buflen - 1] = '\0';
}

/* Clear chat entry text. */
void vida_chat_entry_clear(void) {
    if (s_chat_entry) gtk_editable_set_text(GTK_EDITABLE(s_chat_entry), "");
}

/* ---------- Note form ---------- */

/* Switch to note form view with optional pre-filled title. */
void vida_note_show(const char *prefill_title) {
    if (!s_stack) return;
    if (s_note_title)
        gtk_editable_set_text(GTK_EDITABLE(s_note_title),
                              prefill_title ? prefill_title : "");
    if (s_note_body_tv) {
        GtkTextBuffer *buf = gtk_text_view_get_buffer(GTK_TEXT_VIEW(s_note_body_tv));
        gtk_text_buffer_set_text(buf, "", 0);
    }
    if (s_note_tags)
        gtk_editable_set_text(GTK_EDITABLE(s_note_tags), "");
    gtk_stack_set_visible_child_name(GTK_STACK(s_stack), "note");
    if (s_note_title) gtk_widget_grab_focus(s_note_title);
}

/* Get note title text. */
void vida_note_get_title(char *buf, int buflen) {
    if (!s_note_title) { if (buflen > 0) buf[0] = '\0'; return; }
    const char *text = gtk_editable_get_text(GTK_EDITABLE(s_note_title));
    strncpy(buf, text ? text : "", buflen - 1);
    buf[buflen - 1] = '\0';
}

/* Get note body text. */
void vida_note_get_body(char *buf, int buflen) {
    if (!s_note_body_tv) { if (buflen > 0) buf[0] = '\0'; return; }
    GtkTextBuffer *tbuf = gtk_text_view_get_buffer(GTK_TEXT_VIEW(s_note_body_tv));
    GtkTextIter start, end;
    gtk_text_buffer_get_bounds(tbuf, &start, &end);
    char *text = gtk_text_buffer_get_text(tbuf, &start, &end, FALSE);
    strncpy(buf, text ? text : "", buflen - 1);
    buf[buflen - 1] = '\0';
    g_free(text);
}

/* Get note tags text. */
void vida_note_get_tags(char *buf, int buflen) {
    if (!s_note_tags) { if (buflen > 0) buf[0] = '\0'; return; }
    const char *text = gtk_editable_get_text(GTK_EDITABLE(s_note_tags));
    strncpy(buf, text ? text : "", buflen - 1);
    buf[buflen - 1] = '\0';
}

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
    /* Walk up to panel, then find separator (third child of panel). */
    GtkWidget *panel = gtk_widget_get_parent(results);
    if (!panel) return;
    GtkWidget *child = gtk_widget_get_first_child(panel);
    /* panel children: entry_row, answer_bar, separator, results */
    if (child) child = gtk_widget_get_next_sibling(child); /* answer_bar */
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
    /* Allow label to shrink below its natural width so panel stays at fixed size. */
    gtk_widget_set_size_request(lbl, 0, -1);

    GtkWidget *type_lbl = gtk_label_new(type_label);
    gtk_widget_add_css_class(type_lbl, "vida-row-type");
    gtk_label_set_xalign(GTK_LABEL(type_lbl), 1.0f);

    gtk_box_append(GTK_BOX(hbox), lbl);
    gtk_box_append(GTK_BOX(hbox), type_lbl);
    gtk_button_set_child(GTK_BUTTON(btn), hbox);
    return btn;
}

/* Show a single text label with a given badge. */
static void results_set_label_badge(GtkWidget *box, const char *text, const char *badge) {
    vida_results_clear(box);
    if (!text || !*text) return;
    GtkWidget *btn = make_row(text, badge);
    gtk_box_append(GTK_BOX(box), btn);
    set_separator_visible(box, TRUE);
}

/* Show a single text label (calc result). */
void vida_results_set_label(GtkWidget *box, const char *text) {
    results_set_label_badge(box, text, "Calculator");
}

/* Show a unit conversion result with a Convert badge. */
void vida_results_set_convert(GtkWidget *box, const char *text) {
    results_set_label_badge(box, text, "Convert");
}

/* Show AI streaming text — word-wrapped, with a small "AI" badge below. */
void vida_results_set_ai_text(GtkWidget *box, const char *text) {
    vida_results_clear(box);
    if (!text || !*text) return;

    GtkWidget *outer = gtk_box_new(GTK_ORIENTATION_VERTICAL, 4);
    gtk_widget_add_css_class(outer, "vida-row");
    gtk_widget_set_hexpand(outer, TRUE);
    gtk_widget_set_size_request(outer, 0, -1);

    GtkWidget *lbl = gtk_label_new(text);
    gtk_widget_add_css_class(lbl, "vida-row-label");
    gtk_label_set_wrap(GTK_LABEL(lbl), TRUE);
    gtk_label_set_wrap_mode(GTK_LABEL(lbl), PANGO_WRAP_WORD_CHAR);
    gtk_label_set_xalign(GTK_LABEL(lbl), 0.0f);
    gtk_label_set_yalign(GTK_LABEL(lbl), 0.0f);
    gtk_widget_set_hexpand(lbl, TRUE);
    /* Suppress natural width so the label wraps within the panel instead of
     * driving the panel wider. hexpand fills the allocated 680px normally. */
    gtk_label_set_max_width_chars(GTK_LABEL(lbl), 1);
    gtk_box_append(GTK_BOX(outer), lbl);

    GtkWidget *badge = gtk_label_new("AI");
    gtk_widget_add_css_class(badge, "vida-row-type");
    gtk_label_set_xalign(GTK_LABEL(badge), 1.0f);
    gtk_box_append(GTK_BOX(outer), badge);

    gtk_box_append(GTK_BOX(box), outer);
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

/* make_app_row creates a result row with a 48px app icon and app name. */
static GtkWidget *make_app_row(const char *name, const char *icon_name) {
    GtkWidget *btn = gtk_button_new();
    gtk_widget_add_css_class(btn, "vida-row");
    gtk_button_set_has_frame(GTK_BUTTON(btn), FALSE);

    GtkWidget *hbox = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
    gtk_widget_set_hexpand(hbox, TRUE);
    gtk_widget_set_valign(hbox, GTK_ALIGN_CENTER);

    /* Icon */
    GtkWidget *img;
    const char *resolved = (icon_name && *icon_name) ? icon_name
                                                      : "application-x-executable-symbolic";
    GIcon *gicon = g_themed_icon_new(resolved);
    img = gtk_image_new_from_gicon(gicon);
    g_object_unref(gicon);
    gtk_image_set_pixel_size(GTK_IMAGE(img), 48);
    gtk_widget_add_css_class(img, "vida-app-icon");
    gtk_box_append(GTK_BOX(hbox), img);

    /* App name */
    GtkWidget *lbl = gtk_label_new(name);
    gtk_widget_add_css_class(lbl, "vida-row-label");
    gtk_label_set_ellipsize(GTK_LABEL(lbl), PANGO_ELLIPSIZE_END);
    gtk_label_set_xalign(GTK_LABEL(lbl), 0.0f);
    gtk_widget_set_hexpand(lbl, TRUE);
    gtk_box_append(GTK_BOX(hbox), lbl);

    /* Type badge */
    GtkWidget *type_lbl = gtk_label_new("App");
    gtk_widget_add_css_class(type_lbl, "vida-row-type");
    gtk_label_set_xalign(GTK_LABEL(type_lbl), 1.0f);
    gtk_box_append(GTK_BOX(hbox), type_lbl);

    gtk_button_set_child(GTK_BUTTON(btn), hbox);
    return btn;
}

/* Show a list of app rows with icons. */
void vida_results_set_apps(GtkWidget *box, const char **names,
                            const char **icons, int n) {
    vida_results_clear(box);
    for (int i = 0; i < n && i < 6; i++) {
        if (!names[i] || !*names[i]) continue;
        const char *icon = (icons && icons[i]) ? icons[i] : "";
        GtkWidget *row = make_app_row(names[i], icon);
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

/* make_cmd_row creates a command result row with a 32px icon, bold name, and muted desc. */
static GtkWidget *make_cmd_row(const char *name, const char *desc, const char *icon_name) {
    GtkWidget *btn = gtk_button_new();
    gtk_widget_add_css_class(btn, "vida-row");
    gtk_button_set_has_frame(GTK_BUTTON(btn), FALSE);

    GtkWidget *hbox = gtk_box_new(GTK_ORIENTATION_HORIZONTAL, 0);
    gtk_widget_set_hexpand(hbox, TRUE);
    gtk_widget_set_valign(hbox, GTK_ALIGN_CENTER);

    /* 32px icon */
    const char *resolved = (icon_name && *icon_name) ? icon_name : "system-run-symbolic";
    GIcon *gicon = g_themed_icon_new(resolved);
    GtkWidget *img = gtk_image_new_from_gicon(gicon);
    g_object_unref(gicon);
    gtk_image_set_pixel_size(GTK_IMAGE(img), 32);
    gtk_widget_add_css_class(img, "vida-app-icon");
    gtk_box_append(GTK_BOX(hbox), img);

    /* Name + desc stacked vertically */
    GtkWidget *vbox = gtk_box_new(GTK_ORIENTATION_VERTICAL, 0);
    gtk_widget_set_hexpand(vbox, TRUE);
    gtk_widget_set_valign(vbox, GTK_ALIGN_CENTER);

    GtkWidget *name_lbl = gtk_label_new(name);
    gtk_widget_add_css_class(name_lbl, "vida-cmd-name");
    gtk_label_set_xalign(GTK_LABEL(name_lbl), 0.0f);
    gtk_box_append(GTK_BOX(vbox), name_lbl);

    if (desc && *desc) {
        GtkWidget *desc_lbl = gtk_label_new(desc);
        gtk_widget_add_css_class(desc_lbl, "vida-cmd-desc");
        gtk_label_set_xalign(GTK_LABEL(desc_lbl), 0.0f);
        gtk_label_set_ellipsize(GTK_LABEL(desc_lbl), PANGO_ELLIPSIZE_END);
        gtk_box_append(GTK_BOX(vbox), desc_lbl);
    }

    gtk_box_append(GTK_BOX(hbox), vbox);
    gtk_button_set_child(GTK_BUTTON(btn), hbox);
    return btn;
}

/* Show a filtered list of command rows. */
void vida_results_set_commands(GtkWidget *box, const char **names,
                                const char **descs, const char **icons, int n) {
    vida_results_clear(box);
    if (n == 0) {
        GtkWidget *row = make_row("No commands match", "");
        gtk_box_append(GTK_BOX(box), row);
        set_separator_visible(box, TRUE);
        return;
    }
    for (int i = 0; i < n; i++) {
        if (!names[i] || !*names[i]) continue;
        const char *desc = (descs && descs[i]) ? descs[i] : "";
        const char *icon = (icons && icons[i]) ? icons[i] : "";
        GtkWidget *row = make_cmd_row(names[i], desc, icon);
        gtk_box_append(GTK_BOX(box), row);
    }
    set_separator_visible(box, TRUE);
}

/* Show a brief "Copied" HUD label in the results area for 1.5 seconds. */
void vida_show_copied_hud(GtkWidget *box) {
    GtkWidget *lbl = gtk_label_new("Copied");
    gtk_widget_add_css_class(lbl, "vida-hud");
    gtk_label_set_xalign(GTK_LABEL(lbl), 0.5f);
    gtk_box_append(GTK_BOX(box), lbl);
    set_separator_visible(box, TRUE);
}

/* Set the entry placeholder text (used when switching modes). */
void vida_entry_set_placeholder(GtkWidget *entry, const char *text) {
    gtk_entry_set_placeholder_text(GTK_ENTRY(entry), text);
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
