package ui

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/darakcheeff/pac/internal/engine/pty"
	engineSSH "github.com/darakcheeff/pac/internal/engine/ssh"
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
	isRestoring bool
	restoreMu   sync.Mutex
}


const compactCSS = `
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
	min-width: 3px;
	min-height: 3px;
	background-color: rgba(0, 0, 0, 0.12);
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

func applyCompactTheme() {
	cssProvider, err := gtk.CssProviderNew()
	if err == nil {
		_ = cssProvider.LoadFromData(compactCSS)
		screen, err := gdk.ScreenGetDefault()
		if err == nil && screen != nil {
			gtk.AddProviderForScreen(screen, cssProvider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
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
	applyCompactTheme()

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
	leftPaned.Pack1(hostTree.Box, true, true)
	leftPaned.Pack2(sftpPanel.Box, true, true)
	leftPaned.SetPosition(250)

	// Center horizontal split: Left Sidebar + Center Terminals
	// leftPaned doesn't expand on window resize; tabView takes all extra width
	centerPaned.Pack1(leftPaned, false, true)
	centerPaned.Pack2(tabView.Notebook, true, true)
	centerPaned.SetPosition(240)

	// Main horizontal split: (Left Sidebar + Center Terminals) + Notes Panel (Right)
	// centerPaned takes all window resize; notesPanel stays docked on right
	mainPaned.Pack1(centerPaned, true, true)
	mainPaned.Pack2(notesPanel.Box, false, true)
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
	}

	app.setupMenuAndToolbar()
	app.setupSignals()

	// Periodic auto-save of active session state (every 5 seconds with diff checking)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			app.restoreMu.Lock()
			restoring := app.isRestoring
			app.restoreMu.Unlock()

			if !restoring && app.settings.AutoRestoreSessions {
				app.SaveAllSessionState()
			}
		}
	}()

	// Background initialization: Migrate legacy config (if any) and restore saved sessions
	go func() {
		n, err := migration.MigrateOldConfig(store, "")
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("[MIGRATE] Error checking legacy config: %v", err)
			}
			if n > 0 {
				app.HostTree.Reload()
				app.StatusLabel.SetText(i18n.Tf("Импортировано %d хостов из старого Ásbrú", "Imported %d hosts from old Ásbrú", n))
			}

			// Restore saved sessions
			if app.settings.AutoRestoreSessions {
				app.RestoreSavedSessions()
			} else {
				app.restoreMu.Lock()
				app.isRestoring = false
				app.restoreMu.Unlock()
			}
		})
	}()

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
		g := &storage.Group{
			ID:       fmt.Sprintf("grp-%d", time.Now().UnixNano()),
			ParentID: parentGroupID,
			Name:     i18n.T("Новая папка", "New Folder"),
			Icon:     "folder",
		}
		_ = app.store.SaveGroup(g)
		app.HostTree.Reload()
	}

	app.HostTree.OnImportOld = func() {
		n, err := migration.MigrateOldConfig(app.store, "")
		if err == nil {
			app.HostTree.Reload()
			app.StatusLabel.SetText(i18n.Tf("Успешно импортировано %d сессий", "Successfully imported %d sessions", n))
		}
	}

	app.TabView.OnTabChanged = func(sess *session.Session) {
		if sess != nil {
			app.NotesPanel.LoadSessionNotes(sess)
			if sess.SFTPClient != nil && sess.Host != nil {
				app.SFTPPanel.AttachClient(sess.Host.ID, sess.SFTPClient, app.settings.DefaultEditor)
			}
			if sess.Host != nil {
				app.StatusLabel.SetText(i18n.Tf("Сессия: %s (%s) | Протокол: %s", "Session: %s (%s) | Protocol: %s", sess.Title, sess.Host.Host, sess.Host.Protocol))
			}
		}
	}

	app.TabView.OnTabClosed = func(sess *session.Session) {
		if sess != nil {
			log.Printf("[APP] Tab closed: %s (ID=%s)", sess.Title, sess.ID)
			app.manager.Unregister(sess.ID)
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
			_ = os.WriteFile(filename, []byte(sess.GetScrollbackText()), 0644)
			app.StatusLabel.SetText(i18n.T("Журнал сохранен: ", "Log saved: ") + filename)
		}
	}

	app.TabView.OnNewConnection = func() {
		dialogs.ShowQuickConnectDialog(app.Window, func(h *storage.Host) {
			app.ConnectToHost(h)
		})
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
		newSess, err := session.StartSessionWithBridge(context.Background(), targetHost, tab.Session.Title+i18n.T(" [сплит]", " [split]"), app.settings.DefaultLogsDir, bridge, nil)
		glib.IdleAdd(func() {
			if err != nil {
				app.StatusLabel.SetText(i18n.T("Ошибка создания сплита: ", "Error creating split: ") + err.Error())
				log.Printf("[APP] ERROR creating split session: %v", err)
				return
			}
			app.manager.Register(newSess)

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

			app.attachSessionExitHandler(newSess, term, targetHost, tab.Session.Title+i18n.T(" [сплит]", " [split]"))
		})
	}()
}

// ConnectToHost opens a new session and attaches it to a new tab
func (app *AppWindow) ConnectToHost(host *storage.Host) {
	log.Printf("[APP] ConnectToHost initiated for: %s (%s:%d, proto=%s)", host.Name, host.Host, host.Port, host.Protocol)
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
				app.StatusLabel.SetText(i18n.T("Ошибка подключения: ", "Connection error: ") + err.Error())
				log.Printf("[APP] ERROR connecting to host %s: %v", host.Name, err)
				return
			}

			app.manager.Register(sess)

			// Propagate window resize to PTY and remote SSH
			term.OnResize = func(rows, cols int) {
				sess.Resize(rows, cols)
			}

			// Add to notebook tab
			_, _ = app.TabView.AddTab(sess, term)
			app.NotesPanel.LoadSessionNotes(sess)

			if sess.SFTPClient != nil {
				app.SFTPPanel.AttachClient(host.ID, sess.SFTPClient, app.settings.DefaultEditor)
			}

			app.StatusLabel.SetText(i18n.Tf("Подключено: %s (%s)", "Connected: %s (%s)", host.Name, host.Host))
			log.Printf("[APP] Successfully connected and opened tab for: %s (sessionID=%s)", host.Name, sess.ID)

			if app.settings.AutoRestoreSessions {
				app.SaveAllSessionState()
			}

			app.attachSessionExitHandler(sess, term, host, host.Name)
		})
	}()
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

	totalSessions := len(savedSessions)
	restoredCount := 0

	for _, tIdx := range tabIndices {
		grp := groups[tIdx]
		st := grp.primary
		splits := grp.splits

		var h *storage.Host
		if st.HostID != "" {
			h, _ = app.store.GetHost(st.HostID)
		}
		if h == nil {
			h = &storage.Host{
				ID:             st.HostID,
				Name:           st.Title,
				Protocol:       st.Protocol,
				TerminalType:   "xterm-256color",
				RestoreHistory: true,
			}
		}

		term, err := vte.NewTerminal()
		if err != nil {
			continue
		}
		slaveFile, err := term.SetupNativePTY()
		if err != nil {
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
			sess, err := session.StartSessionWithBridge(context.Background(), hostCopy, savedState.Title, app.settings.DefaultLogsDir, bridge, nil)
			glib.IdleAdd(func() {
				if err != nil {
					log.Printf("[RESTORE] ERROR starting session for %s: %v", savedState.Title, err)
					return
				}
				sess.ID = savedState.ID
				sess.Notes = savedState.Notes
				app.manager.Register(sess)

				term.OnResize = func(rows, cols int) {
					sess.Resize(rows, cols)
				}
				if savedState.ScrollbackDump != "" {
					header := session.FormatRestoredHistoryHeader(savedState.SavedAt)
					term.FeedText(savedState.ScrollbackDump + header)
				}

				tabItem, _ := app.TabView.AddTab(sess, term)
				app.NotesPanel.LoadSessionNotes(sess)
				if sess.SFTPClient != nil {
					app.SFTPPanel.AttachClient(hostCopy.ID, sess.SFTPClient, app.settings.DefaultEditor)
				}

				app.attachSessionExitHandler(sess, term, hostCopy, savedState.Title)

				restoredCount++
				if restoredCount >= totalSessions {
					app.restoreMu.Lock()
					app.isRestoring = false
					app.restoreMu.Unlock()
					log.Printf("[RESTORE] All %d sessions restored. isRestoring cleared.", restoredCount)
				}

				// Restore split children into this tabItem
				for _, chState := range splitStates {
					app.restoreSplitPane(tabItem, chState, func() {
						restoredCount++
						if restoredCount >= totalSessions {
							app.restoreMu.Lock()
							app.isRestoring = false
							app.restoreMu.Unlock()
							log.Printf("[RESTORE] All %d sessions restored. isRestoring cleared.", restoredCount)
						}
					})
				}
			})
		}()
	}
}

func (app *AppWindow) restoreSplitPane(tabItem *TabItem, st storage.SavedSessionState, onDone func()) {
	var h *storage.Host
	if st.HostID != "" {
		h, _ = app.store.GetHost(st.HostID)
	}
	if h == nil {
		h = &storage.Host{
			ID:             st.HostID,
			Name:           st.Title,
			Protocol:       st.Protocol,
			TerminalType:   "xterm-256color",
			RestoreHistory: true,
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
				log.Printf("[RESTORE] ERROR restoring split session %s: %v", st.Title, err)
				return
			}
			sess.ID = st.ID
			sess.Notes = st.Notes
			app.manager.Register(sess)

			term.OnResize = func(rows, cols int) {
				sess.Resize(rows, cols)
			}
			if st.ScrollbackDump != "" {
				header := session.FormatRestoredHistoryHeader(st.SavedAt)
				term.FeedText(st.ScrollbackDump + header)
			}

			isVertical := st.SplitDirection == "vertical" || st.SplitDirection == "left-right"
			_ = app.TabView.SplitActiveTab(tabItem, sess, term, isVertical)
			app.attachSessionExitHandler(sess, term, h, st.Title)
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

	var states []storage.SavedSessionState
	for tabIdx, item := range app.TabView.items {
		for paneIdx, pane := range item.Panes {
			s := pane.Session
			if s == nil {
				continue
			}
			hostID := ""
			protocol := storage.ProtoLocal
			if s.Host != nil {
				hostID = s.Host.ID
				protocol = s.Host.Protocol
			}
			scrollback := s.GetScrollbackText()
			if len(scrollback) > 50*1024 {
				scrollback = scrollback[len(scrollback)-50*1024:]
			}
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
				SplitParentID:  parentID,
				SplitDirection: splitDir,
				WorkingDir:     workingDir,
				ScrollbackDump: scrollback,
				Notes:          s.Notes,
				SavedAt:        time.Now(),
			}
			states = append(states, st)
		}
	}
	log.Printf("[STATE] Saving %d active pane(s) across %d tab(s) to SQLite...", len(states), len(app.TabView.items))
	_ = app.store.SaveActiveSessions(states)
}

// Quit saves session states and exits cleanly
func (app *AppWindow) Quit() {
	log.Printf("[APP] Application quit requested. Active sessions: %d", len(app.manager.GetAll()))
	if app.settings.AutoRestoreSessions {
		app.SaveAllSessionState()
	}
	app.manager.CloseAll()
	_ = app.watcherMgr.Close()
	_ = app.store.Close()
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
	jBridge, bErr := pty.Open()
	if bErr != nil {
		log.Printf("[APP] resolveJumpClient: failed to open PTY for jump host: %v", bErr)
		return nil
	}
	jSess, jErr := engineSSH.ConnectSSH(context.Background(), jumpHost, jBridge, nil)
	if jErr != nil {
		log.Printf("[APP] resolveJumpClient: failed to connect to jump host %s: %v", host.ProxyJumpHost, jErr)
		return nil
	}
	return jSess.Client()
}

// attachSessionExitHandler monitors session termination and prompts user to reconnect via Enter
func (app *AppWindow) attachSessionExitHandler(sess *session.Session, term *vte.Terminal, host *storage.Host, tabTitle string) {
	if sess == nil || term == nil || host == nil {
		return
	}

	sess.OnExit = func(exitErr error) {
		glib.IdleAdd(func() {
			if term.IsDisconnected() {
				return
			}
			// Verify tab still exists
			if app.TabView.FindTabBySession(sess) == nil {
				return
			}

			// Show yellow disconnect prompt in terminal buffer
			term.FeedText("\r\n\r\n\x1b[1;33m" + i18n.T("[Сессия закрыта. Переподключить? (Нажмите Enter)]", "[Session closed. Reconnect? (Press Enter)]") + "\x1b[0m\r\n")
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

						oldID := sess.ID
						app.manager.Unregister(oldID)
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
							app.SFTPPanel.AttachClient(host.ID, newSess.SFTPClient, app.settings.DefaultEditor)
						}

						app.StatusLabel.SetText(i18n.Tf("Подключено: %s (%s)", "Connected: %s (%s)", host.Name, host.Host))
						log.Printf("[APP] Reconnected session for: %s (new sessionID=%s)", host.Name, newSess.ID)

						if app.settings.AutoRestoreSessions {
							app.SaveAllSessionState()
						}

						app.attachSessionExitHandler(newSess, term, host, tabTitle)
					})
				}()
			}

			term.SetDisconnected(true, reconnectFunc)
		})
	}
}
