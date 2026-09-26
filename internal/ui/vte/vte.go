package vte

/*
#cgo pkg-config: vte-2.91 gtk+-3.0 libpcre2-8
#cgo CFLAGS: -D_GNU_SOURCE
#define PCRE2_CODE_UNIT_WIDTH 8
#include <stdlib.h>
#include <fcntl.h>
#include <string.h>
#include <pcre2.h>
#include <vte/vte.h>
#include <gtk/gtk.h>
#include <gdk/gdk.h>
#include <gdk/gdkkeysyms.h>
#include <pango/pango.h>

static VteTerminal* TO_VTE_TERMINAL(GtkWidget* w) {
    return VTE_TERMINAL(w);
}

static glong get_terminal_row_count(GtkWidget* term) {
    return vte_terminal_get_row_count(VTE_TERMINAL(term));
}

static glong get_terminal_column_count(GtkWidget* term) {
    return vte_terminal_get_column_count(VTE_TERMINAL(term));
}

static char* get_clean_text(VteTerminal* term) {
    if (!term) return NULL;
    return vte_terminal_get_text_format(term, VTE_FORMAT_TEXT);
}

static void paste_clean_text(VteTerminal* term, GdkAtom selection) {
    GtkClipboard* clipboard = gtk_clipboard_get(selection);
    if (!clipboard) return;

    gchar* text = gtk_clipboard_wait_for_text(clipboard);
    if (text) {
        GString* s = g_string_new(text);
        g_string_replace(s, "\x1b[200~", "", 0);
        g_string_replace(s, "\x1b[201~", "", 0);
        g_string_replace(s, "^[[200~", "", 0);
        g_string_replace(s, "^[[201~", "", 0);

        vte_terminal_paste_text(term, s->str);
        g_string_free(s, TRUE);
        g_free(text);
    }
}

static void reset_terminal_state(VteTerminal* term) {
    vte_terminal_reset(term, TRUE, FALSE);
    const char* reset_seq = "\x1b[?2004l\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1049l\x1b[0m\x0f";
    vte_terminal_feed(term, reset_seq, strlen(reset_seq));
}

static gboolean on_vte_button_press(GtkWidget* widget, GdkEventButton* event, gpointer user_data) {
    // Intercept middle-click paste to prevent bracketed paste markers (^[[200~ / ^[[201~)
    if (event->button == GDK_BUTTON_MIDDLE) {
        paste_clean_text(VTE_TERMINAL(widget), GDK_SELECTION_PRIMARY);
        return TRUE; // Consume event completely
    }
    return FALSE;
}

extern int goOnVteKeyPress(GtkWidget* widget, guint keyval);
extern void goOnVteDirectoryChanged(GtkWidget* widget, char* uri);
extern void goOnVteTitleChanged(GtkWidget* widget, char* title);
extern void goOnVteContentsChanged(GtkWidget* widget);

static void on_vte_contents_changed(VteTerminal* term, gpointer user_data) {
    goOnVteContentsChanged((GtkWidget*)term);
}

static void on_vte_directory_uri_changed(VteTerminal* term, gpointer user_data) {
    const char* uri = vte_terminal_get_current_directory_uri(term);
    if (uri) {
        goOnVteDirectoryChanged((GtkWidget*)term, (char*)uri);
    }
}

static void on_vte_window_title_changed(VteTerminal* term, gpointer user_data) {
    const char* title = vte_terminal_get_window_title(term);
    if (title) {
        goOnVteTitleChanged((GtkWidget*)term, (char*)title);
    }
}

static gboolean on_vte_key_press(GtkWidget* widget, GdkEventKey* event, gpointer user_data) {
    if (goOnVteKeyPress(widget, event->keyval)) {
        return TRUE;
    }

    // Ctrl+Shift+V or Shift+Insert: Paste from clipboard with native bracketed paste support
    if (((event->state & GDK_CONTROL_MASK) && (event->state & GDK_SHIFT_MASK) && (event->keyval == GDK_KEY_V || event->keyval == GDK_KEY_v)) ||
        ((event->state & GDK_SHIFT_MASK) && (event->keyval == GDK_KEY_Insert || event->keyval == GDK_KEY_KP_Insert))) {
        paste_clean_text(VTE_TERMINAL(widget), GDK_SELECTION_CLIPBOARD);
        return TRUE;
    }

    // Ctrl+Shift+C: Copy selected text to clipboard
    if ((event->state & GDK_CONTROL_MASK) && (event->state & GDK_SHIFT_MASK) && (event->keyval == GDK_KEY_C || event->keyval == GDK_KEY_c)) {
        vte_terminal_copy_clipboard_format(VTE_TERMINAL(widget), VTE_FORMAT_TEXT);
        return TRUE;
    }

    // Ctrl+Shift+R or Ctrl+Shift+K: Reset terminal state (clears stuck bracketed paste and mouse modes)
    if ((event->state & GDK_CONTROL_MASK) && (event->state & GDK_SHIFT_MASK) && 
        (event->keyval == GDK_KEY_R || event->keyval == GDK_KEY_r || event->keyval == GDK_KEY_K || event->keyval == GDK_KEY_k)) {
        reset_terminal_state(VTE_TERMINAL(widget));
        return TRUE;
    }

    // Pass all other keys cleanly to VTE without client interception
    // If Control or Alt is pressed, let standard terminal key combinations pass through to VTE
    if ((event->state & GDK_CONTROL_MASK) || (event->state & GDK_MOD1_MASK)) {
        return FALSE;
    }

    // Intercept Tab / KP_Tab for terminal autocomplete and prevent GTK focus traversal
    if (event->keyval == GDK_KEY_Tab || event->keyval == GDK_KEY_KP_Tab) {
        vte_terminal_feed_child(VTE_TERMINAL(widget), "\t", 1);
        return TRUE; // Consume event completely
    }

    // Intercept Shift+Tab (Backtab)
    if (event->keyval == GDK_KEY_ISO_Left_Tab) {
        vte_terminal_feed_child(VTE_TERMINAL(widget), "\x1b[Z", 3);
        return TRUE; // Consume event
    }

    return FALSE;
}

// Filter spurious any-event mouse tracking codes (DECSET 1003 / code 35: ESC[<35;X;YM)
// when the mouse is simply hovering or moving across the terminal with no buttons pressed.
// Button-motion (dragging, text selection, code 32), button clicks (code 0/1/2), and
// mouse wheel scrolling (scroll-event, code 64/65) pass through untouched.
static gboolean on_vte_motion_notify(GtkWidget* widget, GdkEventMotion* event, gpointer user_data) {
    if (!event) return FALSE;
    // If no mouse button is held down, consume the event so VTE doesn't generate code 35 spam
    if ((event->state & (GDK_BUTTON1_MASK | GDK_BUTTON2_MASK | GDK_BUTTON3_MASK | GDK_BUTTON4_MASK | GDK_BUTTON5_MASK)) == 0) {
        return TRUE; // Consume event completely
    }
    return FALSE; // Allow button-motion (drag-selection)
}

static void configure_vte_terminal(GtkWidget* w) {
    VteTerminal* term = VTE_TERMINAL(w);

    gtk_widget_set_can_focus(w, TRUE);
    gtk_widget_set_can_default(w, TRUE);

    gtk_widget_add_events(w, GDK_POINTER_MOTION_MASK | GDK_LEAVE_NOTIFY_MASK);

    vte_terminal_set_mouse_autohide(term, TRUE);
    vte_terminal_set_bold_is_bright(term, TRUE);
    vte_terminal_set_scroll_on_keystroke(term, TRUE);
    vte_terminal_set_scroll_on_output(term, FALSE);
    vte_terminal_set_scrollback_lines(term, 10000);
    vte_terminal_set_backspace_binding(term, VTE_ERASE_AUTO);
    vte_terminal_set_delete_binding(term, VTE_ERASE_DELETE_SEQUENCE);
    vte_terminal_set_cursor_shape(term, VTE_CURSOR_SHAPE_BLOCK);
    vte_terminal_set_cursor_blink_mode(term, VTE_CURSOR_BLINK_SYSTEM);
    vte_terminal_set_audible_bell(term, FALSE);

    g_signal_connect(w, "key-press-event", G_CALLBACK(on_vte_key_press), NULL);
    g_signal_connect(w, "button-press-event", G_CALLBACK(on_vte_button_press), NULL);
    g_signal_connect(w, "motion-notify-event", G_CALLBACK(on_vte_motion_notify), NULL);
    g_signal_connect(w, "current-directory-uri-changed", G_CALLBACK(on_vte_directory_uri_changed), NULL);
    g_signal_connect(w, "window-title-changed", G_CALLBACK(on_vte_window_title_changed), NULL);
    g_signal_connect(w, "contents-changed", G_CALLBACK(on_vte_contents_changed), NULL);
}

static int create_vte_native_pty(GtkWidget* term, char* slave_path, size_t slave_path_len, GError** error) {
    VtePty* pty = vte_pty_new_sync(VTE_PTY_DEFAULT, NULL, error);
    if (!pty) {
        return -1;
    }
    vte_pty_set_utf8(pty, TRUE, NULL);
    vte_terminal_set_pty(VTE_TERMINAL(term), pty);

    int master_fd = vte_pty_get_fd(pty);
    if (master_fd < 0) {
        g_object_unref(pty);
        return -1;
    }

    if (grantpt(master_fd) != 0 || unlockpt(master_fd) != 0) {
        g_object_unref(pty);
        return -1;
    }

    char* sname = ptsname(master_fd);
    if (!sname) {
        g_object_unref(pty);
        return -1;
    }

    strncpy(slave_path, sname, slave_path_len - 1);
    slave_path[slave_path_len - 1] = '\0';
    g_object_unref(pty);
    return master_fd;
}

static gboolean set_terminal_pty_fd(GtkWidget* term, int fd, GError** error) {
    int flags = fcntl(fd, F_GETFL, 0);
    if (flags >= 0) {
        fcntl(fd, F_SETFL, flags | O_NONBLOCK);
    }
    VtePty* pty = vte_pty_new_foreign_sync(fd, NULL, error);
    if (!pty) {
        return FALSE;
    }
    vte_terminal_set_pty(VTE_TERMINAL(term), pty);
    g_object_unref(pty);
    return TRUE;
}

static gboolean set_terminal_search_pattern(GtkWidget* term, const char* pattern, gboolean case_sensitive) {
    if (!pattern || strlen(pattern) == 0) {
        vte_terminal_search_set_regex(VTE_TERMINAL(term), NULL, 0);
        return TRUE;
    }
    guint32 flags = PCRE2_MULTILINE;
    if (!case_sensitive) {
        flags |= PCRE2_CASELESS;
    }
    GError* error = NULL;
    VteRegex* regex = vte_regex_new_for_search(pattern, -1, flags, &error);
    if (!regex) {
        if (error) g_error_free(error);
        return FALSE;
    }
    vte_terminal_search_set_regex(VTE_TERMINAL(term), regex, 0);
    vte_regex_unref(regex);
    return TRUE;
}

static void set_terminal_scheme_colors(GtkWidget* term, const char* scheme_name) {
    GdkRGBA fg, bg;
    if (g_strcmp0(scheme_name, "solarized-dark") == 0) {
        gdk_rgba_parse(&fg, "#839496");
        gdk_rgba_parse(&bg, "#002b36");
    } else if (g_strcmp0(scheme_name, "dracula") == 0) {
        gdk_rgba_parse(&fg, "#f8f8f2");
        gdk_rgba_parse(&bg, "#282a36");
    } else if (g_strcmp0(scheme_name, "monokai") == 0) {
        gdk_rgba_parse(&fg, "#f8f8f2");
        gdk_rgba_parse(&bg, "#272822");
    } else if (g_strcmp0(scheme_name, "white-on-black") == 0) {
        gdk_rgba_parse(&fg, "#ffffff");
        gdk_rgba_parse(&bg, "#000000");
    } else if (g_strcmp0(scheme_name, "black-on-white") == 0) {
        gdk_rgba_parse(&fg, "#000000");
        gdk_rgba_parse(&bg, "#ffffff");
    } else { // default mate-terminal dark
        gdk_rgba_parse(&fg, "#d0d0d0");
        gdk_rgba_parse(&bg, "#1a1a1a");
    }
    vte_terminal_set_colors(VTE_TERMINAL(term), &fg, &bg, NULL, 0);
}
*/
import "C"

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unsafe"

	"github.com/darakcheeff/pac/internal/engine/pty"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	"golang.org/x/sys/unix"
)

