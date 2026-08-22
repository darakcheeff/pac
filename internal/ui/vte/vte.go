package vte

/*
#cgo pkg-config: vte-2.91 gtk+-3.0 libpcre2-8
#define PCRE2_CODE_UNIT_WIDTH 8
#include <pcre2.h>
#include <vte/vte.h>
#include <gtk/gtk.h>
#include <pango/pango.h>

static VteTerminal* TO_VTE_TERMINAL(GtkWidget* w) {
    return VTE_TERMINAL(w);
}

static gboolean set_terminal_pty_fd(GtkWidget* term, int fd, GError** error) {
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
	"unsafe"

	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

func init() {
	// Register VteTerminal in gotk3 WrapMap to prevent reflection conversion panics
	gtk.WrapMap["VteTerminal"] = func(obj *glib.Object) *gtk.Widget {
		return &gtk.Widget{InitiallyUnowned: glib.InitiallyUnowned{Object: obj}}
	}
}

// Terminal wraps VteTerminal C widget
type Terminal struct {
	*gtk.Widget
	vteWidget *C.GtkWidget
	vteTerm   *C.VteTerminal
}

// NewTerminal creates a new VTE Terminal widget
func NewTerminal() (*Terminal, error) {
	cWidget := C.vte_terminal_new()
	if cWidget == nil {
		return nil, fmt.Errorf("failed to create vte_terminal")
	}

	cTerm := C.TO_VTE_TERMINAL(cWidget)

	// Set sensible defaults
	C.vte_terminal_set_mouse_autohide(cTerm, C.TRUE)
	C.vte_terminal_set_scroll_on_output(cTerm, C.FALSE)
	C.vte_terminal_set_scroll_on_keystroke(cTerm, C.TRUE)
	C.vte_terminal_set_scrollback_lines(cTerm, 10000)

	// Wrap C GtkWidget into gotk3 *gtk.Widget properly via glib.Take
	glibObj := glib.Take(unsafe.Pointer(cWidget))
	gWidget := &gtk.Widget{InitiallyUnowned: glib.InitiallyUnowned{Object: glibObj}}

	term := &Terminal{
		Widget:    gWidget,
		vteWidget: cWidget,
		vteTerm:   cTerm,
	}

	term.ApplyColorScheme("mate")
	return term, nil
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

// FeedText writes string directly to VTE display buffer
func (t *Terminal) FeedText(text string) {
	cStr := C.CString(text)
	defer C.free(unsafe.Pointer(cStr))
	C.vte_terminal_feed(t.vteTerm, cStr, C.gssize(len(text)))
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

// PasteClipboard pastes clipboard text into terminal
func (t *Terminal) PasteClipboard() {
	C.vte_terminal_paste_clipboard(t.vteTerm)
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
