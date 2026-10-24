package ui

/*
#cgo pkg-config: ayatana-appindicator3-0.1 gio-2.0
#include <libayatana-appindicator/app-indicator.h>
#include <gtk/gtk.h>
#include <gio/gio.h>
#include <stdlib.h>

extern void pacTrayActivateCallback(void);

static GtkStatusIcon* (*pac_orig_fallback)(AppIndicator *indicator) = NULL;

static void pac_status_icon_activate_cb(GtkStatusIcon *si, gpointer user_data) {
    (void)si;
    (void)user_data;
    pacTrayActivateCallback();
}

static GtkStatusIcon* pac_custom_fallback(AppIndicator *indicator) {
    GtkStatusIcon *si = NULL;
    if (pac_orig_fallback) {
        si = pac_orig_fallback(indicator);
    }
    if (si) {
        gtk_status_icon_set_tooltip_text(si, "PacNG");
        gtk_status_icon_set_title(si, "PacNG");
        g_signal_connect(si, "activate", G_CALLBACK(pac_status_icon_activate_cb), NULL);
    }
    return si;
}

static void pac_init_indicator_class(void) {
    static int initialized = 0;
    if (!initialized) {
        initialized = 1;
        gpointer klass = g_type_class_ref(APP_INDICATOR_TYPE);
        if (klass) {
            AppIndicatorClass *ind_class = APP_INDICATOR_CLASS(klass);
            pac_orig_fallback = ind_class->fallback;
            ind_class->fallback = pac_custom_fallback;
        }
    }
}

static GDBusMessage* pac_dbus_sni_filter(GDBusConnection *connection,
                                         GDBusMessage *message,
                                         gboolean incoming,
                                         gpointer user_data) {
    (void)user_data;
    if (incoming && g_dbus_message_get_message_type(message) == G_DBUS_MESSAGE_TYPE_METHOD_CALL) {
        const gchar *member = g_dbus_message_get_member(message);
        if (member && g_strcmp0(member, "Activate") == 0) {
            pacTrayActivateCallback();
            GDBusMessage *reply = g_dbus_message_new_method_reply(message);
            if (reply) {
                g_dbus_connection_send_message(connection, reply, G_DBUS_SEND_MESSAGE_FLAGS_NONE, NULL, NULL);
                g_object_unref(reply);
            }
            g_object_unref(message);
            return NULL;
        }
    }
    return message;
}

static void pac_attach_dbus_filter(void) {
    static int filter_attached = 0;
    if (!filter_attached) {
        filter_attached = 1;
        GError *err = NULL;
        GDBusConnection *conn = g_bus_get_sync(G_BUS_TYPE_SESSION, NULL, &err);
        if (conn) {
            g_dbus_connection_add_filter(conn, pac_dbus_sni_filter, NULL, NULL);
            g_object_unref(conn);
        } else if (err) {
            g_error_free(err);
        }
    }
}

static AppIndicator* pac_create_indicator(const char* icon_path) {
    AppIndicator* ind = app_indicator_new(
        "pac-connection-manager",
        "pac-tray",
        APP_INDICATOR_CATEGORY_APPLICATION_STATUS
    );
    app_indicator_set_status(ind, APP_INDICATOR_STATUS_ACTIVE);
    // Use the provided icon path as the icon theme path + name
    app_indicator_set_icon_theme_path(ind, icon_path); // directory
    return ind;
}

static void pac_set_indicator_title(AppIndicator* ind, const char* title) {
    app_indicator_set_title(ind, title);
}

static void pac_set_indicator_icon_path(AppIndicator* ind, const char* dir, const char* name, const char* desc) {
    app_indicator_set_icon_theme_path(ind, dir);
    app_indicator_set_icon_full(ind, name, desc);
}

static void pac_set_indicator_secondary(AppIndicator* ind, GtkWidget* item) {
    app_indicator_set_secondary_activate_target(ind, item);
}

static void pac_set_indicator_menu(AppIndicator* ind, GtkMenu* menu) {
    app_indicator_set_menu(ind, menu);
}
*/
import "C"