func init() {
	// Register VteTerminal in gotk3 WrapMap to prevent reflection conversion panics
	gtk.WrapMap["VteTerminal"] = func(obj *glib.Object) *gtk.Widget {
		return &gtk.Widget{InitiallyUnowned: glib.InitiallyUnowned{Object: obj}}
	}
}

var (
	termRegistryMu sync.Mutex
	termRegistry   = make(map[uintptr]*Terminal)
)

func registerTerminal(t *Terminal) {
	termRegistryMu.Lock()
	defer termRegistryMu.Unlock()
	termRegistry[uintptr(unsafe.Pointer(t.vteWidget))] = t
}

func unregisterTerminal(t *Terminal) {
	termRegistryMu.Lock()
	defer termRegistryMu.Unlock()
	delete(termRegistry, uintptr(unsafe.Pointer(t.vteWidget)))
}

var titleDirRegex = regexp.MustCompile(`(?:[^;@]*@)?[^;]*:\s*([/~][^\s]*)`)

func parseURIPath(rawURI string) string {
	rawURI = strings.TrimSpace(rawURI)
	if rawURI == "" {
		return ""
	}
	u, err := url.Parse(rawURI)
	if err == nil && u.Path != "" {
		return filepath.Clean(u.Path)
	}
	if strings.HasPrefix(rawURI, "file://") {
		idx := strings.Index(rawURI[7:], "/")
		if idx != -1 {
			return filepath.Clean(rawURI[7+idx:])
		}
	}
	return ""
}

