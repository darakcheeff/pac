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
	"github.com/darakcheeff/pac/internal/migration"
	"github.com/darakcheeff/pac/internal/session"
	"github.com/darakcheeff/pac/internal/storage"
	"github.com/darakcheeff/pac/internal/ui/dialogs"
	"github.com/darakcheeff/pac/internal/ui/vte"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	cryptoSsh "golang.org/x/crypto/ssh"
)

// AppWindow represents the primary application window
type AppWindow struct {
	Window       *gtk.Window
	MainBox      *gtk.Box
	MenuBar      *gtk.MenuBar
	ToolBar      *gtk.Toolbar
	MainPaned    *gtk.Paned
	LeftPaned    *gtk.Paned
	RightPaned   *gtk.Paned
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

func NewAppWindow(store *storage.Store) (*AppWindow, error) {
	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		return nil, err
	}
	win.SetTitle("PAC Connection Manager NextGen")
	win.SetDefaultSize(1200, 750)
	win.SetPosition(gtk.WIN_POS_CENTER)

	watcherMgr, _ := watcher.NewRemoteEditManager()
	manager := session.NewManager(store)
	settings, _ := store.GetSettings()

	mainBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	win.Add(mainBox)

	// 1. MenuBar
	menuBar, _ := gtk.MenuBarNew()
	mainBox.PackStart(menuBar, false, false, 0)

	// 2. ToolBar
	toolBar, _ := gtk.ToolbarNew()
	toolBar.SetStyle(gtk.TOOLBAR_BOTH_HORIZ)
	mainBox.PackStart(toolBar, false, false, 0)

	// 3. Center Workspace Paneds
	leftRightPaned, _ := gtk.PanedNew(gtk.ORIENTATION_HORIZONTAL)
	leftRightPaned.SetWideHandle(true)

	leftPaned, _ := gtk.PanedNew(gtk.ORIENTATION_VERTICAL)
	leftPaned.SetWideHandle(true)

	rightPaned, _ := gtk.PanedNew(gtk.ORIENTATION_HORIZONTAL)
	rightPaned.SetWideHandle(true)

	hostTree, _ := NewHostTree(store)
	sftpPanel, _ := NewSFTPPanel(watcherMgr)
	tabView, _ := NewTabView()
	notesPanel, _ := NewNotesPanel(store)
	broadcastBar, _ := NewBroadcastBar(manager)

	leftPaned.Pack1(hostTree.Box, true, true)
	leftPaned.Pack2(sftpPanel.Box, true, true)
	leftPaned.SetPosition(320)

	rightPaned.Pack1(tabView.Notebook, true, true)
	rightPaned.Pack2(notesPanel.Box, true, true)
	rightPaned.SetPosition(800)

	leftRightPaned.Pack1(leftPaned, true, true)
	leftRightPaned.Pack2(rightPaned, true, true)
	leftRightPaned.SetPosition(280)

	mainBox.PackStart(leftRightPaned, true, true, 0)

	// 4. Bottom Broadcast Bar (Hidden by default, won't show with ShowAll)
	broadcastBar.Box.SetNoShowAll(true)
	mainBox.PackStart(broadcastBar.Box, false, false, 0)
	broadcastBar.Box.Hide()

	// 5. StatusBar
	statusBar, _ := gtk.StatusbarNew()
	statusLabel, _ := gtk.LabelNew("Готово к работе")
	statusLabel.SetMarginStart(8)
	statusBar.PackStart(statusLabel, false, false, 0)
	mainBox.PackEnd(statusBar, false, false, 0)

	app := &AppWindow{
		Window:       win,
		MainBox:      mainBox,
		MenuBar:      menuBar,
		ToolBar:      toolBar,
		MainPaned:    leftRightPaned,
		LeftPaned:    leftPaned,
		RightPaned:   rightPaned,
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
				app.StatusLabel.SetText(fmt.Sprintf("Импортировано %d хостов из старого Ásbrú", n))
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
	// --- Menus ---
	// File Menu
	mFile, _ := gtk.MenuItemNewWithMnemonic("_Файл")
	fileMenu, _ := gtk.MenuNew()
	mFile.SetSubmenu(fileMenu)

	mNewHost, _ := gtk.MenuItemNewWithLabel("Новое подключение...")
	mNewHost.Connect("activate", func() {
		dialogs.ShowHostEditorDialog(app.Window, app.store, nil, "root", func(h *storage.Host) {
			app.HostTree.Reload()
		})
	})
	fileMenu.Append(mNewHost)

	mQuickConnect, _ := gtk.MenuItemNewWithLabel("Быстрое подключение...")
	mQuickConnect.Connect("activate", func() {
		dialogs.ShowQuickConnectDialog(app.Window, func(h *storage.Host) {
			app.ConnectToHost(h)
		})
	})
	fileMenu.Append(mQuickConnect)

	mImportOld, _ := gtk.MenuItemNewWithLabel("Импорт из Ásbrú v6 (asbru.conf)...")
	mImportOld.Connect("activate", func() {
		n, err := migration.MigrateOldConfig(app.store, "")
		if err == nil {
			app.HostTree.Reload()
			app.StatusLabel.SetText(fmt.Sprintf("Успешно импортировано %d сессий", n))
		}
	})
	fileMenu.Append(mImportOld)

	sep1, _ := gtk.SeparatorMenuItemNew()
	fileMenu.Append(sep1)

	mQuit, _ := gtk.MenuItemNewWithLabel("Выход")
	mQuit.Connect("activate", func() {
		app.Quit()
	})
	fileMenu.Append(mQuit)
	app.MenuBar.Append(mFile)

	// Edit Menu
	mEdit, _ := gtk.MenuItemNewWithMnemonic("_Правка")
	editMenu, _ := gtk.MenuNew()
	mEdit.SetSubmenu(editMenu)

	mGlobalSearch, _ := gtk.MenuItemNewWithLabel("Глобальный поиск по всем сессиям (Ctrl+Shift+Alt+F)")
	mGlobalSearch.Connect("activate", func() {
		ShowGlobalSearchDialog(app.Window, app.manager, func(sessionID string) {
			// Select tab
		})
	})
	editMenu.Append(mGlobalSearch)
	app.MenuBar.Append(mEdit)

	// View Menu
	mView, _ := gtk.MenuItemNewWithMnemonic("_Вид")
	viewMenu, _ := gtk.MenuNew()
	mView.SetSubmenu(viewMenu)

	mToggleNotes, _ := gtk.CheckMenuItemNewWithLabel("Панель заметок")
	mToggleNotes.SetActive(true)
	mToggleNotes.Connect("toggled", func() {
		if mToggleNotes.GetActive() {
			app.NotesPanel.Box.Show()
		} else {
			app.NotesPanel.Box.Hide()
		}
	})
	viewMenu.Append(mToggleNotes)

	mToggleSFTP, _ := gtk.CheckMenuItemNewWithLabel("SFTP файловый менеджер")
	mToggleSFTP.SetActive(true)
	mToggleSFTP.Connect("toggled", func() {
		if mToggleSFTP.GetActive() {
			app.SFTPPanel.Box.Show()
		} else {
			app.SFTPPanel.Box.Hide()
		}
	})
	viewMenu.Append(mToggleSFTP)

	mToggleBroadcast, _ := gtk.CheckMenuItemNewWithLabel("Панель кластерного ввода")
	mToggleBroadcast.Connect("toggled", func() {
		if mToggleBroadcast.GetActive() {
			app.BroadcastBar.Box.Show()
			app.BroadcastBar.Entry.GrabFocus()
		} else {
			app.BroadcastBar.Box.Hide()
		}
	})
	viewMenu.Append(mToggleBroadcast)

	app.MenuBar.Append(mView)

	// Sessions Menu
	mSessions, _ := gtk.MenuItemNewWithMnemonic("_Сессии")
	sessMenu, _ := gtk.MenuNew()
	mSessions.SetSubmenu(sessMenu)

	mSplitHoriz, _ := gtk.MenuItemNewWithLabel("Разделить экран по горизонтали")
	mSplitHoriz.Connect("activate", func() {
		tab := app.TabView.GetCurrentTab()
		if tab != nil {
			app.handleSplit(tab.Session, false)
		}
	})
	sessMenu.Append(mSplitHoriz)

	mSplitVert, _ := gtk.MenuItemNewWithLabel("Разделить экран по вертикали")
	mSplitVert.Connect("activate", func() {
		tab := app.TabView.GetCurrentTab()
		if tab != nil {
			app.handleSplit(tab.Session, true)
		}
	})
	sessMenu.Append(mSplitVert)

	mUnsplit, _ := gtk.MenuItemNewWithLabel("Разгруппировать сплит в новую вкладку")
	mUnsplit.Connect("activate", func() {
		tab := app.TabView.GetCurrentTab()
		if tab != nil {
			app.TabView.UnsplitTab(tab)
		}
	})
	sessMenu.Append(mUnsplit)

	app.MenuBar.Append(mSessions)

	// --- ToolBar Buttons with standard icons & rich tooltips ---
	// 1. New Connection
	btnNew, _ := gtk.ToolButtonNew(nil, "Новое подключение")
	btnNew.SetIconName("tab-new-symbolic")
	btnNew.SetTooltipText("Создать новое подключение к серверу (SSH, Telnet, Serial, Local)")
	btnNew.Connect("clicked", func() {
		dialogs.ShowHostEditorDialog(app.Window, app.store, nil, "root", func(h *storage.Host) {
			app.HostTree.Reload()
		})
	})
	app.ToolBar.Insert(btnNew, -1)

	// 2. Quick Connect
	btnQuick, _ := gtk.ToolButtonNew(nil, "Быстрое подключение")
	btnQuick.SetIconName("network-wired-symbolic")
	btnQuick.SetTooltipText("Быстрое подключение к хосту без предварительного сохранения")
	btnQuick.Connect("clicked", func() {
		dialogs.ShowQuickConnectDialog(app.Window, func(h *storage.Host) {
			app.ConnectToHost(h)
		})
	})
	app.ToolBar.Insert(btnQuick, -1)

	sepTool1, _ := gtk.SeparatorToolItemNew()
	app.ToolBar.Insert(sepTool1, -1)

	// 3. Split Horizontal (Top / Bottom)
	btnSplitH, _ := gtk.ToolButtonNew(GetSplitHorizontalImage(), "Разделить горизонтально")
	btnSplitH.SetTooltipText("Разделить экран по горизонтали (сверху и снизу)")
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
	btnSplitV, _ := gtk.ToolButtonNew(GetSplitVerticalImage(), "Разделить вертикально")
	btnSplitV.SetTooltipText("Разделить экран по вертикали (слева и справа)")
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

	sepTool2, _ := gtk.SeparatorToolItemNew()
	app.ToolBar.Insert(sepTool2, -1)

	// 5. Broadcast / Cluster Input
	btnBroadcast, _ := gtk.ToolButtonNew(nil, "Кластерный ввод")
	btnBroadcast.SetIconName("input-keyboard-symbolic")
	btnBroadcast.SetTooltipText("Кластерный ввод: одновременная трансляция команд во все открытые вкладки")
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
	btnSearch, _ := gtk.ToolButtonNew(nil, "Поиск по всем сессиям")
	btnSearch.SetIconName("edit-find-symbolic")
	btnSearch.SetTooltipText("Глобальный поиск текста по всем открытым сессиям и вкладкам (Ctrl+Shift+Alt+F)")
	btnSearch.Connect("clicked", func() {
		ShowGlobalSearchDialog(app.Window, app.manager, nil)
	})
	app.ToolBar.Insert(btnSearch, -1)
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
			Name:     "Новая папка",
			Icon:     "folder",
		}
		_ = app.store.SaveGroup(g)
		app.HostTree.Reload()
	}

	app.TabView.OnTabChanged = func(sess *session.Session) {
		if sess != nil {
			app.NotesPanel.LoadSessionNotes(sess)
			if sess.SFTPClient != nil && sess.Host != nil {
				app.SFTPPanel.AttachClient(sess.Host.ID, sess.SFTPClient, app.settings.DefaultEditor)
			}
			if sess.Host != nil {
				app.StatusLabel.SetText(fmt.Sprintf("Сессия: %s (%s) | Протокол: %s", sess.Title, sess.Host.Host, sess.Host.Protocol))
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
	app.StatusLabel.SetText("Разделение экрана...")

	term, err := vte.NewTerminal()
	if err != nil {
		app.StatusLabel.SetText("Ошибка создания VTE виджета: " + err.Error())
		return
	}
	slaveFile, err := term.SetupNativePTY()
	if err != nil {
		app.StatusLabel.SetText("Ошибка инициализации PTY: " + err.Error())
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
		newSess, err := session.StartSessionWithBridge(context.Background(), targetHost, tab.Session.Title+" [сплит]", app.settings.DefaultLogsDir, bridge, nil)
		glib.IdleAdd(func() {
			if err != nil {
				app.StatusLabel.SetText("Ошибка создания сплита: " + err.Error())
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
				app.StatusLabel.SetText("Экран успешно разделен")
				log.Printf("[APP] Split created successfully")
			}

			if app.settings.AutoRestoreSessions {
				app.SaveAllSessionState()
			}
		})
	}()
}

// ConnectToHost opens a new session and attaches it to a new tab
func (app *AppWindow) ConnectToHost(host *storage.Host) {
	log.Printf("[APP] ConnectToHost initiated for: %s (%s:%d, proto=%s)", host.Name, host.Host, host.Port, host.Protocol)
	app.StatusLabel.SetText("Подключение к " + host.Host + "...")

	term, err := vte.NewTerminal()
	if err != nil {
		app.StatusLabel.SetText("Ошибка создания VTE виджета: " + err.Error())
		log.Printf("[APP] ERROR creating VTE terminal: %v", err)
		return
	}

	slaveFile, err := term.SetupNativePTY()
	if err != nil {
		app.StatusLabel.SetText("Ошибка инициализации PTY: " + err.Error())
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
		// Resolve Jump Host if specified
		if host.ProxyJumpHost != "" {
			log.Printf("[APP] ProxyJump configured: %s", host.ProxyJumpHost)
			if jumpHost, err := app.store.GetHost(host.ProxyJumpHost); err == nil && jumpHost != nil {
				jBridge, bErr := pty.Open()
				if bErr == nil {
					if jSess, jErr := engineSSH.ConnectSSH(context.Background(), jumpHost, jBridge, nil); jErr == nil {
						jumpClient = jSess.Client()
					}
				}
			}
		}

		sess, err := session.StartSessionWithBridge(context.Background(), host, host.Name, app.settings.DefaultLogsDir, bridge, jumpClient)
		glib.IdleAdd(func() {
			if err != nil {
				app.StatusLabel.SetText("Ошибка подключения: " + err.Error())
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

			app.StatusLabel.SetText(fmt.Sprintf("Подключено: %s (%s)", host.Name, host.Host))
			log.Printf("[APP] Successfully connected and opened tab for: %s (sessionID=%s)", host.Name, sess.ID)

			if app.settings.AutoRestoreSessions {
				app.SaveAllSessionState()
			}
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
	app.StatusLabel.SetText(fmt.Sprintf("Восстановление %d сессий...", len(savedSessions)))

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
				Notes:          item.Session.Notes,
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
