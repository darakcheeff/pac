package ui

import (
	"context"
	_ "embed"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/darakcheeff/pac/internal/engine/pty"
	"github.com/darakcheeff/pac/internal/engine/rdp"
	"github.com/darakcheeff/pac/internal/engine/sftp"
	engineSSH "github.com/darakcheeff/pac/internal/engine/ssh"
	"github.com/darakcheeff/pac/internal/engine/vnc"
	"github.com/darakcheeff/pac/internal/engine/watcher"
	"github.com/darakcheeff/pac/internal/i18n"
	"github.com/darakcheeff/pac/internal/migration"
	"github.com/darakcheeff/pac/internal/session"
	"github.com/darakcheeff/pac/internal/storage"
	"github.com/darakcheeff/pac/internal/ui/dialogs"
	"github.com/darakcheeff/pac/internal/ui/vte"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
	cryptoSsh "golang.org/x/crypto/ssh"
)

//go:embed assets/icon.png
var appIconBytes []byte

// AppWindow represents the primary application window
type AppWindow struct {
	Window       *gtk.Window
	MainBox      *gtk.Box
	ToolBar      *gtk.Toolbar
	MainPaned    *gtk.Paned
	LeftPaned    *gtk.Paned
	CenterPaned  *gtk.Paned
	HostTree     *HostTree
	SFTPPanel    *SFTPPanel
	TabView      *TabView
	NotesPanel   *NotesPanel
	BroadcastBar *BroadcastBar
	StatusBar    *gtk.Statusbar
	StatusLabel  *gtk.Label

	store       *storage.Store
	manager     *session.Manager
	watcherMgr  *watcher.RemoteEditManager
	settings    *storage.AppSettings
	isRestoring   bool
	restoreMu     sync.Mutex
	lastSavedHash string
	lastSavedMu   sync.Mutex
	currentTheme  string
	btnTheme      *gtk.ToolButton
	btnExit       *gtk.ToolButton
	themeProvider *gtk.CssProvider
	trayIcon      *TrayIcon
	activeBaudBars map[string]*gtk.InfoBar
}


const baseCompactCSS = `
toolbar {
	padding: 1px 2px;
	min-height: 28px;
}
toolbar button {
	padding: 2px 4px;
	margin: 0 1px;
	min-height: 24px;
	min-width: 24px;
}
notebook tab {
	padding: 2px 6px;
	min-height: 24px;
	font-size: 11px;
}
notebook tab button {
	padding: 0;
	min-height: 16px;
	min-width: 16px;
	border-radius: 3px;
}
treeview {
	font-size: 11px;
}
treeview.view {
	min-height: 20px;
}
statusbar {
	padding: 0 4px;
	min-height: 20px;
	font-size: 11px;
}
menubar {
	padding: 0;
	min-height: 22px;
}
menubar > menuitem {
	padding: 2px 6px;
	font-size: 11px;
}
paned > separator {
	min-width: 4px;
	min-height: 4px;
}
entry {
	min-height: 24px;
	padding: 2px 4px;
	font-size: 11px;
}
button {
	min-height: 24px;
	padding: 2px 6px;
}
`

const darkCSS = baseCompactCSS + `
window, dialog, .background {
	background-color: #242424;
	color: #dddddd;
}
headerbar, toolbar {
	background-color: #2d2d2d;
	color: #dddddd;
	border-bottom: 1px solid #1a1a1a;
}
toolbar button {
	color: #dddddd;
	background-color: transparent;
	border: 1px solid transparent;
}
toolbar button:hover {
	background-color: #3d3d3d;
	border: 1px solid #505050;
}
menubar {
	background-color: #2d2d2d;
	color: #dddddd;
	border-bottom: 1px solid #1a1a1a;
}
menubar > menuitem {
	color: #dddddd;
}
menubar > menuitem:hover {
	background-color: #3d3d3d;
}
menu, .menu {
	background-color: #2d2d2d;
	color: #dddddd;
	border: 1px solid #3d3d3d;
}
menuitem {
	color: #dddddd;
}
menuitem:hover {
	background-color: #3584e4;
	color: #ffffff;
}
scrolledwindow, viewport {
	background-color: #242424;
}
treeview, treeview.view {
	background-color: #242424;
	color: #dddddd;
	border: none;
}
treeview:selected, treeview.view:selected {
	background-color: #1a5fb4;
	color: #ffffff;
}
treeview header button, treeview header button:hover {
	background-color: #2d2d2d;
	color: #aaaaaa;
	border-bottom: 1px solid #1a1a1a;
	border-right: 1px solid #1a1a1a;
}
notebook {
	background-color: #242424;
}
notebook > header, notebook header {
	background-color: #282828;
	border-bottom: 1px solid #1a1a1a;
}
notebook > header > tabs > tab, notebook tab {
	background-color: transparent;
	color: #aaaaaa;
	border: none;
}
notebook > header > tabs > tab:checked, notebook tab:checked, notebook tab:active {
	background-color: #383838;
	color: #ffffff;
	border: none;
}
notebook tab button {
	color: #aaaaaa;
	background-color: transparent;
	border: none;
}
notebook tab button:hover {
	color: #ffffff;
	background-color: rgba(255, 255, 255, 0.2);
}
textview, textview text {
	background-color: #242424;
	color: #eeeeee;
}
entry {
	background-color: #1a1a1a;
	color: #ffffff;
	border: 1px solid #3d3d3d;
}
entry:focus {
	border-color: #3584e4;
}
statusbar {
	background-color: #282828;
	color: #aaaaaa;
	border-top: 1px solid #1a1a1a;
}
paned > separator {
	background-color: #555555;
	border: 1px solid #383838;
}
paned > separator:hover {
	background-color: #3584e4;
	border-color: #1a5fb4;
}
button {
	background-color: #333333;
	color: #dddddd;
	border: 1px solid #444444;
}
button:hover {
	background-color: #404040;
	border-color: #555555;
}
button:active {
	background-color: #202020;
}
scrollbar slider {
	background-color: #404040;
}
scrollbar slider:hover {
	background-color: #555555;
}
tooltip, tooltip.background {
	background-color: #222222;
	color: #ffffff;
	border: 1px solid #505050;
	border-radius: 4px;
	padding: 4px 8px;
}
tooltip label, tooltip * {
	color: #ffffff;
}
`

const lightCSS = baseCompactCSS + `
window, dialog, .background {
	background-color: #f6f6f6;
	color: #2e3436;
}
headerbar, toolbar {
	background-color: #ebebeb;
	color: #2e3436;
	border-bottom: 1px solid #d0d0d0;
}
toolbar button {
	color: #2e3436;
	background-color: transparent;
	border: 1px solid transparent;
}
toolbar button:hover {
	background-color: #dedede;
	border: 1px solid #cccccc;
}
menubar {
	background-color: #f0f0f0;
	color: #2e3436;
	border-bottom: 1px solid #d0d0d0;
}
menubar > menuitem {
	color: #2e3436;
}
menubar > menuitem:hover {
	background-color: #e0e0e0;
}
menu, .menu {
	background-color: #ffffff;
	color: #2e3436;
	border: 1px solid #cccccc;
}
menuitem {
	color: #2e3436;
}
menuitem:hover {
	background-color: #3584e4;
	color: #ffffff;
}
scrolledwindow, viewport {
	background-color: #ffffff;
}
treeview, treeview.view {
	background-color: #ffffff;
	color: #2e3436;
	border: none;
}
treeview:selected, treeview.view:selected {
	background-color: #3584e4;
	color: #ffffff;
}
treeview header button, treeview header button:hover {
	background-color: #ececec;
	color: #555555;
	border-bottom: 1px solid #d0d0d0;
	border-right: 1px solid #d0d0d0;
}
notebook {
	background-color: #f6f6f6;
}
notebook > header, notebook header {
	background-color: #e8e8e8;
	border-bottom: 1px solid #cccccc;
}
notebook > header > tabs > tab, notebook tab {
	background-color: transparent;
	color: #555555;
	border: none;
}
notebook > header > tabs > tab:checked, notebook tab:checked, notebook tab:active {
	background-color: #ffffff;
	color: #2e3436;
	border: none;
}
notebook tab button {
	color: #666666;
	background-color: transparent;
	border: none;
}
notebook tab button:hover {
	color: #000000;
	background-color: rgba(0, 0, 0, 0.1);
}
textview, textview text {
	background-color: #ffffff;
	color: #2e3436;
}
entry {
	background-color: #ffffff;
	color: #2e3436;
	border: 1px solid #cccccc;
}
entry:focus {
	border-color: #3584e4;
}
statusbar {
	background-color: #ececec;
	color: #555555;
	border-top: 1px solid #cccccc;
}
paned > separator {
	background-color: #b0b0b0;
	border: 1px solid #999999;
}
paned > separator:hover {
	background-color: #3584e4;
	border-color: #1a5fb4;
}
button {
	background-color: #f0f0f0;
	color: #2e3436;
	border: 1px solid #cccccc;
}
button:hover {
	background-color: #e4e4e4;
	border-color: #bbbbbb;
}
button:active {
	background-color: #d8d8d8;
}
scrollbar slider {
	background-color: #cccccc;
}
scrollbar slider:hover {
	background-color: #aaaaaa;
}
tooltip, tooltip.background {
	background-color: #ffffff;
	color: #000000;
	border: 1px solid #707070;
	border-radius: 4px;
	padding: 4px 8px;
}
tooltip label, tooltip * {
	color: #000000;
	font-weight: normal;
}
`

// applyTheme configures GTK dark/light theme and custom styling
func (app *AppWindow) applyTheme(theme string) {
	if theme == "" {
		theme = "dark"
	}
	app.currentTheme = theme
	isDark := theme == "dark"

	settings, err := gtk.SettingsGetDefault()
	if err == nil && settings != nil {
		_ = settings.SetProperty("gtk-application-prefer-dark-theme", isDark)
		if isDark {
			_ = settings.SetProperty("gtk-theme-name", "Adwaita-dark")
		} else {
			_ = settings.SetProperty("gtk-theme-name", "Adwaita")
		}
	}

	cssData := darkCSS
	if !isDark {
		cssData = lightCSS
	}

	if app.themeProvider == nil {
		provider, err := gtk.CssProviderNew()
		if err == nil {
			app.themeProvider = provider
			screen, err := gdk.ScreenGetDefault()
			if err == nil && screen != nil {
				gtk.AddProviderForScreen(screen, app.themeProvider, gtk.STYLE_PROVIDER_PRIORITY_USER)
			}
		}
	}

	if app.themeProvider != nil {
		_ = app.themeProvider.LoadFromData(cssData)
	}

	if app.TabView != nil {
		app.TabView.SetTheme(isDark)
	}
	if app.SFTPPanel != nil {
		app.SFTPPanel.UpdateTheme(isDark)
	}

	app.updateThemeButton()
	app.updateExitButton()
}

// updateExitButton updates the door icon for the exit button according to the theme
func (app *AppWindow) updateExitButton() {
	if app.btnExit == nil {
		return
	}
	app.btnExit.SetIconWidget(GetDoorExitImage(app.currentTheme == "dark"))
	app.btnExit.ShowAll()
}

// updateThemeButton updates icon and tooltip for theme toggle button
func (app *AppWindow) updateThemeButton() {
	if app.btnTheme == nil {
		return
	}
	if app.currentTheme == "dark" {
		app.btnTheme.SetIconWidget(GetSunImage())
		app.btnTheme.SetTooltipText(i18n.T("Переключить на светлую тему", "Switch to Light theme"))
		app.btnTheme.SetLabel(i18n.T("Светлая тема", "Light Theme"))
	} else {
		app.btnTheme.SetIconWidget(GetMoonImage())
		app.btnTheme.SetTooltipText(i18n.T("Переключить на тёмную тему", "Switch to Dark theme"))
		app.btnTheme.SetLabel(i18n.T("Тёмная тема", "Dark Theme"))
	}
	app.btnTheme.ShowAll()
}