func parseTitleDir(title string) string {
	matches := titleDirRegex.FindStringSubmatch(title)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

//export goOnVteDirectoryChanged
func goOnVteDirectoryChanged(widget *C.GtkWidget, cUri *C.char) {
	if cUri == nil {
		return
	}
	rawUri := C.GoString(cUri)
	termRegistryMu.Lock()
	t := termRegistry[uintptr(unsafe.Pointer(widget))]
	termRegistryMu.Unlock()
	if t == nil {
		return
	}
	parsed := parseURIPath(rawUri)
	if parsed != "" {
		t.mu.Lock()
		cb := t.OnDirectoryChanged
		t.mu.Unlock()
		if cb != nil {
			cb(parsed)
		}
	}
}

//export goOnVteTitleChanged
func goOnVteTitleChanged(widget *C.GtkWidget, cTitle *C.char) {
	if cTitle == nil {
		return
	}
	title := C.GoString(cTitle)
	termRegistryMu.Lock()
	t := termRegistry[uintptr(unsafe.Pointer(widget))]
	termRegistryMu.Unlock()
	if t == nil {
		return
	}
	dir := parseTitleDir(title)
	if dir != "" {
		t.mu.Lock()
		cb := t.OnDirectoryChanged
		t.mu.Unlock()
		if cb != nil {
			cb(dir)
		}
	}
}

//export goOnVteContentsChanged
func goOnVteContentsChanged(widget *C.GtkWidget) {
	termRegistryMu.Lock()
	t := termRegistry[uintptr(unsafe.Pointer(widget))]
	termRegistryMu.Unlock()
	if t == nil {
		return
	}
	t.mu.Lock()
	cb := t.OnContentsChanged
	t.mu.Unlock()
	if cb != nil {
		cb()
	}
}

//export goOnVteKeyPress
func goOnVteKeyPress(widget *C.GtkWidget, keyval C.guint) C.int {
	termRegistryMu.Lock()
	t := termRegistry[uintptr(unsafe.Pointer(widget))]
	termRegistryMu.Unlock()

	if t == nil {
		return 0
	}

	if t.IsDisconnected() {
		if keyval == C.GDK_KEY_Return || keyval == C.GDK_KEY_KP_Enter {
			t.mu.Lock()
			handler := t.OnReconnect
			t.mu.Unlock()
			if handler != nil {
				glib.IdleAdd(func() {
					handler()
				})
			}
		}
		return 1
	}

	return 0
}

// Terminal wraps VteTerminal C widget
type Terminal struct {
	*gtk.Widget
	vteWidget          *C.GtkWidget
	vteTerm            *C.VteTerminal
	OnResize           func(rows, cols int)
	OnReconnect        func()
	OnDirectoryChanged func(path string)
	OnContentsChanged  func()
	isDisconnected     bool
	mu                 sync.Mutex
}

// SetDisconnected updates disconnected state and reconnect callback
func (t *Terminal) SetDisconnected(disconnected bool, onReconnect func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.isDisconnected = disconnected
	t.OnReconnect = onReconnect
}

// IsDisconnected returns true if terminal is currently waiting for reconnect
func (t *Terminal) IsDisconnected() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.isDisconnected
}

