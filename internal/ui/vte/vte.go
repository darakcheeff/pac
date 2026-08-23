package vte

/*
#cgo pkg-config: vte-2.91 gtk+-3.0 libpcre2-8
#define PCRE2_CODE_UNIT_WIDTH 8
#define _XOPEN_SOURCE 600
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

static void paste_clean_text(VteTerminal* term, GdkAtom selection) {
    GtkClipboard* clipboard = gtk_clipboard_get(selection);
    if (!clipboard) return;

    gchar* text = gtk_clipboard_wait_for_text(clipboard);
    if (text) {
        vte_terminal_feed_child(term, text, strlen(text));
        g_free(text);
    }
}

static gboolean on_vte_button_press(GtkWidget* widget, GdkEventButton* event, gpointer user_data) {
    // Intercept middle-click paste to prevent bracketed paste markers (^[[200~ / ^[[201~)
    if (event->button == GDK_BUTTON_MIDDLE) {
        paste_clean_text(VTE_TERMINAL(widget), GDK_SELECTION_PRIMARY);
        return TRUE; // Consume event completely
    }
    return FALSE;
}

static gboolean on_vte_key_press(GtkWidget* widget, GdkEventKey* event, gpointer user_data) {
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

static void configure_vte_terminal(GtkWidget* w) {
    VteTerminal* term = VTE_TERMINAL(w);

    gtk_widget_set_can_focus(w, TRUE);
    gtk_widget_set_can_default(w, TRUE);

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
    g_object_unref(pty); // Terminal holds its own reference to pty
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

static void search_terminal_previous(GtkWidget* term) {
    vte_terminal_search_find_previous(VTE_TERMINAL(term));
}

static void search_terminal_next(GtkWidget* term) {
    vte_terminal_search_find_next(VTE_TERMINAL(term));
}
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"github.com/darakcheeff/pac/internal/engine/pty"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	"github.com/gotk3/gotk3/pango"
	"golang.org/x/sys/unix"
)

// Terminal wraps libvte GtkWidget
type Terminal struct {
	Widget   *gtk.Widget
	OnResize func(rows, cols int)
}

// NewTerminal creates an instance of VteTerminal
func NewTerminal() (*Terminal, error) {
	obj, err := glib.ObjectNew(glib.TypeFromName("VteTerminal"))
	if err != nil {
		return nil, fmt.Errorf("failed to create VteTerminal: %w", err)
	}

	gWidget := &gtk.Widget{InitiallyUnowned: glib.InitiallyUnowned{Object: obj}}
	C.configure_vte_terminal(C.TO_VTE_TERMINAL(unsafe.Pointer(gWidget.GWidget)))

	term := &Terminal{
		Widget: gWidget,
	}

	// Connect size-allocate to notify Go of window resize
	gWidget.Connect("size-allocate", func() {
		rows := term.GetRowCount()
		cols := term.GetColumnCount()
		if rows > 0 && cols > 0 && term.OnResize != nil {
			term.OnResize(rows, cols)
		}
	})

	return term, nil
}

// SetupNativePTY initializes native VTE PTY and returns the opened slave *os.File in RAW mode
func (t *Terminal) SetupNativePTY() (*os.File, error) {
	var cErr *C.GError
	var slavePathBuf [512]C.char

	masterFd := C.create_vte_native_pty(
		C.TO_VTE_TERMINAL(unsafe.Pointer(t.Widget.GWidget)),
		&slavePathBuf[0],
		C.size_t(len(slavePathBuf)),
		&cErr,
	)
	if masterFd < 0 {
		errMsg := "unknown error"
		if cErr != nil {
			errMsg = C.GoString(cErr.message)
			C.g_error_free(cErr)
		}
		return nil, fmt.Errorf("failed to create native VtePty: %s", errMsg)
	}

	slavePath := C.GoString(&slavePathBuf[0])
	slaveFile, err := os.OpenFile(slavePath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open PTY slave %s: %w", slavePath, err)
	}

	// Put slave into raw mode
	termios, err := unix.IoctlGetTermios(int(slaveFile.Fd()), unix.TCGETS)
	if err == nil {
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

// SetPTYFD connects a master PTY file descriptor to VTE
func (t *Terminal) SetPTYFD(fd int) error {
	var cErr *C.GError
	success := C.set_terminal_pty_fd(C.TO_VTE_TERMINAL(unsafe.Pointer(t.Widget.GWidget)), C.int(fd), &cErr)
	if !bool(success) {
		defer C.g_error_free(cErr)
		return errors.New(C.GoString(cErr.message))
	}
	return nil
}

// AttachPTY connects a master PTY file descriptor to VTE
func (t *Terminal) AttachPTY(bridge *pty.PTYBridge) error {
	if bridge == nil || bridge.Master == nil {
		return errors.New("invalid pty bridge")
	}
	return t.SetPTYFD(int(bridge.Master.Fd()))
}

// SetFont changes terminal font face and size
func (t *Terminal) SetFont(fontDescStr string) {
	if fontDescStr == "" {
		fontDescStr = "Monospace 11"
	}
	pangoDesc := pango.FontDescriptionFromString(fontDescStr)
	if pangoDesc != nil {
		cDesc := (*C.PangoFontDescription)(unsafe.Pointer(pangoDesc.Native()))
		C.vte_terminal_set_font(C.TO_VTE_TERMINAL(unsafe.Pointer(t.Widget.GWidget)), cDesc)
	}
}

// ApplyColorScheme applies background, foreground, and 16 palette colors
func (t *Terminal) ApplyColorScheme(schemeName string) {
	scheme := GetColorScheme(schemeName)

	bg := parseGdkRGBA(scheme.Background)
	fg := parseGdkRGBA(scheme.Foreground)

	var palette [16]C.GdkRGBA
	for i, cHex := range scheme.Palette {
		if i >= 16 {
			break
		}
		cRGBA := parseGdkRGBA(cHex)
		palette[i] = *(*C.GdkRGBA)(unsafe.Pointer(cRGBA.Native()))
	}

	cBg := (*C.GdkRGBA)(unsafe.Pointer(bg.Native()))
	cFg := (*C.GdkRGBA)(unsafe.Pointer(fg.Native()))

	C.vte_terminal_set_colors(
		C.TO_VTE_TERMINAL(unsafe.Pointer(t.Widget.GWidget)),
		cFg,
		cBg,
		&palette[0],
		16,
	)
}

func parseGdkRGBA(hex string) *gdk.RGBA {
	rgba := gdk.NewRGBA()
	if ok := rgba.Parse(hex); !ok {
		_ = rgba.Parse("#000000")
	}
	return rgba
}

// GetRowCount returns current visible terminal rows
func (t *Terminal) GetRowCount() int {
	return int(C.get_terminal_row_count(C.TO_VTE_TERMINAL(unsafe.Pointer(t.Widget.GWidget))))
}

// GetColumnCount returns current visible terminal columns
func (t *Terminal) GetColumnCount() int {
	return int(C.get_terminal_column_count(C.TO_VTE_TERMINAL(unsafe.Pointer(t.Widget.GWidget))))
}

// SearchSetPattern configures text search query
func (t *Terminal) SearchSetPattern(pattern string, caseSensitive bool) error {
	cPattern := C.CString(pattern)
	defer C.free(unsafe.Pointer(cPattern))

	cCase := C.gboolean(0)
	if caseSensitive {
		cCase = C.gboolean(1)
	}

	success := C.set_terminal_search_pattern(C.TO_VTE_TERMINAL(unsafe.Pointer(t.Widget.GWidget)), cPattern, cCase)
	if !bool(success) {
		return errors.New("invalid search regex pattern")
	}
	return nil
}

// SearchPrevious navigates to earlier match
func (t *Terminal) SearchPrevious() {
	C.search_terminal_previous(C.TO_VTE_TERMINAL(unsafe.Pointer(t.Widget.GWidget)))
}

// SearchNext navigates to next match
func (t *Terminal) SearchNext() {
	C.search_terminal_next(C.TO_VTE_TERMINAL(unsafe.Pointer(t.Widget.GWidget)))
}

// GrabFocus assigns keyboard focus directly to VTE terminal
func (t *Terminal) GrabFocus() {
	if t.Widget != nil {
		t.Widget.GrabFocus()
	}
}

// FeedText writes text into terminal emulator for rendering
func (t *Terminal) FeedText(text string) {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))
	C.vte_terminal_feed(C.TO_VTE_TERMINAL(unsafe.Pointer(t.Widget.GWidget)), cText, C.glong(len(text)))
}

// CopyClipboard copies active terminal selection to clipboard
func (t *Terminal) CopyClipboard() {
	C.vte_terminal_copy_clipboard_format(C.TO_VTE_TERMINAL(unsafe.Pointer(t.Widget.GWidget)), C.VTE_FORMAT_TEXT)
}

// PasteClipboard pastes text from system clipboard to terminal
func (t *Terminal) PasteClipboard() {
	C.vte_terminal_paste_clipboard(C.TO_VTE_TERMINAL(unsafe.Pointer(t.Widget.GWidget)))
}