// ToggleTheme switches between dark and light themes and persists user preference
func (app *AppWindow) ToggleTheme() {
	newTheme := "light"
	if app.currentTheme == "light" {
		newTheme = "dark"
	}
	log.Printf("[APP] ToggleTheme: switching to %s", newTheme)
	app.applyTheme(newTheme)
	if app.settings != nil {
		app.settings.Theme = newTheme
		if app.store != nil {
			_ = app.store.SaveSetting("theme", newTheme)
		}
	}
}

func NewAppWindow(store *storage.Store) (*AppWindow, error) {
	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		return nil, err
	}
	win.SetTitle("PAC Connection Manager NextGen")
	win.SetDefaultSize(1200, 750)

	if len(appIconBytes) > 0 {
		loader, err := gdk.PixbufLoaderNew()
		if err == nil {
			_, _ = loader.Write(appIconBytes)
			_ = loader.Close()
			if pixbuf, err := loader.GetPixbuf(); err == nil && pixbuf != nil {
				win.SetIcon(pixbuf)
				gtk.WindowSetDefaultIcon(pixbuf)
			}
		}
	}

	// Connect OSC 52 terminal clipboard sequences to system clipboard
	session.GlobalClipboardHandler = func(target, text string) {
		glib.IdleAdd(func() {
			clip, err := gtk.ClipboardGet(gdk.SELECTION_CLIPBOARD)
			if err == nil && clip != nil {
				clip.SetText(text)
			}
			primary, err := gtk.ClipboardGet(gdk.SELECTION_PRIMARY)
			if err == nil && primary != nil {
				primary.SetText(text)
			}
		})
	}

	win.SetPosition(gtk.WIN_POS_CENTER)

	watcherMgr, _ := watcher.NewRemoteEditManager()
	manager := session.NewManager(store)
	settings, _ := store.GetSettings()

	mainBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	win.Add(mainBox)

	// 1. ToolBar (Clean, compact single top bar)
	toolBar, _ := gtk.ToolbarNew()
	toolBar.SetStyle(gtk.TOOLBAR_ICONS)
	toolBar.SetIconSize(gtk.ICON_SIZE_SMALL_TOOLBAR)
	mainBox.PackStart(toolBar, false, false, 0)

	// 3. Center Workspace Paneds with clean independent docking
	// mainPaned: [ centerPaned (Left Sidebar + Terminals) | notesPanel (Right Sidebar) ]
	// centerPaned: [ leftPaned (Hosts + SFTP) | tabView.Notebook (Center Terminals) ]
	// leftPaned: [ hostTree (Top) | sftpPanel (Bottom) ]
	mainPaned, _ := gtk.PanedNew(gtk.ORIENTATION_HORIZONTAL)
	mainPaned.SetWideHandle(true)

	centerPaned, _ := gtk.PanedNew(gtk.ORIENTATION_HORIZONTAL)
	centerPaned.SetWideHandle(true)

	leftPaned, _ := gtk.PanedNew(gtk.ORIENTATION_VERTICAL)
	leftPaned.SetWideHandle(true)

	hostTree, _ := NewHostTree(store)
	sftpPanel, _ := NewSFTPPanel(watcherMgr)
	tabView, _ := NewTabView()
	notesPanel, _ := NewNotesPanel(store)
	broadcastBar, _ := NewBroadcastBar(manager)

	// Left vertical split: Hosts (top) + SFTP (bottom)
	leftPaned.Pack1(hostTree.Box, true, false)
	leftPaned.Pack2(sftpPanel.Box, true, false)
	leftPaned.SetPosition(260)

	// Center horizontal split: Left Sidebar + Center Terminals
	// leftPaned doesn't expand on window resize; tabView takes all extra width
	// shrink=false guarantees left sidebar is never pushed out of window boundaries
	centerPaned.Pack1(leftPaned, false, false)
	centerPaned.Pack2(tabView.Notebook, true, false)
	centerPaned.SetPosition(280)

	// Main horizontal split: (Left Sidebar + Center Terminals) + Notes Panel (Right)
	// centerPaned takes all window resize; notesPanel stays docked on right
	mainPaned.Pack1(centerPaned, true, false)
	mainPaned.Pack2(notesPanel.Box, false, false)
	mainPaned.SetPosition(750)

	// Notes panel hidden by default to maximize terminal space
	notesPanel.Box.SetNoShowAll(true)
	notesPanel.Box.Hide()

	mainBox.PackStart(mainPaned, true, true, 0)

	// 4. Bottom Broadcast Bar (Hidden by default, won't show with ShowAll)
	broadcastBar.Box.SetNoShowAll(true)
	mainBox.PackStart(broadcastBar.Box, false, false, 0)
	broadcastBar.Box.Hide()

	// 5. StatusBar
	statusBar, _ := gtk.StatusbarNew()
	statusLabel, _ := gtk.LabelNew(i18n.T("Готово к работе", "Ready"))
	statusLabel.SetMarginStart(8)
	statusBar.PackStart(statusLabel, false, false, 0)
	mainBox.PackEnd(statusBar, false, false, 0)

	app := &AppWindow{
		Window:       win,
		MainBox:      mainBox,
		ToolBar:      toolBar,
		MainPaned:    mainPaned,
		LeftPaned:    leftPaned,
		CenterPaned:  centerPaned,
		HostTree:     hostTree,
		SFTPPanel:    sftpPanel,
		TabView:      tabView,
		NotesPanel:   notesPanel,
		BroadcastBar: broadcastBar,
		StatusBar:    statusBar,
		StatusLabel:  statusLabel,
		store:        store,
		manager:      manager,
		watcherMgr:   watcherMgr,
		settings:     settings,
		isRestoring:  true, // Prevent overwriting state before restore completes
		activeBaudBars: make(map[string]*gtk.InfoBar),
	}

	app.setupMenuAndToolbar()
	app.setupSignals()

	// Apply initial theme from settings (default to dark)
	initTheme := "dark"
	if settings != nil && settings.Theme == "light" {
		initTheme = "light"
	}
	app.applyTheme(initTheme)

	// Periodic auto-save of active session state (every 30 seconds with diff checking)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			app.restoreMu.Lock()
			restoring := app.isRestoring
			app.restoreMu.Unlock()

			if !restoring && app.settings.AutoRestoreSessions {
				glib.IdleAdd(func() {
					app.SaveAllSessionState()
				})
			}
		}
	}()

	// Background initialization: Restore saved sessions
	go func() {
		glib.IdleAdd(func() {
			if app.settings.AutoRestoreSessions {
				app.RestoreSavedSessions()
			} else {
				app.restoreMu.Lock()
				app.isRestoring = false
				app.restoreMu.Unlock()
			}
		})
	}()

	// Initialize system tray icon
	app.trayIcon = setupTrayIcon(app)

	return app, nil
}

func (app *AppWindow) setupMenuAndToolbar() {
	// --- ToolBar Buttons with standard icons & rich tooltips ---
	// 1. New Connection
	btnNew, _ := gtk.ToolButtonNew(nil, i18n.T("Новое подключение", "New Connection"))
	btnNew.SetIconName("tab-new-symbolic")
	btnNew.SetTooltipText(i18n.T("Создать новое подключение к серверу (SSH, Telnet, Serial, Local)", "Create new connection (SSH, Telnet, Serial, Local)"))
	btnNew.Connect("clicked", func() {
		dialogs.ShowHostEditorDialog(app.Window, app.store, nil, "root", func(h *storage.Host) {
			app.HostTree.Reload()
		})
	})
	app.ToolBar.Insert(btnNew, -1)

	// 2. Quick Connect
	btnQuick, _ := gtk.ToolButtonNew(nil, i18n.T("Быстрое подключение", "Quick Connect"))
	btnQuick.SetIconName("network-wired-symbolic")
	btnQuick.SetTooltipText(i18n.T("Быстрое подключение к хосту без предварительного сохранения", "Quick connect without saving host"))
	btnQuick.Connect("clicked", func() {
		dialogs.ShowQuickConnectDialog(app.Window, func(h *storage.Host) {
			app.ConnectToHost(h)
		})
	})
	app.ToolBar.Insert(btnQuick, -1)

	sepTool1, _ := gtk.SeparatorToolItemNew()
	app.ToolBar.Insert(sepTool1, -1)

	// 3. Split Horizontal (Top / Bottom)
	btnSplitH, _ := gtk.ToolButtonNew(GetSplitHorizontalImage(), i18n.T("Разделить горизонтально", "Split Horizontally"))
	btnSplitH.SetTooltipText(i18n.T("Разделить экран по горизонтали (сверху и снизу)", "Split screen horizontally (top / bottom)"))
	btnSplitH.Connect("clicked", func() {
		tab := app.TabView.GetCurrentTab()
		if tab != nil {
			sess := tab.Session
			if tab.FocusedPane != nil {
				sess = tab.FocusedPane.Session
			}
			app.handleSplit(sess, false)
		}
	})
	app.ToolBar.Insert(btnSplitH, -1)

	// 4. Split Vertical (Left / Right)
	btnSplitV, _ := gtk.ToolButtonNew(GetSplitVerticalImage(), i18n.T("Разделить вертикально", "Split Vertically"))
	btnSplitV.SetTooltipText(i18n.T("Разделить экран по вертикали (слева и справа)", "Split screen vertically (left / right)"))
	btnSplitV.Connect("clicked", func() {
		tab := app.TabView.GetCurrentTab()
		if tab != nil {
			sess := tab.Session
			if tab.FocusedPane != nil {
				sess = tab.FocusedPane.Session
			}
			app.handleSplit(sess, true)
		}
	})
	app.ToolBar.Insert(btnSplitV, -1)

	// 5. Unsplit (Restore / Detach split pane)
	btnUnsplit, _ := gtk.ToolButtonNew(GetUnsplitImage(), i18n.T("Разгруппировать", "Unsplit"))
	btnUnsplit.SetTooltipText(i18n.T("Разгруппировать сплит в отдельную вкладку", "Unsplit pane into separate tab"))
	btnUnsplit.Connect("clicked", func() {
		tab := app.TabView.GetCurrentTab()
		if tab == nil {
			return
		}
		if len(tab.Panes) <= 1 {
			app.StatusLabel.SetText(i18n.T("Текущая вкладка не содержит разделенных панелей", "Current tab has no split panes"))
			return
		}
		app.TabView.UnsplitTab(tab)
		app.StatusLabel.SetText(i18n.T("Активная панель вынесена в отдельную вкладку", "Active pane detached into separate tab"))
	})
	app.ToolBar.Insert(btnUnsplit, -1)

	sepTool2, _ := gtk.SeparatorToolItemNew()
	app.ToolBar.Insert(sepTool2, -1)

	// 5. Broadcast / Cluster Input
	btnBroadcast, _ := gtk.ToolButtonNew(nil, i18n.T("Кластерный ввод", "Cluster Input"))
	btnBroadcast.SetIconName("input-keyboard-symbolic")
	btnBroadcast.SetTooltipText(i18n.T("Кластерный ввод: одновременная трансляция команд во все открытые вкладки", "Cluster input: broadcast commands to all open tabs"))
	btnBroadcast.Connect("clicked", func() {
		if app.BroadcastBar.Box.IsVisible() {
			app.BroadcastBar.Box.Hide()
		} else {
			app.BroadcastBar.Box.Show()
			app.BroadcastBar.Entry.GrabFocus()
		}
	})
	app.ToolBar.Insert(btnBroadcast, -1)

	// 6. Global Search
	btnSearch, _ := gtk.ToolButtonNew(nil, i18n.T("Поиск по всем сессиям", "Global Search"))
	btnSearch.SetIconName("edit-find-symbolic")
	btnSearch.SetTooltipText(i18n.T("Глобальный поиск текста по всем открытым сессиям и вкладкам", "Global text search across all open sessions and tabs"))
	btnSearch.Connect("clicked", func() {
		ShowGlobalSearchDialog(app.Window, app.manager, func(sessionID string) {
			app.TabView.SelectSession(sessionID)
		})
	})
	app.ToolBar.Insert(btnSearch, -1)

	// 7. Toggle Notes Panel
	btnNotes, _ := gtk.ToolButtonNew(nil, i18n.T("Заметки", "Notes"))
	btnNotes.SetIconName("x-office-document-symbolic")
	btnNotes.SetTooltipText(i18n.T("Показать / скрыть панель заметок", "Show / hide notes panel"))
	btnNotes.Connect("clicked", func() {
		app.ToggleNotesPanel()
	})
	app.ToolBar.Insert(btnNotes, -1)

	// 8. Import from Ásbrú / PAC
	btnImport, _ := gtk.ToolButtonNew(nil, i18n.T("Импорт из Ásbrú / PAC", "Import from Ásbrú / PAC"))
	btnImport.SetIconName("document-open-symbolic")
	btnImport.SetTooltipText(i18n.T("Импортировать подключения из Ásbrú / PAC (asbru.conf, pac.yml)", "Import connections from Ásbrú / PAC (asbru.conf, pac.yml)"))
	btnImport.Connect("clicked", func() {
		if app.HostTree.OnImportOld != nil {
			app.HostTree.OnImportOld()
		}
	})
	app.ToolBar.Insert(btnImport, -1)

	// Separator
	sepTool3, _ := gtk.SeparatorToolItemNew()
	app.ToolBar.Insert(sepTool3, -1)

	// 9. Theme Toggle (Sun / Moon)
	btnTheme, _ := gtk.ToolButtonNew(nil, i18n.T("Тема", "Theme"))
	app.btnTheme = btnTheme
	btnTheme.Connect("clicked", func() {
		app.ToggleTheme()
	})
	app.ToolBar.Insert(btnTheme, -1)
	app.updateThemeButton()

	// 10. Exit Application (Door symbol: quits application completely instead of minimizing to tray)
	btnExit, _ := gtk.ToolButtonNew(GetDoorExitImage(app.currentTheme == "dark"), i18n.T("Выход", "Exit"))
	app.btnExit = btnExit
	btnExit.SetTooltipText(i18n.T("Выход из программы", "Exit Application"))
	btnExit.Connect("clicked", func() {
		app.Quit()
	})
	app.ToolBar.Insert(btnExit, -1)
}


