package ui

/*
#cgo pkg-config: ayatana-appindicator3-0.1
#include <libayatana-appindicator/app-indicator.h>
#include <gtk/gtk.h>
#include <stdlib.h>

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

static void pac_set_indicator_icon_path(AppIndicator* ind, const char* dir, const char* name) {
    app_indicator_set_icon_theme_path(ind, dir);
    app_indicator_set_icon_full(ind, name, "PAC");
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

	// Set icon — name without extension, dir is the theme path
	cIconDir := C.CString(tmpDir)
	defer C.free(unsafe.Pointer(cIconDir))
	cIconName := C.CString("pac-tray")
	defer C.free(unsafe.Pointer(cIconName))
	C.pac_set_indicator_icon_path(t.indicator, cIconDir, cIconName)

	// Build the right-click / scroll-wheel context menu
	t.buildMenu()

	return t
}

// buildMenu creates the indicator context menu and attaches it.
func (t *TrayIcon) buildMenu() {
	menu, _ := gtk.MenuNew()

	// Show/Hide entry
	showHideItem, _ := gtk.MenuItemNewWithLabel(t.showHideLabel())
	showHideItem.Connect("activate", func() {
		glib.IdleAdd(func() {
			t.toggleWindow(showHideItem)
		})
	})
	menu.Append(showHideItem)

	// Separator
	sep, _ := gtk.SeparatorMenuItemNew()
	menu.Append(sep)

	// Quit entry
	quitItem, _ := gtk.MenuItemNewWithLabel("Выход / Quit")
	quitItem.Connect("activate", func() {
		glib.IdleAdd(func() {
			t.app.Quit()
		})
	})
	menu.Append(quitItem)

	menu.ShowAll()

	// Pass GtkMenu* to AppIndicator via CGo
	menuNative := menu.Native()
	C.pac_set_indicator_menu(t.indicator, (*C.GtkMenu)(unsafe.Pointer(menuNative)))

	// Hold reference so GC doesn't collect
	_ = menu
}

func (t *TrayIcon) showHideLabel() string {
	return "Показать / Скрыть"
}

// toggleWindow toggles window visibility and rebuilds the Show/Hide item label.
func (t *TrayIcon) toggleWindow(item *gtk.MenuItem) {
	win := t.app.Window
	// GetVisible reports whether the widget is visible (not iconified check needed separately).
	// We rely on a simple boolean we can check from mapping state.
	visible := win.IsVisible()
	if visible {
		win.Hide()
	} else {
		win.Show()
		win.Present()
	}
}

// iconPath returns the path to the icon file inside the temp dir.
func (t *TrayIcon) iconPath() string {
	return filepath.Join(t.iconDir, "pac-tray.png")
}

// Destroy cleans up temp dir resources.
func (t *TrayIcon) Destroy() {
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