// NewTerminal creates a new VTE Terminal widget matching mate-terminal specifications
func NewTerminal() (*Terminal, error) {
	cWidget := C.vte_terminal_new()
	if cWidget == nil {
		return nil, fmt.Errorf("failed to create vte_terminal")
	}

	cTerm := C.TO_VTE_TERMINAL(cWidget)

	// Configure terminal defaults exactly like mate-terminal
	C.configure_vte_terminal(cWidget)

	// Wrap C GtkWidget into gotk3 *gtk.Widget properly via glib.Take
	glibObj := glib.Take(unsafe.Pointer(cWidget))
	gWidget := &gtk.Widget{InitiallyUnowned: glib.InitiallyUnowned{Object: glibObj}}

	term := &Terminal{
		Widget:    gWidget,
		vteWidget: cWidget,
		vteTerm:   cTerm,
	}

	// Connect size-allocate to notify Go of window resize and update SIGWINCH / PTY size
	gWidget.Connect("size-allocate", func() {
		rows := term.GetRowCount()
		cols := term.GetColumnCount()
		if rows > 0 && cols > 0 && term.OnResize != nil {
			term.OnResize(rows, cols)
		}
	})

	registerTerminal(term)

	term.ApplyColorScheme("mate")
	term.Widget.SetSizeRequest(10, 10)
	return term, nil
}