// ToggleNotesPanel toggles visibility of the right-hand notes panel without losing state
func (app *AppWindow) ToggleNotesPanel() {
	if app.NotesPanel.Box.GetVisible() {
		app.NotesPanel.Box.SetVisible(false)
	} else {
		app.NotesPanel.Box.SetVisible(true)
		w := app.MainPaned.GetAllocatedWidth()
		if w > 400 {
			app.MainPaned.SetPosition(w - 250)
		}
		curTab := app.TabView.GetCurrentTab()
		if curTab != nil {
			sess := curTab.Session
			if curTab.FocusedPane != nil && curTab.FocusedPane.Session != nil {
				sess = curTab.FocusedPane.Session
			}
			app.NotesPanel.LoadSessionNotes(sess)
		}
	}
}

func (app *AppWindow) setupSignals() {
	app.HostTree.OnConnectHost = func(host *storage.Host) {
		app.ConnectToHost(host)
	}

	app.HostTree.OnEditHost = func(host *storage.Host) {
		dialogs.ShowHostEditorDialog(app.Window, app.store, host, host.GroupID, func(h *storage.Host) {
			app.HostTree.Reload()
		})
	}

	app.HostTree.OnAddHost = func(parentGroupID string) {
		dialogs.ShowHostEditorDialog(app.Window, app.store, nil, parentGroupID, func(h *storage.Host) {
			app.HostTree.Reload()
		})
	}

	app.HostTree.OnAddGroup = func(parentGroupID string) {
		name, ok := promptFolderDialog(app.Window, i18n.T("Новая папка", "New Folder"), i18n.T("Новая папка", "New Folder"))
		if !ok || name == "" {
			return
		}
		g := &storage.Group{
			ID:       fmt.Sprintf("grp-%d", time.Now().UnixNano()),
			ParentID: parentGroupID,
			Name:     name,
			Icon:     "folder",
		}
		if err := app.store.SaveGroup(g); err != nil {
			app.ShowError(i18n.T("Ошибка создания папки", "Folder Creation Error"),
				i18n.Tf("Не удалось сохранить папку \"%s\":\n\n%s", "Failed to save folder \"%s\":\n\n%s", name, err.Error()))
			return
		}
		app.HostTree.Reload()
	}

	app.HostTree.OnRenameGroup = func(groupID string) {
		g, err := app.store.GetGroup(groupID)
		if err != nil || g == nil {
			return
		}
		name, ok := promptFolderDialog(app.Window, i18n.T("Переименовать папку", "Rename Folder"), g.Name)
		if !ok || name == "" || name == g.Name {
			return
		}
		g.Name = name
		if err := app.store.SaveGroup(g); err != nil {
			app.ShowError(i18n.T("Ошибка переименования папки", "Folder Rename Error"),
				i18n.Tf("Не удалось переименовать папку \"%s\":\n\n%s", "Failed to rename folder \"%s\":\n\n%s", name, err.Error()))
			return
		}
		app.HostTree.Reload()
	}

	app.HostTree.OnRenameHost = func(hostID string) {
		h, err := app.store.GetHost(hostID)
		if err != nil || h == nil {
			return
		}
		name, ok := promptInputDialog(app.Window, i18n.T("Переименовать подключение", "Rename Connection"), i18n.T("Имя подключения:", "Connection name:"), h.Name)
		if !ok || name == "" || name == h.Name {
			return
		}
		h.Name = name
		if err := app.store.SaveHost(h); err != nil {
			app.ShowError(i18n.T("Ошибка переименования подключения", "Connection Rename Error"),
				i18n.Tf("Не удалось переименовать подключение \"%s\":\n\n%s", "Failed to rename connection \"%s\":\n\n%s", name, err.Error()))
			return
		}
		app.HostTree.Reload()
	}

	app.HostTree.OnImportOld = func() {
		configPath := migration.FindLegacyConfigPath()
		if configPath == "" {
			fc, err := gtk.FileChooserDialogNewWith2Buttons(
				i18n.T("Выберите файл конфигурации Ásbrú / PAC", "Select Ásbrú / PAC configuration file"),
				app.Window,
				gtk.FILE_CHOOSER_ACTION_OPEN,
				i18n.T("Отмена", "Cancel"), gtk.RESPONSE_CANCEL,
				i18n.T("Открыть", "Open"), gtk.RESPONSE_ACCEPT,
			)
			if err == nil {
				filter, _ := gtk.FileFilterNew()
				filter.SetName(i18n.T("Конфигурации Ásbrú / PAC (*.conf, *.yml)", "Ásbrú / PAC Configs (*.conf, *.yml)"))
				filter.AddPattern("*.conf")
				filter.AddPattern("*.yml")
				filter.AddPattern("*.yaml")
				filter.AddPattern("pac.nfreeze")
				fc.AddFilter(filter)

				allFilter, _ := gtk.FileFilterNew()
				allFilter.SetName(i18n.T("Все файлы (*.*)", "All files (*.*)"))
				allFilter.AddPattern("*")
				fc.AddFilter(allFilter)

				if fc.Run() == gtk.RESPONSE_ACCEPT {
					configPath = fc.GetFilename()
				}
				fc.Destroy()
			}
		}

		if configPath == "" {
			return
		}

		hosts, _, err := migration.ParseLegacyConfigFile(configPath)
		if err != nil {
			app.StatusLabel.SetText(i18n.T("Ошибка чтения конфигурации: ", "Configuration read error: ") + err.Error())
			app.ShowError(i18n.T("Ошибка чтения конфигурации", "Configuration Read Error"),
				i18n.Tf("Не удалось прочитать конфигурацию из \"%s\":\n\n%s", "Failed to read configuration from \"%s\":\n\n%s", configPath, err.Error()))
			return
		}

		if len(hosts) == 0 {
			app.StatusLabel.SetText(i18n.T("В конфигурационном файле не найдено подключений", "No connections found in configuration file"))
			app.ShowError(i18n.T("Импорт конфигурации", "Configuration Import"),
				i18n.T("В указанном файле не найдено подключений для импорта.", "No connections found to import in specified file."))
			return
		}

		dialogs.ShowImportHostsDialog(app.Window, hosts, func(selected []*storage.Host) {
			if len(selected) == 0 {
				return
			}

			// Ensure "asbru" group exists
			asbruGroupID := "group-asbru"
			allGroups, _ := app.store.GetAllGroups()
			asbruExists := false
			for _, g := range allGroups {
				if g.ID == asbruGroupID || strings.EqualFold(g.Name, "asbru") {
					asbruGroupID = g.ID
					asbruExists = true
					break
				}
			}

			if !asbruExists {
				newGroup := &storage.Group{
					ID:        asbruGroupID,
					ParentID:  "root",
					Name:      "asbru",
					Icon:      "folder-remote",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				_ = app.store.SaveGroup(newGroup)
			}

			importedCount := 0
			for _, h := range selected {
				h.GroupID = asbruGroupID
				if err := app.store.SaveHost(h); err == nil {
					importedCount++
				}
			}

			app.HostTree.Reload()
			app.StatusLabel.SetText(i18n.Tf("Успешно импортировано %d хостов в папку asbru", "Successfully imported %d hosts into asbru folder", importedCount))
		})
	}

	app.TabView.OnTabChanged = func(sess *session.Session) {
		if sess != nil {
			app.NotesPanel.LoadSessionNotes(sess)
			if sess.SFTPClient != nil {
				hostID := ""
				if sess.Host != nil {
					hostID = sess.Host.ID
				}
				app.SFTPPanel.AttachClient(sess.ID, hostID, sess.SFTPClient, app.settings.DefaultEditor)
			} else {
				app.SFTPPanel.AttachClient("", "", nil, app.settings.DefaultEditor)
			}
			if sess.Host != nil {
				app.StatusLabel.SetText(i18n.Tf("Сессия: %s (%s) | Протокол: %s", "Session: %s (%s) | Protocol: %s", sess.Title, sess.Host.Host, sess.Host.Protocol))
			}
		}
	}

	app.TabView.OnTabClosed = func(sess *session.Session) {
		if sess != nil {
			log.Printf("[APP] Tab closed: %s (ID=%s)", sess.Title, sess.ID)
			if app.activeBaudBars != nil {
				if bar, ok := app.activeBaudBars[sess.ID]; ok && bar != nil {
					bar.Destroy()
					delete(app.activeBaudBars, sess.ID)
				}
			}
			app.manager.Unregister(sess.ID)
			app.SFTPPanel.RemoveSession(sess.ID)
			if app.settings.AutoRestoreSessions {
				app.SaveAllSessionState()
			}
		}
	}

	app.TabView.OnSplitRequested = func(sess *session.Session, vertical bool) {
		app.handleSplit(sess, vertical)
	}

	app.TabView.OnDuplicateRequested = func(sess *session.Session) {
		if sess != nil && sess.Host != nil {
			app.ConnectToHost(sess.Host)
		}
	}

	app.TabView.OnReconnectRequested = func(sess *session.Session) {
		if sess != nil && sess.Host != nil {
			tab := app.TabView.FindTabBySession(sess)
			if tab != nil {
				app.TabView.CloseTab(tab)
			}
			app.ConnectToHost(sess.Host)
		}
	}

	app.TabView.OnToggleNotesRequested = func() {
		app.ToggleNotesPanel()
	}

	app.TabView.OnTabReordered = func() {
		if app.settings.AutoRestoreSessions {
			app.SaveAllSessionState()
		}
	}

	app.TabView.OnFindRequested = func(item *TabItem) {
		if item != nil && item.FocusedPane != nil && item.FocusedPane.Search != nil {
			item.FocusedPane.Search.Show()
		}
	}

	app.TabView.OnEditHostRequested = func(host *storage.Host) {
		if host != nil {
			dialogs.ShowHostEditorDialog(app.Window, app.store, host, host.GroupID, func(h *storage.Host) {
				app.HostTree.Reload()
			})
		}
	}

	app.TabView.OnSaveLogRequested = func(sess *session.Session) {
		if sess == nil {
			return
		}
		dlg, err := gtk.FileChooserDialogNewWith2Buttons(
			i18n.T("Сохранить журнал сессии", "Save Session Log"),
			app.Window,
			gtk.FILE_CHOOSER_ACTION_SAVE,
			i18n.T("Отмена", "Cancel"), gtk.RESPONSE_CANCEL,
			i18n.T("Сохранить", "Save"), gtk.RESPONSE_ACCEPT,
		)
		if err != nil {
			return
		}
		defer dlg.Destroy()
		dlg.SetCurrentName(fmt.Sprintf("%s_%s.log", sess.Title, time.Now().Format("20060102_150405")))
		if dlg.Run() == gtk.RESPONSE_ACCEPT {
			filename := dlg.GetFilename()
			textToSave := ""
			if tab := app.TabView.FindTabBySession(sess); tab != nil {
				for _, p := range tab.Panes {
					if p.Session == sess && p.Terminal != nil {
						textToSave = p.Terminal.GetCleanText()
						break
					}
				}
			}
			if textToSave == "" {
				textToSave = session.CleanScrollbackDump(sess.GetScrollbackText())
			}
			_ = os.WriteFile(filename, []byte(textToSave), 0644)
			app.StatusLabel.SetText(i18n.T("Журнал сохранен: ", "Log saved: ") + filename)
		}
	}

	app.TabView.OnNewConnection = func() {
		dialogs.ShowQuickConnectDialog(app.Window, func(h *storage.Host) {
			app.ConnectToHost(h)
		})
	}

	app.TabView.OnNewLocalTerminal = func() {
		app.OpenLocalTerminal()
	}

	app.TabView.OnClusterAdmin = func() {
		if app.BroadcastBar.Box.IsVisible() {
			app.BroadcastBar.Box.Hide()
		} else {
			app.BroadcastBar.Box.Show()
			app.BroadcastBar.Entry.GrabFocus()
		}
	}



	app.Window.Connect("delete-event", func() bool {
		if app.trayIcon != nil {
			// Minimize to tray: hide the window but keep the process alive
			app.Window.Hide()
			return true // prevent default destroy
		}
		app.Quit()
		return false
	})
}

func (app *AppWindow) handleSplit(sess *session.Session, vertical bool) {
	tab := app.TabView.FindTabBySession(sess)
	if tab == nil {
		tab = app.TabView.GetCurrentTab()
	}
	if tab == nil {
		log.Printf("[APP] Cannot split: no active tab found")
		return
	}

	var targetHost *storage.Host
	if sess != nil && sess.Host != nil {
		targetHost = sess.Host
	} else if tab.FocusedPane != nil && tab.FocusedPane.Session != nil && tab.FocusedPane.Session.Host != nil {
		targetHost = tab.FocusedPane.Session.Host
	} else if tab.Session != nil && tab.Session.Host != nil {
		targetHost = tab.Session.Host
	}

	if targetHost == nil {
		targetHost = &storage.Host{
			ID:           "local",
			Name:         "Локальный терминал",
			Protocol:     storage.ProtoLocal,
			TerminalType: "xterm-256color",
		}
	}

	log.Printf("[APP] Splitting tab %q for host %s (vertical=%v)", tab.Session.Title, targetHost.Name, vertical)
	app.StatusLabel.SetText(i18n.T("Разделение экрана...", "Splitting screen..."))

	term, err := vte.NewTerminal()
	if err != nil {
		app.StatusLabel.SetText(i18n.T("Ошибка создания VTE виджета: ", "Error creating VTE widget: ") + err.Error())
		return
	}
	slaveFile, err := term.SetupNativePTY()
	if err != nil {
		app.StatusLabel.SetText(i18n.T("Ошибка инициализации PTY: ", "Error initializing PTY: ") + err.Error())
		return
	}

	if targetHost.FontName != "" {
		term.SetFont(targetHost.FontName)
	} else if app.settings.DefaultFont != "" {
		term.SetFont(app.settings.DefaultFont)
	}
	if targetHost.ColorScheme != "" {
		term.ApplyColorScheme(targetHost.ColorScheme)
	} else if app.settings.DefaultColorScheme != "" {
		term.ApplyColorScheme(app.settings.DefaultColorScheme)
	}

	bridge := pty.FromSlave(slaveFile)
	go func() {
		newSess, err := session.StartSessionWithBridge(context.Background(), targetHost, tab.Session.Title, app.settings.DefaultLogsDir, bridge, nil)
		glib.IdleAdd(func() {
			if err != nil {
				app.StatusLabel.SetText(i18n.T("Ошибка создания сплита: ", "Error creating split: ") + err.Error())
				log.Printf("[APP] ERROR creating split session: %v", err)
				return
			}
			app.manager.Register(newSess)
			app.setupSessionDirectorySync(newSess, term)

			// Propagate window resize to PTY and remote SSH
			term.OnResize = func(rows, cols int) {
				newSess.Resize(rows, cols)
			}

			err = app.TabView.SplitActiveTab(tab, newSess, term, vertical)
			if err != nil {
				log.Printf("[APP] ERROR in SplitActiveTab: %v", err)
			} else {
				app.StatusLabel.SetText(i18n.T("Экран успешно разделен", "Screen split successfully"))
				log.Printf("[APP] Split created successfully")
			}

			if app.settings.AutoRestoreSessions {
				app.SaveAllSessionState()
			}

			app.attachSessionExitHandler(newSess, term, targetHost, tab.Session.Title)
			app.wireSerialAutoBaud(newSess, targetHost, term)
		})
	}()
}

// OpenLocalTerminal opens a new tab with an interactive local terminal session
func (app *AppWindow) OpenLocalTerminal() {
	localHost := &storage.Host{
		ID:           fmt.Sprintf("local-%d", time.Now().UnixNano()),
		Name:         i18n.T("Локальный терминал", "Local Terminal"),
		Protocol:     storage.ProtoLocal,
		Host:         "/bin/bash",
		TerminalType: "xterm-256color",
	}
	app.ConnectToHost(localHost)
}

// ConnectToHost opens a new session and attaches it to a new tab
func (app *AppWindow) ConnectToHost(host *storage.Host) {
	log.Printf("[APP] ConnectToHost initiated for: %s (%s:%d, proto=%s)", host.Name, host.Host, host.Port, host.Protocol)
	if host.Protocol == storage.ProtoRDP {
		app.connectToRDP(host)
		return
	}
	if host.Protocol == storage.ProtoVNC {
		app.connectToVNC(host)
		return
	}
	app.StatusLabel.SetText(i18n.Tf("Подключение к %s...", "Connecting to %s...", host.Host))

	term, err := vte.NewTerminal()
	if err != nil {
		app.StatusLabel.SetText(i18n.T("Ошибка создания VTE виджета: ", "Error creating VTE widget: ") + err.Error())
		log.Printf("[APP] ERROR creating VTE terminal: %v", err)
		return
	}

	slaveFile, err := term.SetupNativePTY()
	if err != nil {
		app.StatusLabel.SetText(i18n.T("Ошибка инициализации PTY: ", "Error initializing PTY: ") + err.Error())
		log.Printf("[APP] ERROR initializing native PTY: %v", err)
		return
	}

	if host.FontName != "" {
		term.SetFont(host.FontName)
	} else if app.settings.DefaultFont != "" {
		term.SetFont(app.settings.DefaultFont)
	}

	if host.ColorScheme != "" {
		term.ApplyColorScheme(host.ColorScheme)
	} else if app.settings.DefaultColorScheme != "" {
		term.ApplyColorScheme(app.settings.DefaultColorScheme)
	}

	bridge := pty.FromSlave(slaveFile)

	go func() {
		var jumpClient *cryptoSsh.Client
		if host.ProxyJumpHost != "" {
			log.Printf("[APP] ProxyJump configured: %s", host.ProxyJumpHost)
			jumpClient = app.resolveJumpClient(host)
		}

		sess, err := session.StartSessionWithBridge(context.Background(), host, host.Name, app.settings.DefaultLogsDir, bridge, jumpClient)
		glib.IdleAdd(func() {
			if err != nil {
				errMsg := err.Error()
				app.StatusLabel.SetText(i18n.T("Ошибка подключения: ", "Connection error: ") + errMsg)
				log.Printf("[APP] ERROR connecting to host %s: %v", host.Name, err)

				if host.Protocol == storage.ProtoSerial && strings.Contains(strings.ToLower(errMsg), "permission denied") {
					app.showSerialPermissionDialog(host, errMsg)
					return
				}

				msgText := i18n.Tf("Не удалось подключиться к \"%s\":\n\n%s", "Failed to connect to \"%s\":\n\n%s", host.Name, errMsg)
				dlg := gtk.MessageDialogNew(
					app.Window,
					gtk.DIALOG_MODAL,
					gtk.MESSAGE_ERROR,
					gtk.BUTTONS_OK,
					"%s",
					msgText,
				)
				dlg.SetTitle(i18n.T("Ошибка подключения", "Connection Error"))
				dlg.Run()
				dlg.Destroy()
				return
			}

			app.manager.Register(sess)
			app.setupSessionDirectorySync(sess, term)

			// Propagate window resize to PTY and remote SSH
			term.OnResize = func(rows, cols int) {
				sess.Resize(rows, cols)
			}

			// Add to notebook tab
			_, _ = app.TabView.AddTab(sess, term)
			app.NotesPanel.LoadSessionNotes(sess)

			if sess.SFTPClient != nil {
				app.SFTPPanel.AttachClient(sess.ID, host.ID, sess.SFTPClient, app.settings.DefaultEditor)
			}

			app.StatusLabel.SetText(i18n.Tf("Подключено: %s (%s)", "Connected: %s (%s)", host.Name, host.Host))
			log.Printf("[APP] Successfully connected and opened tab for: %s (sessionID=%s)", host.Name, sess.ID)

			if app.settings.AutoRestoreSessions {
				app.SaveAllSessionState()
			}

			app.attachSessionExitHandler(sess, term, host, host.Name)
			app.wireSerialAutoBaud(sess, host, term)
		})
	}()
}

// connectToRDP establishes a Remote Desktop connection either embedded in a tab or in a separate window
func (app *AppWindow) connectToRDP(host *storage.Host) {
	log.Printf("[APP] Initiating RDP connection to %s (%s:%d)", host.Name, host.Host, host.Port)
	app.StatusLabel.SetText(i18n.Tf("Подключение по RDP к %s...", "Connecting via RDP to %s...", host.Host))

	var jumpClient *cryptoSsh.Client
	if host.ProxyJumpHost != "" {
		log.Printf("[APP] ProxyJump configured for RDP: %s", host.ProxyJumpHost)
		jumpClient = app.resolveJumpClient(host)
	}

	sess := &session.Session{
		ID:    fmt.Sprintf("sess-%d", time.Now().UnixNano()),
		Host:  host,
		Title: host.Name,
		Notes: host.Notes,
	}

	if host.RDPEmbedded {
		socket, err := gtk.SocketNew()
		if err != nil {
			app.ShowError(i18n.T("Ошибка RDP", "RDP Error"), i18n.Tf("Не удалось создать контейнер окна RDP: %v", "Failed to create RDP window container: %v", err.Error()))
			return
		}
		socket.SetHExpand(true)
		socket.SetVExpand(true)

		tabItem, err := app.TabView.AddRDPTab(sess, socket)
		if err != nil {
			app.ShowError(i18n.T("Ошибка вкладки", "Tab Error"), err.Error())
			return
		}
		app.manager.Register(sess)
		app.NotesPanel.LoadSessionNotes(sess)

		socket.Show()
		socket.Realize()
		xid := socket.GetId()
		log.Printf("[APP] Embedded RDP socket realized with XID=0x%x", xid)

		go func() {
			rdpSess, err := rdp.ConnectRDP(context.Background(), host, xid, jumpClient)
			glib.IdleAdd(func() {
				if err != nil {
					app.StatusLabel.SetText(i18n.T("Ошибка подключения RDP: ", "RDP connection error: ") + err.Error())
					app.ShowError(i18n.T("Ошибка подключения RDP", "RDP Connection Error"), err.Error())
					app.TabView.CloseTab(tabItem)
					return
				}
				sess.RDPSession = rdpSess
				app.StatusLabel.SetText(i18n.Tf("Подключено (RDP): %s", "Connected (RDP): %s", host.Name))

				rdpSess.OnExit = func(exitErr error) {
					glib.IdleAdd(func() {
						log.Printf("[APP] RDP session %s exited: %v", host.Name, exitErr)
						app.StatusLabel.SetText(i18n.Tf("RDP сессия %s завершена", "RDP session %s closed", host.Name))
						app.TabView.CloseTab(tabItem)
					})
				}
			})
		}()
	} else {
		// Launch in separate standalone window
		go func() {
			rdpSess, err := rdp.ConnectRDP(context.Background(), host, 0, jumpClient)
			glib.IdleAdd(func() {
				if err != nil {
					app.StatusLabel.SetText(i18n.T("Ошибка запуска RDP: ", "RDP launch error: ") + err.Error())
					app.ShowError(i18n.T("Ошибка RDP", "RDP Error"), err.Error())
					return
				}
				sess.RDPSession = rdpSess
				app.manager.Register(sess)
				app.StatusLabel.SetText(i18n.Tf("Запущена RDP сессия (окно): %s", "RDP window session started: %s", host.Name))

				rdpSess.OnExit = func(exitErr error) {
					glib.IdleAdd(func() {
						log.Printf("[APP] Standalone RDP session %s exited: %v", host.Name, exitErr)
						app.manager.Unregister(sess.ID)
						app.StatusLabel.SetText(i18n.Tf("RDP сессия %s завершена", "RDP session %s closed", host.Name))
					})
				}
			})
		}()
	}
}

// connectToVNC establishes a Virtual Network Computing (VNC) connection either embedded in a tab or in a separate window
func (app *AppWindow) connectToVNC(host *storage.Host) {
	log.Printf("[APP] Initiating VNC connection to %s (%s:%d)", host.Name, host.Host, host.Port)
	app.StatusLabel.SetText(i18n.Tf("Подключение по VNC к %s...", "Connecting via VNC to %s...", host.Host))

	var jumpClient *cryptoSsh.Client
	if host.ProxyJumpHost != "" {
		log.Printf("[APP] ProxyJump configured for VNC: %s", host.ProxyJumpHost)
		jumpClient = app.resolveJumpClient(host)
	}

	sess := &session.Session{
		ID:    fmt.Sprintf("sess-%d", time.Now().UnixNano()),
		Host:  host,
		Title: host.Name,
		Notes: host.Notes,
	}

	if host.VNCEmbedded {
		socket, err := gtk.SocketNew()
		if err != nil {
			app.ShowError(i18n.T("Ошибка VNC", "VNC Error"), i18n.Tf("Не удалось создать контейнер окна VNC: %v", "Failed to create VNC window container: %v", err.Error()))
			return
		}
		socket.SetHExpand(true)
		socket.SetVExpand(true)

		tabItem, err := app.TabView.AddVNCTab(sess, socket)
		if err != nil {
			app.ShowError(i18n.T("Ошибка вкладки", "Tab Error"), err.Error())
			return
		}
		app.manager.Register(sess)
		app.NotesPanel.LoadSessionNotes(sess)

		socket.Show()
		socket.Realize()
		xid := socket.GetId()
		log.Printf("[APP] Embedded VNC socket realized with XID=0x%x", xid)

		go func() {
			vncSess, err := vnc.ConnectVNC(context.Background(), host, xid, jumpClient)
			glib.IdleAdd(func() {
				if err != nil {
					app.StatusLabel.SetText(i18n.T("Ошибка подключения VNC: ", "VNC connection error: ") + err.Error())
					app.ShowError(i18n.T("Ошибка подключения VNC", "VNC Connection Error"), err.Error())
					app.TabView.CloseTab(tabItem)
					return
				}
				sess.VNCSession = vncSess
				app.StatusLabel.SetText(i18n.Tf("Подключено (VNC): %s", "Connected (VNC): %s", host.Name))

				vncSess.OnExit = func(exitErr error) {
					glib.IdleAdd(func() {
						log.Printf("[APP] VNC session %s exited: %v", host.Name, exitErr)
						app.StatusLabel.SetText(i18n.Tf("VNC сессия %s завершена", "VNC session %s closed", host.Name))
						app.TabView.CloseTab(tabItem)
					})
				}
			})
		}()
	} else {
		// Launch in separate standalone window
		go func() {
			vncSess, err := vnc.ConnectVNC(context.Background(), host, 0, jumpClient)
			glib.IdleAdd(func() {
				if err != nil {
					app.StatusLabel.SetText(i18n.T("Ошибка запуска VNC: ", "VNC launch error: ") + err.Error())
					app.ShowError(i18n.T("Ошибка VNC", "VNC Error"), err.Error())
					return
				}
				sess.VNCSession = vncSess
				app.manager.Register(sess)
				app.StatusLabel.SetText(i18n.Tf("Запущена VNC сессия (окно): %s", "VNC window session started: %s", host.Name))

				vncSess.OnExit = func(exitErr error) {
					glib.IdleAdd(func() {
						log.Printf("[APP] Standalone VNC session %s exited: %v", host.Name, exitErr)
						app.manager.Unregister(sess.ID)
						app.StatusLabel.SetText(i18n.Tf("VNC сессия %s завершена", "VNC session %s closed", host.Name))
					})
				}
			})
		}()
	}
}

// RestoreSavedSessions restores tabs and nested split panes from database
func (app *AppWindow) RestoreSavedSessions() {
	savedSessions, err := app.store.GetSavedSessions()
	if err != nil || len(savedSessions) == 0 {
		log.Printf("[RESTORE] No saved sessions found to restore.")
		app.restoreMu.Lock()
		app.isRestoring = false
		app.restoreMu.Unlock()
		return
	}

	log.Printf("[RESTORE] Restoring %d saved session(s)...", len(savedSessions))
	app.StatusLabel.SetText(i18n.Tf("Восстановление %d сессий...", "Restoring %d sessions...", len(savedSessions)))

	// Group sessions by TabIndex
	type tabGroup struct {
		primary storage.SavedSessionState
		splits  []storage.SavedSessionState
	}

	groups := make(map[int]*tabGroup)
	var tabIndices []int

	for _, st := range savedSessions {
		g, exists := groups[st.TabIndex]
		if !exists {
			g = &tabGroup{}
			groups[st.TabIndex] = g
			tabIndices = append(tabIndices, st.TabIndex)
		}
		if st.SplitParentID == "" || st.SplitParentID == "none" || g.primary.ID == "" {
			if g.primary.ID == "" {
				g.primary = st
			} else {
				g.splits = append(g.splits, st)
			}
		} else {
			g.splits = append(g.splits, st)
		}
	}

	sort.Ints(tabIndices)

	totalSessions := len(savedSessions)
	restoredCount := 0

	finishSessionRestore := func() {
		restoredCount++
		if restoredCount >= totalSessions {
			app.restoreMu.Lock()
			app.isRestoring = false
			app.restoreMu.Unlock()
			log.Printf("[RESTORE] All %d sessions restored. isRestoring cleared.", restoredCount)

			app.TabView.ReorderTabsByTargetIndex()
			if savedActiveIdx, sErr := app.store.GetSetting("active_tab_index"); sErr == nil && savedActiveIdx != "" {
				if idx, aErr := strconv.Atoi(savedActiveIdx); aErr == nil && idx >= 0 && idx < len(app.TabView.items) {
					app.TabView.Notebook.SetCurrentPage(idx)
				}
			}
		}
	}

	for _, tIdx := range tabIndices {
		grp := groups[tIdx]
		st := grp.primary
		splits := grp.splits
		targetTabIndex := tIdx

		var h *storage.Host
		hostFoundInDB := false
		if st.HostID != "" {
			var dbErr error
			h, dbErr = app.store.GetHost(st.HostID)
			if h != nil {
				hostFoundInDB = true
				log.Printf("[RESTORE] Found host in DB: id=%q name=%q proto=%s host=%q port=%d", h.ID, h.Name, h.Protocol, h.Host, h.Port)
			} else {
				log.Printf("[RESTORE] GetHost(%q) returned nil, err=%v", st.HostID, dbErr)
			}
		}
		if h == nil {
			// Host not found in DB — try the saved snapshot (Quick Connect hosts)
			if st.HostSnapshot != "" {
				var snapHost storage.Host
				if err := json.Unmarshal([]byte(st.HostSnapshot), &snapHost); err == nil {
					h = &snapHost
					log.Printf("[RESTORE] Using host snapshot for quick-connect session %q", st.Title)
				}
			}
			// Still nil — fall back to local terminal with history
			if h == nil {
				log.Printf("[RESTORE] Host %q not found in DB and no snapshot, restoring as local terminal", st.Title)
				h = &storage.Host{
					ID:             st.HostID,
					Name:           st.Title,
					Protocol:       storage.ProtoLocal,
					TerminalType:   "xterm-256color",
					RestoreHistory: true,
				}
			}
		}
		_ = hostFoundInDB // used for future logic if needed

		term, err := vte.NewTerminal()
		if err != nil {
			totalSessions -= (1 + len(splits))
			continue
		}
		slaveFile, err := term.SetupNativePTY()
		if err != nil {
			totalSessions -= (1 + len(splits))
			continue
		}
		if h.FontName != "" {
			term.SetFont(h.FontName)
		}
		if h.ColorScheme != "" {
			term.ApplyColorScheme(h.ColorScheme)
		}
		bridge := pty.FromSlave(slaveFile)

		savedState := st
		hostCopy := h
		splitStates := splits

		go func() {
			log.Printf("[RESTORE] Starting session %q: host_id=%q proto=%s host=%q port=%d user=%q key=%q",
				savedState.Title, hostCopy.ID, hostCopy.Protocol, hostCopy.Host, hostCopy.Port, hostCopy.Username, hostCopy.KeyPath)
			sess, err := session.StartSessionWithBridge(context.Background(), hostCopy, savedState.Title, app.settings.DefaultLogsDir, bridge, nil)
			glib.IdleAdd(func() {
				if err != nil {
					log.Printf("[RESTORE] Failed to start session for %s: %v", savedState.Title, err)
					sess = &session.Session{
						ID:    savedState.ID,
						Host:  hostCopy,
						Title: savedState.Title,
						Notes: savedState.Notes,
					}
					app.manager.Register(sess)

					if savedState.ScrollbackDump != "" {
						restored := session.FormatRestoredText(savedState.ScrollbackDump, savedState.SavedAt)
						if restored != "" {
							term.FeedText(restored)
						}
					}

					tabItem, _ := app.TabView.AddTab(sess, term)
					tabItem.TargetTabIndex = targetTabIndex
					app.TabView.ReorderTabsByTargetIndex()

					app.NotesPanel.LoadSessionNotes(sess)
					app.setSessionDisconnected(sess, term, hostCopy, savedState.Title, err)
					finishSessionRestore()

					// Sort split children by PaneIndex so parent panes are restored before their children
					sort.Slice(splitStates, func(i, j int) bool {
						return splitStates[i].PaneIndex < splitStates[j].PaneIndex
					})

					// Restore split children into this tabItem sequentially
					var restoreNextSplit func(idx int)
					restoreNextSplit = func(idx int) {
						if idx >= len(splitStates) {
							return
						}
						chState := splitStates[idx]
						app.restoreSplitPane(tabItem, chState, func() {
							finishSessionRestore()
							restoreNextSplit(idx + 1)
						})
					}
					if len(splitStates) > 0 {
						restoreNextSplit(0)
					}
					return
				}
				sess.ID = savedState.ID
				sess.Notes = savedState.Notes
				app.manager.Register(sess)
				app.setupSessionDirectorySync(sess, term)

				term.OnResize = func(rows, cols int) {
					sess.Resize(rows, cols)
				}
				if savedState.ScrollbackDump != "" {
					restored := session.FormatRestoredText(savedState.ScrollbackDump, savedState.SavedAt)
					if restored != "" {
						term.FeedText(restored)
					}
				}

				tabItem, _ := app.TabView.AddTab(sess, term)
				tabItem.TargetTabIndex = targetTabIndex
				app.TabView.ReorderTabsByTargetIndex()

				app.NotesPanel.LoadSessionNotes(sess)
				if sess.SFTPClient != nil {
					app.SFTPPanel.AttachClient(sess.ID, hostCopy.ID, sess.SFTPClient, app.settings.DefaultEditor)
				}

				app.attachSessionExitHandler(sess, term, hostCopy, savedState.Title)
				app.wireSerialAutoBaud(sess, hostCopy, term)

				finishSessionRestore()

				// Sort split children by PaneIndex so parent panes are restored before their children
				sort.Slice(splitStates, func(i, j int) bool {
					return splitStates[i].PaneIndex < splitStates[j].PaneIndex
				})

				// Restore split children into this tabItem sequentially
				var restoreNextSplit func(idx int)
				restoreNextSplit = func(idx int) {
					if idx >= len(splitStates) {
						return
					}
					chState := splitStates[idx]
					app.restoreSplitPane(tabItem, chState, func() {
						finishSessionRestore()
						restoreNextSplit(idx + 1)
					})
				}
				if len(splitStates) > 0 {
					restoreNextSplit(0)
				}
			})
		}()
	}

	if totalSessions <= 0 {
		app.restoreMu.Lock()
		app.isRestoring = false
		app.restoreMu.Unlock()
	}
}

func (app *AppWindow) restoreSplitPane(tabItem *TabItem, st storage.SavedSessionState, onDone func()) {
	var h *storage.Host
	if st.HostID != "" {
		h, _ = app.store.GetHost(st.HostID)
	}
	if h == nil {
		// Try snapshot first (Quick Connect hosts not in DB)
		if st.HostSnapshot != "" {
			var snapHost storage.Host
			if err := json.Unmarshal([]byte(st.HostSnapshot), &snapHost); err == nil {
				h = &snapHost
			}
		}
		if h == nil {
			h = &storage.Host{
				ID:             st.HostID,
				Name:           st.Title,
				Protocol:       storage.ProtoLocal,
				TerminalType:   "xterm-256color",
				RestoreHistory: true,
			}
		}
	}

	term, err := vte.NewTerminal()
	if err != nil {
		if onDone != nil {
			onDone()
		}
		return
	}
	slaveFile, err := term.SetupNativePTY()
	if err != nil {
		if onDone != nil {
			onDone()
		}
		return
	}
	if h.FontName != "" {
		term.SetFont(h.FontName)
	}
	if h.ColorScheme != "" {
		term.ApplyColorScheme(h.ColorScheme)
	}
	bridge := pty.FromSlave(slaveFile)

	go func() {
		sess, err := session.StartSessionWithBridge(context.Background(), h, st.Title, app.settings.DefaultLogsDir, bridge, nil)
		glib.IdleAdd(func() {
			defer func() {
				if onDone != nil {
					onDone()
				}
			}()

			if err != nil {
				log.Printf("[RESTORE] Failed to restore split session %s: %v", st.Title, err)
				sess = &session.Session{
					ID:    st.ID,
					Host:  h,
					Title: st.Title,
					Notes: st.Notes,
				}
				app.manager.Register(sess)

				if st.ScrollbackDump != "" {
					restored := session.FormatRestoredText(st.ScrollbackDump, st.SavedAt)
					if restored != "" {
						term.FeedText(restored)
					}
				}

				var targetParent *TerminalPane
				if st.SplitParentID != "" {
					targetParent = app.TabView.FindPaneBySessionID(tabItem, st.SplitParentID)
				}
				isVertical := st.SplitDirection == "vertical" || st.SplitDirection == "left-right"
				_ = app.TabView.SplitPane(tabItem, targetParent, sess, term, isVertical)
				app.setSessionDisconnected(sess, term, h, st.Title, err)
				return
			}
			sess.ID = st.ID
			sess.Notes = st.Notes
			app.manager.Register(sess)

			term.OnResize = func(rows, cols int) {
				sess.Resize(rows, cols)
			}
			if st.ScrollbackDump != "" {
				restored := session.FormatRestoredText(st.ScrollbackDump, st.SavedAt)
				if restored != "" {
					term.FeedText(restored)
				}
			}

			var targetParent *TerminalPane
			if st.SplitParentID != "" {
				targetParent = app.TabView.FindPaneBySessionID(tabItem, st.SplitParentID)
			}
			isVertical := st.SplitDirection == "vertical" || st.SplitDirection == "left-right"
			_ = app.TabView.SplitPane(tabItem, targetParent, sess, term, isVertical)
			app.attachSessionExitHandler(sess, term, h, st.Title)
			app.wireSerialAutoBaud(sess, h, term)
		})
	}()
}

// SaveAllSessionState dumps current tab layout and all nested split panes into SQLite
func (app *AppWindow) SaveAllSessionState() {
	app.restoreMu.Lock()
	if app.isRestoring {
		app.restoreMu.Unlock()
		return
	}
	app.restoreMu.Unlock()

	// Ensure items slice reflects current notebook page order
	app.TabView.SyncItemsWithNotebook()

	// Save active tab index
	curPage := app.TabView.Notebook.GetCurrentPage()
	if curPage >= 0 {
		_ = app.store.SaveSetting("active_tab_index", fmt.Sprintf("%d", curPage))
	}

	var states []storage.SavedSessionState
	for sliceIdx, item := range app.TabView.items {
		tabIdx := app.TabView.Notebook.PageNum(item.ContentBox)
		if tabIdx < 0 {
			tabIdx = sliceIdx
		}
		for paneIdx, pane := range item.Panes {
			s := pane.Session
			if s == nil {
				continue
			}
			hostID := ""
			protocol := storage.ProtoLocal
			hostSnapshot := ""
			if s.Host != nil {
				hostID = s.Host.ID
				protocol = s.Host.Protocol
				// For Quick Connect hosts (not saved in the DB), embed the full
				// host config as JSON so it can be restored without a DB lookup.
				if strings.HasPrefix(hostID, "quick-") {
					if snap, err := json.Marshal(s.Host); err == nil {
						hostSnapshot = string(snap)
					}
				}
			}
			scrollback := ""
			if pane.Terminal != nil {
				scrollback = pane.Terminal.GetCleanText()
			}
			if scrollback == "" {
				scrollback = s.GetScrollbackText()
			}
			scrollback = session.CleanScrollbackDump(scrollback)
			workingDir := "/"
			if s.SFTPClient != nil {
				workingDir = s.SFTPClient.CurrentDir()
			}
			parentID := ""
			splitDir := "none"
			if paneIdx > 0 {
				parentID = pane.ParentSessionID
				if parentID == "" && len(item.Panes) > 0 && item.Panes[0].Session != nil {
					parentID = item.Panes[0].Session.ID
				}
				splitDir = pane.SplitDirection
				if splitDir == "" {
					splitDir = "horizontal"
				}
			}

			st := storage.SavedSessionState{
				ID:             s.ID,
				HostID:         hostID,
				Title:          s.Title,
				Protocol:       protocol,
				TabIndex:       tabIdx,
				PaneIndex:      paneIdx,
				SplitParentID:  parentID,
				SplitDirection: splitDir,
				WorkingDir:     workingDir,
				ScrollbackDump: scrollback,
				Notes:          s.Notes,
				SavedAt:        time.Now(),
				HostSnapshot:   hostSnapshot,
			}
			states = append(states, st)
		}
	}
	hasher := sha256.New()
	for _, st := range states {
		fmt.Fprintf(hasher, "%s|%s|%s|%s|%d|%d|%s|%s|%s|%s|%s;",
			st.ID, st.HostID, st.Title, st.Protocol, st.TabIndex, st.PaneIndex,
			st.SplitParentID, st.SplitDirection, st.WorkingDir, st.Notes, st.ScrollbackDump)
	}
	currentHash := hex.EncodeToString(hasher.Sum(nil))

	app.lastSavedMu.Lock()
	if currentHash == app.lastSavedHash {
		app.lastSavedMu.Unlock()
		return
	}
	app.lastSavedHash = currentHash
	app.lastSavedMu.Unlock()

	log.Printf("[STATE] Saving %d active pane(s) across %d tab(s) to SQLite...", len(states), len(app.TabView.items))
	go func(st []storage.SavedSessionState) {
		if err := app.store.SaveActiveSessions(st); err != nil {
			log.Printf("[STATE] Error saving active sessions to SQLite: %v", err)
		}
	}(states)
}

// SaveAllSessionStateSync dumps and writes state synchronously to SQLite (used during Quit)
func (app *AppWindow) SaveAllSessionStateSync() {
	app.restoreMu.Lock()
	if app.isRestoring {
		app.restoreMu.Unlock()
		return
	}
	app.restoreMu.Unlock()

	app.TabView.SyncItemsWithNotebook()

	curPage := app.TabView.Notebook.GetCurrentPage()
	if curPage >= 0 {
		_ = app.store.SaveSetting("active_tab_index", fmt.Sprintf("%d", curPage))
	}

	var states []storage.SavedSessionState
	for sliceIdx, item := range app.TabView.items {
		tabIdx := app.TabView.Notebook.PageNum(item.ContentBox)
		if tabIdx < 0 {
			tabIdx = sliceIdx
		}
		for paneIdx, pane := range item.Panes {
			s := pane.Session
			if s == nil {
				continue
			}
			hostID := ""
			protocol := storage.ProtoLocal
			hostSnapshot := ""
			if s.Host != nil {
				hostID = s.Host.ID
				protocol = s.Host.Protocol
				if strings.HasPrefix(hostID, "quick-") {
					if snap, err := json.Marshal(s.Host); err == nil {
						hostSnapshot = string(snap)
					}
				}
			}
			scrollback := ""
			if pane.Terminal != nil {
				scrollback = pane.Terminal.GetCleanText()
			}
			if scrollback == "" {
				scrollback = s.GetScrollbackText()
			}
			scrollback = session.CleanScrollbackDump(scrollback)
			workingDir := "/"
			if s.SFTPClient != nil {
				workingDir = s.SFTPClient.CurrentDir()
			}
			parentID := ""
			splitDir := "none"
			if paneIdx > 0 {
				parentID = pane.ParentSessionID
				if parentID == "" && len(item.Panes) > 0 && item.Panes[0].Session != nil {
					parentID = item.Panes[0].Session.ID
				}
				splitDir = pane.SplitDirection
				if splitDir == "" {
					splitDir = "horizontal"
				}
			}

			st := storage.SavedSessionState{
				ID:             s.ID,
				HostID:         hostID,
				Title:          s.Title,
				Protocol:       protocol,
				TabIndex:       tabIdx,
				PaneIndex:      paneIdx,
				SplitParentID:  parentID,
				SplitDirection: splitDir,
				WorkingDir:     workingDir,
				ScrollbackDump: scrollback,
				Notes:          s.Notes,
				SavedAt:        time.Now(),
				HostSnapshot:   hostSnapshot,
			}
			states = append(states, st)
		}
	}

	_ = app.store.SaveActiveSessions(states)
}

// Quit saves session states and exits cleanly
func (app *AppWindow) Quit() {
	log.Printf("[APP] Application quit requested. Active sessions: %d", len(app.manager.GetAll()))
	if app.settings.AutoRestoreSessions {
		app.SaveAllSessionStateSync()
	}
	app.manager.CloseAll()
	_ = app.watcherMgr.Close()
	_ = app.store.Close()
	if app.trayIcon != nil {
		app.trayIcon.Destroy()
	}
	log.Printf("[APP] Exiting GTK main loop.")
	gtk.MainQuit()
	os.Exit(0)
}

// resolveJumpClient creates an SSH client to the proxy jump host if configured
func (app *AppWindow) resolveJumpClient(host *storage.Host) *cryptoSsh.Client {
	if host.ProxyJumpHost == "" {
		return nil
	}
	jumpHost, err := app.store.GetHost(host.ProxyJumpHost)
	if err != nil || jumpHost == nil {
		log.Printf("[APP] resolveJumpClient: jump host %s not found: %v", host.ProxyJumpHost, err)
		return nil
	}
	jClient, _, jErr := engineSSH.DialSSH(context.Background(), jumpHost, nil)
	if jErr != nil {
		log.Printf("[APP] resolveJumpClient: failed to connect to jump host %s: %v", host.ProxyJumpHost, jErr)
		return nil
	}
	return jClient
}

// setSessionDisconnected displays error/disconnect prompt in terminal, updates status, and wires Enter to reconnect
func (app *AppWindow) setSessionDisconnected(sess *session.Session, term *vte.Terminal, host *storage.Host, tabTitle string, reasonErr error) {
	if term == nil || host == nil {
		return
	}

	if reasonErr != nil {
		term.FeedText("\r\n\x1b[1;31m" + i18n.T("Ошибка подключения: ", "Connection error: ") + reasonErr.Error() + "\x1b[0m\r\n")
	}
	term.FeedText("\x1b[1;33m" + i18n.T("[Сессия закрыта. Переподключить? (Нажмите Enter)]", "[Session closed. Reconnect? (Press Enter)]") + "\x1b[0m\r\n")
	app.StatusLabel.SetText(i18n.Tf("Сессия %s закрыта", "Session %s closed", host.Name))

	var reconnectFunc func()
	reconnectFunc = func() {
		term.SetDisconnected(false, nil)
		term.FeedText("\r\n\x1b[1;36m" + i18n.T("[Переподключение...]", "[Reconnecting...]") + "\x1b[0m\r\n")
		app.StatusLabel.SetText(i18n.Tf("Переподключение к %s...", "Reconnecting to %s...", host.Host))

		slaveFile, err := term.SetupNativePTY()
		if err != nil {
			term.FeedText("\r\n\x1b[1;31m" + i18n.T("Ошибка инициализации PTY: ", "PTY initialization error: ") + err.Error() + "\x1b[0m\r\n")
			term.FeedText("\x1b[1;33m" + i18n.T("[Сессия закрыта. Переподключить? (Нажмите Enter)]", "[Session closed. Reconnect? (Press Enter)]") + "\x1b[0m\r\n")
			term.SetDisconnected(true, reconnectFunc)
			return
		}

		bridge := pty.FromSlave(slaveFile)
		go func() {
			if sess != nil {
				_ = sess.Close()
				app.manager.Unregister(sess.ID)
			}
			jumpClient := app.resolveJumpClient(host)
			newSess, err := session.StartSessionWithBridge(context.Background(), host, tabTitle, app.settings.DefaultLogsDir, bridge, jumpClient)
			glib.IdleAdd(func() {
				if err != nil {
					term.FeedText("\r\n\x1b[1;31m" + i18n.T("Ошибка подключения: ", "Connection error: ") + err.Error() + "\x1b[0m\r\n")
					term.FeedText("\x1b[1;33m" + i18n.T("[Сессия закрыта. Переподключить? (Нажмите Enter)]", "[Session closed. Reconnect? (Press Enter)]") + "\x1b[0m\r\n")
					term.SetDisconnected(true, reconnectFunc)
					app.StatusLabel.SetText(i18n.T("Ошибка переподключения: ", "Reconnection error: ") + err.Error())
					return
				}

				app.manager.Register(newSess)

				app.TabView.UpdateSessionForTerminal(term, newSess)

				term.OnResize = func(rows, cols int) {
					newSess.Resize(rows, cols)
				}
				rows := term.GetRowCount()
				cols := term.GetColumnCount()
				if rows > 0 && cols > 0 {
					newSess.Resize(rows, cols)
				}

				if newSess.SFTPClient != nil {
					app.SFTPPanel.AttachClient(newSess.ID, host.ID, newSess.SFTPClient, app.settings.DefaultEditor)
				}

				app.StatusLabel.SetText(i18n.Tf("Подключено: %s (%s)", "Connected: %s (%s)", host.Name, host.Host))
				log.Printf("[APP] Reconnected session for: %s (new sessionID=%s)", host.Name, newSess.ID)

				if app.settings.AutoRestoreSessions {
					app.SaveAllSessionState()
				}

				app.attachSessionExitHandler(newSess, term, host, tabTitle)
				app.wireSerialAutoBaud(newSess, host, term)
			})
		}()
	}

	term.SetDisconnected(true, reconnectFunc)
	if sess != nil {
		if app.activeBaudBars != nil {
			if bar, ok := app.activeBaudBars[sess.ID]; ok && bar != nil {
				bar.Destroy()
				delete(app.activeBaudBars, sess.ID)
			}
		}
		app.TabView.UpdateTabTitleForSession(sess)
	}
}

// attachSessionExitHandler monitors session termination and prompts user to reconnect via Enter
func (app *AppWindow) attachSessionExitHandler(sess *session.Session, term *vte.Terminal, host *storage.Host, tabTitle string) {
	if sess == nil || term == nil || host == nil {
		return
	}

	sess.OnExit = func(exitErr error) {
		log.Printf("[Session] Session %s exited: %v", host.Name, exitErr)
		glib.IdleAdd(func() {
			if term.IsDisconnected() {
				return
			}
			// Verify tab still exists
			if app.TabView.FindTabBySession(sess) == nil {
				return
			}
			app.setSessionDisconnected(sess, term, host, tabTitle, exitErr)
		})
	}
}

// promptInputDialog shows a modal input dialog to specify or rename a string value
func promptInputDialog(parent gtk.IWindow, title, labelText, defaultValue string) (string, bool) {
	dlg, err := gtk.DialogNew()
	if err != nil {
		return "", false
	}
	dlg.SetTitle(title)
	dlg.SetModal(true)
	dlg.SetDefaultSize(320, 110)
	if parent != nil {
		if pw, ok := parent.(*gtk.Window); ok {
			dlg.SetTransientFor(pw)
		}
	}

	contentArea, err := dlg.GetContentArea()
	if err != nil {
		dlg.Destroy()
		return "", false
	}
	contentArea.SetSpacing(8)
	contentArea.SetMarginStart(12)
	contentArea.SetMarginEnd(12)
	contentArea.SetMarginTop(12)
	contentArea.SetMarginBottom(12)

	lbl, _ := gtk.LabelNew(labelText)
	lbl.SetXAlign(0)
	contentArea.Add(lbl)

	entry, _ := gtk.EntryNew()
	entry.SetText(defaultValue)
	entry.SetActivatesDefault(true)
	contentArea.Add(entry)

	_, _ = dlg.AddButton(i18n.T("Отмена", "Cancel"), gtk.RESPONSE_CANCEL)
	btnOk, _ := dlg.AddButton(i18n.T("Сохранить", "Save"), gtk.RESPONSE_OK)
	btnOk.SetCanDefault(true)
	dlg.SetDefault(btnOk)

	dlg.ShowAll()
	response := dlg.Run()
	val, _ := entry.GetText()
	val = strings.TrimSpace(val)
	dlg.Destroy()

	if response == gtk.RESPONSE_OK && val != "" {
		return val, true
	}
	return "", false
}

// promptFolderDialog shows a modal input dialog to specify or rename a folder
func promptFolderDialog(parent gtk.IWindow, title, defaultName string) (string, bool) {
	return promptInputDialog(parent, title, i18n.T("Имя папки:", "Folder name:"), defaultName)
}

func (app *AppWindow) setupSessionDirectorySync(sess *session.Session, term *vte.Terminal) {
	if sess == nil {
		return
	}
	sess.OnSFTPReady = func(sftpCl *sftp.Client) {
		glib.IdleAdd(func() {
			currTab := app.TabView.GetCurrentTab()
			if currTab == nil {
				return
			}
			var activeSess *session.Session
			if currTab.FocusedPane != nil && currTab.FocusedPane.Session != nil {
				activeSess = currTab.FocusedPane.Session
			} else {
				activeSess = currTab.Session
			}
			if activeSess == sess {
				hostID := ""
				if sess.Host != nil {
					hostID = sess.Host.ID
				}
				app.SFTPPanel.AttachClient(sess.ID, hostID, sftpCl, app.settings.DefaultEditor)
			}
		})
	}
	if term != nil {
		term.OnDirectoryChanged = func(path string) {
			sess.NotifyDirectory(path)
		}
	}
	sess.OnDirectoryChanged = func(dirPath string) {
		if dirPath == "~" {
			if sess.SFTPClient != nil {
				dirPath = sess.SFTPClient.CurrentDir()
			}
		} else if strings.HasPrefix(dirPath, "~/") {
			if sess.SFTPClient != nil {
				dirPath = path.Join(sess.SFTPClient.CurrentDir(), strings.TrimPrefix(dirPath, "~/"))
			}
		}
		glib.IdleAdd(func() {
			currTab := app.TabView.GetCurrentTab()
			if currTab == nil {
				return
			}
			var activeSess *session.Session
			if currTab.FocusedPane != nil && currTab.FocusedPane.Session != nil {
				activeSess = currTab.FocusedPane.Session
			} else {
				activeSess = currTab.Session
			}
			if activeSess == sess {
				hostID := ""
				if sess.Host != nil {
					hostID = sess.Host.ID
				}
				if app.SFTPPanel.GetCurrentSessionID() != sess.ID && sess.SFTPClient != nil {
					app.SFTPPanel.AttachClient(sess.ID, hostID, sess.SFTPClient, app.settings.DefaultEditor)
				}
				app.SFTPPanel.LoadDirectory(dirPath)
			} else {
				if sess.SFTPClient != nil {
					sess.SFTPClient.SetCurrentDir(dirPath)
					app.SFTPPanel.InvalidateSessionCache(sess.ID, dirPath)
				}
			}
		})
	}
}

// ShowError displays an explicit modal error dialog
func (app *AppWindow) ShowError(title, msg string) {
	dlg := gtk.MessageDialogNew(app.Window, gtk.DIALOG_MODAL, gtk.MESSAGE_ERROR, gtk.BUTTONS_OK, "%s", msg)
	dlg.SetTitle(title)
	dlg.Run()
	dlg.Destroy()
}

// showSerialPermissionDialog shows an actionable dialog when serial port access is denied
func (app *AppWindow) showSerialPermissionDialog(host *storage.Host, errMsg string) {
	devPath := host.SerialPort
	if devPath == "" {
		devPath = host.Host
	}
	if devPath == "" {
		devPath = "/dev/ttyUSB0"
	}

	username := os.Getenv("USER")
	if username == "" {
		if u, err := user.Current(); err == nil {
			username = u.Username
		} else {
			username = "$USER"
		}
	}

	cmdStr := fmt.Sprintf("sudo usermod -aG dialout %s && sudo chmod 666 %s", username, devPath)

	dlg, err := gtk.DialogNew()
	if err != nil {
		app.ShowError(i18n.T("Ошибка подключения", "Connection Error"), errMsg)
		return
	}
	dlg.SetTitle(i18n.T("Ошибка доступа к COM-порту", "Serial Port Access Error"))
	dlg.SetTransientFor(app.Window)
	dlg.SetModal(true)
	dlg.SetDefaultSize(520, 240)

	contentArea, err := dlg.GetContentArea()
	if err != nil {
		dlg.Destroy()
		return
	}
	contentArea.SetSpacing(12)
	contentArea.SetMarginStart(16)
	contentArea.SetMarginEnd(16)
	contentArea.SetMarginTop(16)
	contentArea.SetMarginBottom(16)

	// Top row: Error icon + Description
	topBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 12)
	img, _ := gtk.ImageNewFromIconName("dialog-error", gtk.ICON_SIZE_DIALOG)
	topBox.PackStart(img, false, false, 0)

	textBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 6)
	titleLbl, _ := gtk.LabelNew("")
	titleLbl.SetMarkup(fmt.Sprintf("<b><big>%s</big></b>", i18n.Tf("Не удалось открыть \"%s\"", "Failed to open \"%s\"", host.Name)))
	titleLbl.SetXAlign(0)
	titleLbl.SetLineWrap(true)
	textBox.PackStart(titleLbl, false, false, 0)

	descLbl, _ := gtk.LabelNew("")
	descMarkup := fmt.Sprintf("%s\n\n<b>%s</b> %s",
		errMsg,
		i18n.T("Причина:", "Reason:"),
		i18n.Tf("у пользователя '%s' нет прав доступа к порту '%s' (требуется членство в группе 'dialout').",
			"user '%s' has no permission to access port '%s' (membership in 'dialout' group required).", username, devPath),
	)
	descLbl.SetMarkup(descMarkup)
	descLbl.SetXAlign(0)
	descLbl.SetLineWrap(true)
	descLbl.SetSelectable(true)
	textBox.PackStart(descLbl, false, false, 0)

	topBox.PackStart(textBox, true, true, 0)
	contentArea.Add(topBox)

	// Separator
	sep, _ := gtk.SeparatorNew(gtk.ORIENTATION_HORIZONTAL)
	contentArea.Add(sep)

	// Command Section
	cmdBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 6)
	cmdTitle, _ := gtk.LabelNew(i18n.T("Команда для ручного выполнения в терминале:", "Command to run manually in terminal:"))
	cmdTitle.SetXAlign(0)
	cmdBox.PackStart(cmdTitle, false, false, 0)

	cmdRow, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 6)
	entry, _ := gtk.EntryNew()
	entry.SetText(cmdStr)
	entry.SetEditable(false)
	entry.SetHExpand(true)
	cmdRow.PackStart(entry, true, true, 0)

	copyBtn, _ := gtk.ButtonNewWithLabel(i18n.T("📋 Скопировать", "📋 Copy"))
	copyBtn.SetTooltipText(i18n.T("Скопировать команду в буфер обмена", "Copy command to clipboard"))
	copyBtn.Connect("clicked", func() {
		if clip, cErr := gtk.ClipboardGet(gdk.SELECTION_CLIPBOARD); cErr == nil && clip != nil {
			clip.SetText(cmdStr)
		}
		if pClip, pErr := gtk.ClipboardGet(gdk.SELECTION_PRIMARY); pErr == nil && pClip != nil {
			pClip.SetText(cmdStr)
		}
		copyBtn.SetLabel(i18n.T("✓ Скопировано!", "✓ Copied!"))
		glib.TimeoutAdd(2000, func() bool {
			copyBtn.SetLabel(i18n.T("📋 Скопировать", "📋 Copy"))
			return false
		})
	})
	cmdRow.PackStart(copyBtn, false, false, 0)
	cmdBox.PackStart(cmdRow, false, false, 0)
	contentArea.Add(cmdBox)

	// Buttons
	_, _ = dlg.AddButton(i18n.T("Закрыть", "Close"), gtk.RESPONSE_CLOSE)
	btnApply, _ := dlg.AddButton(i18n.T("⚡ Исправить автоматически (sudo / Polkit)", "⚡ Fix Automatically (sudo / Polkit)"), gtk.RESPONSE_APPLY)
	if btnApply != nil {
		btnApply.SetCanDefault(true)
		dlg.SetDefault(btnApply)
		if styleCtx, sErr := btnApply.GetStyleContext(); sErr == nil {
			styleCtx.AddClass("suggested-action")
		}
	}

	dlg.ShowAll()

	for {
		resp := dlg.Run()
		if resp != gtk.RESPONSE_APPLY {
			dlg.Destroy()
			return
		}

		fixSuccess := app.applySerialPortPermissions(dlg, username, devPath)
		if fixSuccess {
			dlg.Destroy()
			app.StatusLabel.SetText(i18n.T("Права на COM-порт успешно настроены", "Serial port permissions configured successfully"))
			// Automatically connect now that permissions are granted!
			app.ConnectToHost(host)
			return
		}
	}
}

