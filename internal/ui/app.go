package ui

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/darakcheeff/pac/internal/engine/watcher"
	"github.com/darakcheeff/pac/internal/migration"
	"github.com/darakcheeff/pac/internal/session"
	"github.com/darakcheeff/pac/internal/storage"
	"github.com/darakcheeff/pac/internal/ui/dialogs"
	"github.com/darakcheeff/pac/internal/ui/vte"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
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

	store      *storage.Store
	manager    *session.Manager
	watcherMgr *watcher.RemoteEditManager
	settings   *storage.AppSettings
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

	// 4. Bottom Broadcast Bar
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
	}

	app.setupMenuAndToolbar()
	app.setupSignals()

	// Migrate legacy config if found and db is fresh
	go func() {
		n, _ := migration.MigrateOldConfig(store, "")
		if n > 0 {
			glib.IdleAdd(func() {
				app.HostTree.Reload()
				app.StatusLabel.SetText(fmt.Sprintf("Импортировано %d хостов из старого Ásbrú", n))
			})
		}
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

	// --- ToolBar Buttons ---
	btnNew, _ := gtk.ToolButtonNew(nil, "Новое подключение")
	btnNew.SetIconName("document-new-symbolic")
	btnNew.Connect("clicked", func() {
		dialogs.ShowHostEditorDialog(app.Window, app.store, nil, "root", func(h *storage.Host) {
			app.HostTree.Reload()
		})
	})
	app.ToolBar.Insert(btnNew, -1)

	btnQuick, _ := gtk.ToolButtonNew(nil, "Быстрое подключение")
	btnQuick.SetIconName("network-wired-symbolic")
	btnQuick.Connect("clicked", func() {
		dialogs.ShowQuickConnectDialog(app.Window, func(h *storage.Host) {
			app.ConnectToHost(h)
		})
	})
	app.ToolBar.Insert(btnQuick, -1)

	sepTool1, _ := gtk.SeparatorToolItemNew()
	app.ToolBar.Insert(sepTool1, -1)

	btnBroadcast, _ := gtk.ToolButtonNew(nil, "Кластерный ввод")
	btnBroadcast.SetIconName("input-keyboard-symbolic")
	btnBroadcast.Connect("clicked", func() {
		if app.BroadcastBar.Box.IsVisible() {
			app.BroadcastBar.Box.Hide()
		} else {
			app.BroadcastBar.Box.Show()
			app.BroadcastBar.Entry.GrabFocus()
		}
	})
	app.ToolBar.Insert(btnBroadcast, -1)

	btnSearch, _ := gtk.ToolButtonNew(nil, "Поиск по всем сессиям")
	btnSearch.SetIconName("edit-find-symbolic")
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
		if sess != nil && sess.Host != nil {
			app.NotesPanel.LoadHostNotes(sess.Host)
			if sess.SFTPClient != nil {
				app.SFTPPanel.AttachClient(sess.Host.ID, sess.SFTPClient, app.settings.DefaultEditor)
			}
			app.StatusLabel.SetText(fmt.Sprintf("Сессия: %s (%s) | Протокол: %s", sess.Title, sess.Host.Host, sess.Host.Protocol))
		}
	}

	app.TabView.OnTabClosed = func(sess *session.Session) {
		if sess != nil {
			app.manager.Unregister(sess.ID)
		}
	}

	app.Window.Connect("destroy", func() {
		app.Quit()
	})
}

// ConnectToHost opens a new session and attaches it to a new tab
func (app *AppWindow) ConnectToHost(host *storage.Host) {
	app.StatusLabel.SetText("Подключение к " + host.Host + "...")

	go func() {
		sess, err := session.StartSession(context.Background(), host, host.Name, app.settings.DefaultLogsDir)
		glib.IdleAdd(func() {
			if err != nil {
				app.StatusLabel.SetText("Ошибка подключения: " + err.Error())
				return
			}

			app.manager.Register(sess)

			term, err := vte.NewTerminal()
			if err != nil {
				app.StatusLabel.SetText("Ошибка создания VTE виджета: " + err.Error())
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

			// Attach PTY FD to VTE widget
			if sess.PTY != nil && sess.PTY.Master != nil {
				_ = term.SetPTYFD(int(sess.PTY.Master.Fd()))
			}

			// Add to notebook tab
			_, _ = app.TabView.AddTab(sess, term)
			app.NotesPanel.LoadHostNotes(host)

			if sess.SFTPClient != nil {
				app.SFTPPanel.AttachClient(host.ID, sess.SFTPClient, app.settings.DefaultEditor)
			}

			app.StatusLabel.SetText(fmt.Sprintf("Подключено: %s (%s)", host.Name, host.Host))
		})
	}()
}

// Quit saves session states and exits cleanly
func (app *AppWindow) Quit() {
	if app.settings.AutoRestoreSessions {
		_ = session.SaveState(app.store, app.manager.GetAll())
	}
	app.manager.CloseAll()
	_ = app.watcherMgr.Close()
	_ = app.store.Close()
	gtk.MainQuit()
	os.Exit(0)
}