// SetupNativePTY initializes native VTE PTY and returns the opened slave *os.File in RAW mode
func (t *Terminal) SetupNativePTY() (*os.File, error) {
	var err *C.GError
	var slavePathBuf [512]C.char

	masterFd := C.create_vte_native_pty(
		t.vteWidget,
		&slavePathBuf[0],
		C.size_t(len(slavePathBuf)),
		&err,
	)
	if masterFd < 0 {
		errMsg := "unknown error"
		if err != nil {
			errMsg = C.GoString(err.message)
			C.g_error_free(err)
		}
		return nil, fmt.Errorf("failed to create native VtePty: %s", errMsg)
	}

	slavePath := C.GoString(&slavePathBuf[0])
	slaveFd, oErr := unix.Open(slavePath, unix.O_RDWR|unix.O_NOCTTY, 0)
	if oErr != nil {
		return nil, fmt.Errorf("failed to open PTY slave %s: %w", slavePath, oErr)
	}
	slaveFile := os.NewFile(uintptr(slaveFd), slavePath)

	// Put slave into raw mode
	termios, tErr := unix.IoctlGetTermios(int(slaveFile.Fd()), unix.TCGETS)
	if tErr == nil {
		termios.Iflag &^= (unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON)
		termios.Oflag &^= unix.OPOST
		termios.Lflag &^= (unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN)
		termios.Cflag &^= (unix.CSIZE | unix.PARENB)
		termios.Cflag |= unix.CS8
		termios.Cc[unix.VMIN] = 1
		termios.Cc[unix.VTIME] = 0
		_ = unix.IoctlSetTermios(int(slaveFile.Fd()), unix.TCSETS, termios)
	}

	return slaveFile, nil
}

// GetRowCount returns current visible row count of terminal
func (t *Terminal) GetRowCount() int {
	return int(C.get_terminal_row_count(t.vteWidget))
}

// GetColumnCount returns current visible column count of terminal
func (t *Terminal) GetColumnCount() int {
	return int(C.get_terminal_column_count(t.vteWidget))
}

// GrabFocus gives keyboard focus directly to the terminal
func (t *Terminal) GrabFocus() {
	if t.Widget != nil {
		t.Widget.GrabFocus()
	}
}

// SetPTYFD attaches an open PTY file descriptor to the VTE terminal
func (t *Terminal) SetPTYFD(fd int) error {
	var err *C.GError
	ok := C.set_terminal_pty_fd(t.vteWidget, C.int(fd), &err)
	if ok == C.FALSE {
		if err != nil {
			defer C.g_error_free(err)
			return fmt.Errorf("vte set pty error: %s", C.GoString(err.message))
		}
		return fmt.Errorf("failed to set pty fd on vte terminal")
	}
	return nil
}