func (app *AppWindow) applySerialPortPermissions(parent *gtk.Dialog, username, devPath string) bool {
	// 1. Try pkexec (Polkit) if installed
	if pkexecPath, err := exec.LookPath("pkexec"); err == nil && pkexecPath != "" {
		cmdText := fmt.Sprintf("usermod -aG dialout %s && (chmod 666 %s || setfacl -m u:%s:rw %s)",
			username, devPath, username, devPath)
		cmd := exec.Command(pkexecPath, "sh", "-c", cmdText)
		out, runErr := cmd.CombinedOutput()
		if runErr == nil {
			log.Printf("[APP] Serial permissions successfully applied via pkexec for %s (%s)", username, devPath)
			return true
		}
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 126 || exitErr.ExitCode() == 127 {
				log.Printf("[APP] pkexec cancelled or dismissed by user")
				return false
			}
		}
		log.Printf("[APP] pkexec failed: %v, output: %s. Falling back to sudo prompt...", runErr, string(out))
	}

	// 2. Fallback to sudo with native PAC password dialog
	sudoPath, err := exec.LookPath("sudo")
	if err != nil {
		app.ShowError(
			i18n.T("Утилита sudo не найдена", "sudo not found"),
			i18n.T("В системе не найдены ни pkexec, ни sudo для получения прав root.", "Neither pkexec nor sudo were found on the system to gain root privileges."))
		return false
	}

	pwDlg, _ := gtk.DialogNew()
	pwDlg.SetTitle(i18n.T("Требуются права root (sudo)", "Root Privileges Required (sudo)"))
	pwDlg.SetTransientFor(parent)
	pwDlg.SetModal(true)
	pwDlg.SetDefaultSize(380, 140)

	cArea, _ := pwDlg.GetContentArea()
	cArea.SetSpacing(8)
	cArea.SetMarginStart(16)
	cArea.SetMarginEnd(16)
	cArea.SetMarginTop(16)
	cArea.SetMarginBottom(16)

	lbl, _ := gtk.LabelNew(i18n.Tf("Введите пароль пользователя '%s' для настройки прав на COM-порт:",
		"Enter password for '%s' to configure COM port permissions:", username))
	lbl.SetXAlign(0)
	lbl.SetLineWrap(true)
	cArea.Add(lbl)

	pwEntry, _ := gtk.EntryNew()
	pwEntry.SetVisibility(false)
	pwEntry.SetActivatesDefault(true)
	cArea.Add(pwEntry)

	_, _ = pwDlg.AddButton(i18n.T("Отмена", "Cancel"), gtk.RESPONSE_CANCEL)
	okBtn, _ := pwDlg.AddButton(i18n.T("Применить", "Apply"), gtk.RESPONSE_OK)
	if okBtn != nil {
		okBtn.SetCanDefault(true)
		pwDlg.SetDefault(okBtn)
	}

	pwDlg.ShowAll()
	pwResp := pwDlg.Run()
	password, _ := pwEntry.GetText()
	pwDlg.Destroy()

	if pwResp != gtk.RESPONSE_OK || password == "" {
		return false
	}

	cmdText := fmt.Sprintf("usermod -aG dialout %s && (chmod 666 %s || setfacl -m u:%s:rw %s)",
		username, devPath, username, devPath)
	cmd := exec.Command(sudoPath, "-S", "-p", "", "sh", "-c", cmdText)
	cmd.Stdin = strings.NewReader(password + "\n")
	out, sErr := cmd.CombinedOutput()
	if sErr != nil {
		errStr := strings.TrimSpace(string(out))
		if errStr == "" {
			errStr = sErr.Error()
		}
		app.ShowError(
			i18n.T("Ошибка sudo", "sudo Error"),
			i18n.Tf("Не удалось применить права через sudo:\n\n%s", "Failed to apply permissions via sudo:\n\n%s", errStr))
		return false
	}

	log.Printf("[APP] Serial permissions successfully applied via sudo for %s (%s)", username, devPath)
	return true
}