import (
	"log"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

var activeTrayIcon *TrayIcon

//export pacTrayActivateCallback
func pacTrayActivateCallback() {
	glib.IdleAdd(func() {
		if activeTrayIcon != nil {
			activeTrayIcon.toggleWindow()
		}
	})
}

// TrayIcon wraps an AppIndicator system tray icon.
type TrayIcon struct {
	indicator *C.AppIndicator
	app       *AppWindow
	iconDir   string // temp dir holding the icon png
}

// setupTrayIcon creates a system tray indicator for the given AppWindow.
// It writes the embedded PNG icon to a temp directory (AppIndicator needs a
// filesystem path), then creates the indicator with a context menu containing
// Show/Hide and Quit actions.
func setupTrayIcon(app *AppWindow) *TrayIcon {
	// Hook fallback GtkStatusIcon and DBus message filter for left click toggle & tooltip
	C.pac_init_indicator_class()
	C.pac_attach_dbus_filter()

	// Write icon to a temp directory so AppIndicator can reference it
	tmpDir, err := os.MkdirTemp("", "pac-tray-*")
	if err != nil {
		log.Printf("[TRAY] Failed to create temp dir for icon: %v", err)
		return nil
	}

	// AppIndicator requires icons to be named in a specific way for themes.
	// We'll write as "pac-tray.png" inside the temp dir.
	iconPath := filepath.Join(tmpDir, "pac-tray.png")
	if writeErr := os.WriteFile(iconPath, appIconBytes, 0600); writeErr != nil {
		log.Printf("[TRAY] Failed to write tray icon: %v", writeErr)
		os.RemoveAll(tmpDir)
		return nil
	}

	t := &TrayIcon{
		app:     app,
		iconDir: tmpDir,
	}

	cDir := C.CString(tmpDir)
	defer C.free(unsafe.Pointer(cDir))

	t.indicator = C.pac_create_indicator(cDir)
	if t.indicator == nil {
		log.Printf("[TRAY] AppIndicator creation failed")
		os.RemoveAll(tmpDir)
		return nil
	}

	// Set title ("PacNG") which displays as tooltip on panel hover
	cTitle := C.CString("PacNG")
	defer C.free(unsafe.Pointer(cTitle))
	C.pac_set_indicator_title(t.indicator, cTitle)

	// Set icon — name without extension, dir is the theme path, desc is tooltip
	cIconDir := C.CString(tmpDir)
	defer C.free(unsafe.Pointer(cIconDir))
	cIconName := C.CString("pac-tray")
	defer C.free(unsafe.Pointer(cIconName))
	cIconDesc := C.CString("PacNG")
	defer C.free(unsafe.Pointer(cIconDesc))
	C.pac_set_indicator_icon_path(t.indicator, cIconDir, cIconName, cIconDesc)

	// Build the right-click / scroll-wheel context menu
	t.buildMenu()

	activeTrayIcon = t
	return t
}

// buildMenu creates the indicator context menu and attaches it.
func (t *TrayIcon) buildMenu() {
	menu, _ := gtk.MenuNew()

	// Show/Hide entry
	showHideItem, _ := gtk.MenuItemNewWithLabel(t.showHideLabel())
	showHideItem.Connect("activate", func() {
		t.toggleWindow()
	})
	menu.Append(showHideItem)

	// Separator
	sep, _ := gtk.SeparatorMenuItemNew()
	menu.Append(sep)

	// Quit entry
	quitItem, _ := gtk.MenuItemNewWithLabel("Выход / Quit")
	quitItem.Connect("activate", func() {
		t.app.Quit()
	})
	menu.Append(quitItem)

	menu.ShowAll()

	// Pass GtkMenu* to AppIndicator via CGo
	menuNative := menu.Native()
	C.pac_set_indicator_menu(t.indicator, (*C.GtkMenu)(unsafe.Pointer(menuNative)))

	// Connect secondary activate target (middle click) to show/hide
	C.pac_set_indicator_secondary(t.indicator, (*C.GtkWidget)(unsafe.Pointer(showHideItem.Native())))

	// Hold reference so GC doesn't collect
	_ = menu
}

func (t *TrayIcon) showHideLabel() string {
	return "Показать / Скрыть"
}

// toggleWindow toggles window visibility:
// - If visible and active: hides into tray
// - If hidden or minimized or inactive: brings to front, deiconifies and focuses
func (t *TrayIcon) toggleWindow() {
	if t.app == nil || t.app.Window == nil {
		return
	}
	win := t.app.Window
	visible := win.IsVisible()
	if visible && win.IsActive() {
		win.Hide()
	} else {
		win.Show()
		win.Deiconify()
		win.Present()
	}
}

// iconPath returns the path to the icon file inside the temp dir.
func (t *TrayIcon) iconPath() string {
	return filepath.Join(t.iconDir, "pac-tray.png")
}

// Destroy cleans up temp dir resources.
func (t *TrayIcon) Destroy() {
	if activeTrayIcon == t {
		activeTrayIcon = nil
	}
	if t.iconDir != "" {
		os.RemoveAll(t.iconDir)
	}
}

// loadTrayPixbuf loads the app icon bytes as a pixbuf (helper for callers that need a *gdk.Pixbuf).
func loadTrayPixbuf() (*gdk.Pixbuf, error) {
	loader, err := gdk.PixbufLoaderNew()
	if err != nil {
		return nil, err
	}
	if _, err := loader.Write(appIconBytes); err != nil {
		return nil, err
	}
	if err := loader.Close(); err != nil {
		return nil, err
	}
	return loader.GetPixbuf()
}