// AttachPTY connects a master PTY file descriptor to VTE
func (t *Terminal) AttachPTY(bridge *pty.PTYBridge) error {
	if bridge == nil || bridge.Master == nil {
		return fmt.Errorf("invalid pty bridge")
	}
	return t.SetPTYFD(int(bridge.Master.Fd()))
}

// GetCleanText returns the clean plain-text content of the terminal without ANSI escape codes
func (t *Terminal) GetCleanText() string {
	if t.vteTerm == nil {
		return ""
	}
	cStr := C.get_clean_text(t.vteTerm)
	if cStr == nil {
		return ""
	}
	defer C.g_free(C.gpointer(cStr))
	return C.GoString(cStr)
}

// FeedText writes string directly to VTE display buffer
func (t *Terminal) FeedText(text string) {
	cStr := C.CString(text)
	defer C.free(unsafe.Pointer(cStr))
	C.vte_terminal_feed(t.vteTerm, cStr, C.gssize(len(text)))
}

// FeedChild writes raw string to the child PTY process (stdin)
func (t *Terminal) FeedChild(text string) {
	cStr := C.CString(text)
	defer C.free(unsafe.Pointer(cStr))
	C.vte_terminal_feed_child(t.vteTerm, cStr, C.gssize(len(text)))
}

// SetScrollbackLines updates maximum scrollback buffer depth
func (t *Terminal) SetScrollbackLines(lines int) {
	C.vte_terminal_set_scrollback_lines(t.vteTerm, C.glong(lines))
}

// SetFont sets custom font family and size
func (t *Terminal) SetFont(fontDesc string) {
	if fontDesc == "" {
		fontDesc = "Monospace 11"
	}
	cFont := C.CString(fontDesc)
	defer C.free(unsafe.Pointer(cFont))

	pangoDesc := C.pango_font_description_from_string(cFont)
	if pangoDesc != nil {
		C.vte_terminal_set_font(t.vteTerm, pangoDesc)
		C.pango_font_description_free(pangoDesc)
	}
}

// CopyClipboard copies selection to clipboard
func (t *Terminal) CopyClipboard() {
	C.vte_terminal_copy_clipboard_format(t.vteTerm, C.VTE_FORMAT_TEXT)
}

// PasteClipboard pastes clean clipboard text into terminal without bracketed paste garbage
func (t *Terminal) PasteClipboard() {
	C.paste_clean_text(t.vteTerm, C.GDK_SELECTION_CLIPBOARD)
}

// PastePrimary pastes clean primary selection (middle click) into terminal
func (t *Terminal) PastePrimary() {
	C.paste_clean_text(t.vteTerm, C.GDK_SELECTION_PRIMARY)
}

// SelectAll selects entire terminal content
func (t *Terminal) SelectAll() {
	C.vte_terminal_select_all(t.vteTerm)
}

// Reset clears terminal state
func (t *Terminal) Reset(clearHistory bool) {
	var clear C.gboolean = C.FALSE
	if clearHistory {
		clear = C.TRUE
	}
	C.vte_terminal_reset(t.vteTerm, clear, clear)
}

// ResetTerminal clears terminal state and resets stuck bracketed paste and mouse tracking modes
func (t *Terminal) ResetTerminal() {
	C.reset_terminal_state(t.vteTerm)
}

// SearchSetPattern configures search regex
func (t *Terminal) SearchSetPattern(pattern string, caseSensitive bool) bool {
	cPattern := C.CString(pattern)
	defer C.free(unsafe.Pointer(cPattern))

	var cs C.gboolean = C.FALSE
	if caseSensitive {
		cs = C.TRUE
	}

	return C.set_terminal_search_pattern(t.vteWidget, cPattern, cs) == C.TRUE
}

// SearchFindNext finds next match forward
func (t *Terminal) SearchFindNext() bool {
	return C.vte_terminal_search_find_next(t.vteTerm) == C.TRUE
}

// SearchFindPrevious finds previous match backward
func (t *Terminal) SearchFindPrevious() bool {
	return C.vte_terminal_search_find_previous(t.vteTerm) == C.TRUE
}

// ApplyColorScheme applies color palette to terminal
func (t *Terminal) ApplyColorScheme(scheme string) {
	cScheme := C.CString(scheme)
	defer C.free(unsafe.Pointer(cScheme))
	C.set_terminal_scheme_colors(t.vteWidget, cScheme)
}