// wireSerialAutoBaud attaches automatic baud rate detection handlers if configured
func (app *AppWindow) wireSerialAutoBaud(sess *session.Session, host *storage.Host, term *vte.Terminal) {
	if sess == nil || sess.SerialSession == nil || host == nil || !host.SerialAutoBaud {
		return
	}
	sess.SerialSession.OnBaudRateDetected = func(detectedBaud, currentBaud int) {
		glib.IdleAdd(func() {
			app.showBaudRateSuggestion(sess, host, term, detectedBaud, currentBaud)
		})
	}
}

// showBaudRateSuggestion displays an InfoBar notification offering to switch serial baud rate
func (app *AppWindow) showBaudRateSuggestion(sess *session.Session, host *storage.Host, term *vte.Terminal, detectedBaud, currentBaud int) {
	if sess == nil || host == nil || term == nil {
		return
	}
	pane := app.TabView.FindPaneBySession(sess)
	if pane == nil || pane.Box == nil {
		return
	}

	if app.activeBaudBars == nil {
		app.activeBaudBars = make(map[string]*gtk.InfoBar)
	}
	if existing, ok := app.activeBaudBars[sess.ID]; ok && existing != nil {
		existing.Destroy()
		delete(app.activeBaudBars, sess.ID)
	}

	infoBar, err := gtk.InfoBarNew()
	if err != nil {
		return
	}
	infoBar.SetMessageType(gtk.MESSAGE_QUESTION)
	infoBar.SetShowCloseButton(true)

	contentArea, err := infoBar.GetContentArea()
	if err == nil && contentArea != nil {
		lbl, _ := gtk.LabelNew("")
		msg := i18n.Tf(
			"Обнаружены искаженные данные на COM-порту (текущая: %d бод). Рекомендуемая скорость: <b>%d бод</b>.",
			"Garbage data detected on serial port (current: %d baud). Recommended baud rate: <b>%d baud</b>.",
			currentBaud, detectedBaud,
		)
		lbl.SetMarkup(msg)
		lbl.SetXAlign(0)
		contentArea.Add(lbl)
	}

	infoBar.AddButton(i18n.Tf("⚡ Переключить на %d бод", "⚡ Switch to %d baud", detectedBaud), gtk.RESPONSE_APPLY)
	infoBar.AddButton(i18n.T("Игнорировать", "Ignore"), gtk.RESPONSE_CANCEL)

	infoBar.Connect("response", func(_ *gtk.InfoBar, resp gtk.ResponseType) {
		if bar, ok := app.activeBaudBars[sess.ID]; ok && bar == infoBar {
			delete(app.activeBaudBars, sess.ID)
		}
		infoBar.Destroy()

		if resp == gtk.RESPONSE_APPLY {
			if err := sess.SetSerialBaudRate(detectedBaud); err != nil {
				app.StatusLabel.SetText(i18n.Tf("Ошибка смены скорости: %v", "Error switching baud rate: %v", err))
				term.FeedText(fmt.Sprintf("\r\n\x1b[1;31m%s\x1b[0m\r\n",
					i18n.Tf("[Ошибка смены скорости: %v]", "[Error switching baud rate: %v]", err)))
				return
			}
			host.SerialBaudRate = detectedBaud
			_ = app.store.SaveHost(host)
			app.StatusLabel.SetText(i18n.Tf("Скорость COM-порта установлена в %d бод", "COM port baud rate set to %d baud", detectedBaud))
			term.FeedText(fmt.Sprintf("\r\n\x1b[1;32m%s\x1b[0m\r\n",
				i18n.Tf("[⚡ Скорость COM-порта успешно переключена на %d бод]", "[⚡ Serial baud rate successfully switched to %d baud]", detectedBaud)))
		}
	})

	app.activeBaudBars[sess.ID] = infoBar
	pane.Box.PackStart(infoBar, false, false, 0)
	pane.Box.ReorderChild(infoBar, 0)
	infoBar.ShowAll()
}
