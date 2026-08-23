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
	leftPaned, _ := gtk.PanedNew(gtk.ORIENTATION_VERTICAL)
	rightPaned, _ := gtk.PanedNew(gtk.ORIENTATION_HORIZONTAL)

	hostTree, _ := NewHostTree(store)
	sftpPanel, _ := NewSFTPPanel(watcherMgr)
	tabView, _ := NewTabView()
	notesPanel, _ := NewNotesPanel(store)
	broadcastBar, _ := NewBroadcastBar(manager)

	leftPaned.Pack1(hostTree.Box, false, false)
	leftPaned.Pack2(sftpPanel.Box, true, false)
	leftPaned.SetPosition(320)

	rightPaned.Pack1(tabView.Notebook, true, false)
	rightPaned.Pack2(notesPanel.Box, false, false)
	rightPaned.SetPosition(750)

	leftRightPaned.Pack1(leftPaned, false, false)
	leftRightPaned.Pack2(rightPaned, true, false)
	leftRightPaned.SetPosition(260)

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
				activeList := app.manager.GetAll()
				if len(activeList) > 0 {
					_ = session.SaveState(app.store, activeList)
				}
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

	// 3. Split Horizontal
	btnSplitH, _ := gtk.ToolButtonNew(nil, "Сплит H")
	btnSplitH.SetIconName("view-paged-symbolic")
	btnSplitH.SetTooltipText("Разделить активную вкладку по горизонтали (верхний и нижний терминалы)")
	btnSplitH.Connect("clicked", func() {
		tab := app.TabView.GetCurrentTab()
		if tab != nil {
			app.handleSplit(tab.Session, false)
		}
	})
	app.ToolBar.Insert(btnSplitH, -1)

	// 4. Split Vertical
	btnSplitV, _ := gtk.ToolButtonNew(nil, "Сплит V")
	btnSplitV.SetIconName("view-dual-symbolic")
	btnSplitV.SetTooltipText("Разделить активную вкладку по вертикали (левый и правый терминалы)")
	btnSplitV.Connect("clicked", func() {
		tab := app.TabView.GetCurrentTab()
		if tab != nil {
			app.handleSplit(tab.Session, true)
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
				_ = session.SaveState(app.store, app.manager.GetAll())
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

	targetHost := sess.Host
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

	go func() {
		newSess, err := session.StartSession(context.Background(), targetHost, tab.Session.Title+" [сплит]", app.settings.DefaultLogsDir)
		glib.IdleAdd(func() {
			if err != nil {
				app.StatusLabel.SetText("Ошибка создания сплита: " + err.Error())
				log.Printf("[APP] ERROR creating split session: %v", err)
				return
			}
			app.manager.Register(newSess)

			term, err := vte.NewTerminal()
			if err != nil {
				log.Printf("[APP] ERROR creating split terminal: %v", err)
				newSess.Close()
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

			// Propagate window resize to PTY and remote SSH
			term.OnResize = func(rows, cols int) {
				newSess.Resize(rows, cols)
			}

			if newSess.PTY != nil && newSess.PTY.Master != nil {
				_ = term.SetPTYFD(int(newSess.PTY.Master.Fd()))
			}

			err = app.TabView.SplitActiveTab(tab, newSess, term, vertical)
			if err != nil {
				log.Printf("[APP] ERROR in SplitActiveTab: %v", err)
			} else {
				app.StatusLabel.SetText("Экран успешно разделен")
				log.Printf("[APP] Split created successfully")
			}

			if app.settings.AutoRestoreSessions {
				_ = session.SaveState(app.store, app.manager.GetAll())
			}
		})
	}()
}

// ConnectToHost opens a new session and attaches it to a new tab
func (app *AppWindow) ConnectToHost(host *storage.Host) {
	log.Printf("[APP] ConnectToHost initiated for: %s (%s:%d, proto=%s)", host.Name, host.Host, host.Port, host.Protocol)
	app.StatusLabel.SetText("Подключение к " + host.Host + "...")

	go func() {
		var jumpClient *cryptoSsh.Client
		// Resolve Jump Host if specified
		if host.ProxyJumpHost != "" {
			log.Printf("[APP] ProxyJump configured: %s", host.ProxyJumpHost)
			if jumpHost, err := app.store.GetHost(host.ProxyJumpHost); err == nil && jumpHost != nil {
				bridge, bErr := pty.Open()
				if bErr == nil {
					if jSess, jErr := engineSSH.ConnectSSH(context.Background(), jumpHost, bridge, nil); jErr == nil {
						jumpClient = jSess.Client()
					}
				}
			}
		}

		sess, err := session.StartSessionWithJump(context.Background(), host, host.Name, app.settings.DefaultLogsDir, jumpClient)
		glib.IdleAdd(func() {
			if err != nil {
				app.StatusLabel.SetText("Ошибка подключения: " + err.Error())
				log.Printf("[APP] ERROR connecting to host %s: %v", host.Name, err)
				return
			}

			app.manager.Register(sess)

			term, err := vte.NewTerminal()
			if err != nil {
				app.StatusLabel.SetText("Ошибка создания VTE виджета: " + err.Error())
				log.Printf("[APP] ERROR creating VTE terminal: %v", err)
				sess.Close()
				return
			}

			// Apply terminal settings
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

			// Propagate window resize to PTY and remote SSH
			term.OnResize = func(rows, cols int) {
				sess.Resize(rows, cols)
			}

			// Attach PTY FD to VTE widget
			if sess.PTY != nil && sess.PTY.Master != nil {
				_ = term.SetPTYFD(int(sess.PTY.Master.Fd()))
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
				_ = session.SaveState(app.store, app.manager.GetAll())
			}
		})
	}()
}

// RestoreSavedSessions restores tabs and scrollback history from database
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

	restoredCount := 0
	totalCount := len(savedSessions)

	for _, st := range savedSessions {
		var h *storage.Host
		if st.HostID != "" {
			h, _ = app.store.GetHost(st.HostID)
		}
		if h == nil {
			log.Printf("[RESTORE] HostID %q not found in DB, using fallback profile for %q", st.HostID, st.Title)
			h = &storage.Host{
				ID:             st.HostID,
				Name:           st.Title,
				Protocol:       storage.ProtoLocal,
				TerminalType:   "xterm-256color",
				RestoreHistory: true,
			}
		}

		go func(savedSt storage.SavedSessionState, host *storage.Host) {
			log.Printf("[RESTORE] Starting restored session for: %s (HostID=%s, proto=%s)", savedSt.Title, host.ID, host.Protocol)
			sess, err := session.StartSession(context.Background(), host, savedSt.Title, app.settings.DefaultLogsDir)
			glib.IdleAdd(func() {
				restoredCount++
				if restoredCount >= totalCount {
					app.restoreMu.Lock()
					app.isRestoring = false
					app.restoreMu.Unlock()
					log.Printf("[RESTORE] All %d sessions processed. isRestoring flag cleared.", totalCount)
				}

				if err != nil {
					log.Printf("[RESTORE] ERROR starting session for %s: %v", savedSt.Title, err)
					return
				}
				sess.Notes = savedSt.Notes
				app.manager.Register(sess)

				term, err := vte.NewTerminal()
				if err != nil {
					log.Printf("[RESTORE] ERROR creating VTE for %s: %v", savedSt.Title, err)
					sess.Close()
					return
				}

				if host.FontName != "" {
					term.SetFont(host.FontName)
				}
				if host.ColorScheme != "" {
					term.ApplyColorScheme(host.ColorScheme)
				}

				// Propagate window resize to PTY and remote SSH
				term.OnResize = func(rows, cols int) {
					sess.Resize(rows, cols)
				}

				// Restore history dump into VTE
				if savedSt.ScrollbackDump != "" {
					header := session.FormatRestoredHistoryHeader(savedSt.SavedAt)
					term.FeedText(savedSt.ScrollbackDump + header)
					log.Printf("[RESTORE] Fed %d bytes of scrollback history into VTE for %s", len(savedSt.ScrollbackDump), savedSt.Title)
				}

				// Attach PTY
				if sess.PTY != nil && sess.PTY.Master != nil {
					_ = term.SetPTYFD(int(sess.PTY.Master.Fd()))
				}

				_, _ = app.TabView.AddTab(sess, term)
				app.NotesPanel.LoadSessionNotes(sess)

				if sess.SFTPClient != nil {
					app.SFTPPanel.AttachClient(host.ID, sess.SFTPClient, app.settings.DefaultEditor)
				}
				log.Printf("[RESTORE] Restored tab successfully added for: %s", savedSt.Title)
			})
		}(st, h)
	}
}

// Quit saves session states and exits cleanly
func (app *AppWindow) Quit() {
	log.Printf("[APP] Application quit requested. Active sessions: %d", len(app.manager.GetAll()))
	if app.settings.AutoRestoreSessions {
		_ = session.SaveState(app.store, app.manager.GetAll())
	}
	app.manager.CloseAll()
	_ = app.watcherMgr.Close()
	_ = app.store.Close()
	log.Printf("[APP] Exiting GTK main loop.")
	gtk.MainQuit()
	os.Exit(0)
}
