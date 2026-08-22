package ui

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/darakcheeff/pac/internal/engine/sftp"
	"github.com/darakcheeff/pac/internal/engine/watcher"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

const (
	SFTPColName  = 0
	SFTPColSize  = 1
	SFTPColTime  = 2
	SFTPColMode  = 3
	SFTPColIsDir = 4
	SFTPColIcon  = 5
)

// SFTPPanel represents the MobaXterm-style SFTP file browser panel
type SFTPPanel struct {
	Box           *gtk.Box
	PathEntry     *gtk.Entry
	TreeView      *gtk.TreeView
	ListStore     *gtk.ListStore
	ProgressBar   *gtk.ProgressBar
	ProgressBox   *gtk.Box
	StatusLabel   *gtk.Label
	client        *sftp.Client
	watcherMgr    *watcher.RemoteEditManager
	currentHostID string
	editorPref    string
}

func NewSFTPPanel(watcherMgr *watcher.RemoteEditManager) (*SFTPPanel, error) {
	box, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 2)
	if err != nil {
		return nil, err
	}
	box.SetSizeRequest(260, -1)

	// Top toolbar (Path + Nav buttons)
	topBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 2)
	topBox.SetMarginStart(4)
	topBox.SetMarginEnd(4)
	topBox.SetMarginTop(4)

	upBtn, _ := gtk.ButtonNewFromIconName("go-up-symbolic", gtk.ICON_SIZE_BUTTON)
	upBtn.SetTooltipText("На уровень выше")
	topBox.PackStart(upBtn, false, false, 0)

	refreshBtn, _ := gtk.ButtonNewFromIconName("view-refresh-symbolic", gtk.ICON_SIZE_BUTTON)
	refreshBtn.SetTooltipText("Обновить каталог")
	topBox.PackStart(refreshBtn, false, false, 0)

	pathEntry, _ := gtk.EntryNew()
	pathEntry.SetPlaceholderText("/remote/path")
	pathEntry.SetHExpand(true)
	topBox.PackStart(pathEntry, true, true, 0)

	box.PackStart(topBox, false, false, 0)

	// File List TreeView: Name, Size, ModTime, Mode, IsDir, Icon
	listStore, _ := gtk.ListStoreNew(glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_BOOLEAN, glib.TYPE_STRING)
	treeView, _ := gtk.TreeViewNewWithModel(listStore)
	treeView.SetHeadersVisible(true)

	// Column: Icon + Name
	colName, _ := gtk.TreeViewColumnNew()
	colName.SetTitle("Имя")
	colName.SetResizable(true)
	colName.SetExpand(true)
	rPixbuf, _ := gtk.CellRendererPixbufNew()
	rName, _ := gtk.CellRendererTextNew()
	colName.PackStart(rPixbuf, false)
	colName.AddAttribute(rPixbuf, "icon-name", SFTPColIcon)
	colName.PackStart(rName, true)
	colName.AddAttribute(rName, "text", SFTPColName)
	treeView.AppendColumn(colName)

	// Column: Size
	rSize, _ := gtk.CellRendererTextNew()
	colSize, _ := gtk.TreeViewColumnNewWithAttribute("Размер", rSize, "text", SFTPColSize)
	colSize.SetResizable(true)
	colSize.SetMinWidth(70)
	treeView.AppendColumn(colSize)

	// Scrolled container
	scrolled, _ := gtk.ScrolledWindowNew(nil, nil)
	scrolled.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	scrolled.SetShadowType(gtk.SHADOW_IN)
	scrolled.Add(treeView)
	box.PackStart(scrolled, true, true, 0)

	// Bottom Progress Box
	progressBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 2)
	progressBox.SetMarginStart(4)
	progressBox.SetMarginEnd(4)
	progressBox.SetMarginBottom(4)

	statusLabel, _ := gtk.LabelNew("Готово")
	statusLabel.SetHAlign(gtk.ALIGN_START)
	progressBox.PackStart(statusLabel, false, false, 0)

	pBar, _ := gtk.ProgressBarNew()
	progressBox.PackStart(pBar, false, false, 0)
	box.PackEnd(progressBox, false, false, 0)

	panel := &SFTPPanel{
		Box:         box,
		PathEntry:   pathEntry,
		TreeView:    treeView,
		ListStore:   listStore,
		ProgressBar: pBar,
		ProgressBox: progressBox,
		StatusLabel: statusLabel,
		watcherMgr:  watcherMgr,
	}

	// Double click row action
	treeView.Connect("row-activated", func(tv *gtk.TreeView, path *gtk.TreePath, column *gtk.TreeViewColumn) {
		iter, err := listStore.GetIter(path)
		if err != nil {
			return
		}
		valName, _ := listStore.GetValue(iter, SFTPColName)
		nameStr, _ := valName.GetString()
		valIsDir, _ := listStore.GetValue(iter, SFTPColIsDir)
		isDirVal, _ := valIsDir.GoValue()
		isDir, _ := isDirVal.(bool)

		if isDir {
			newPath := filepath.Join(panel.client.CurrentDir(), nameStr)
			panel.LoadDirectory(newPath)
		} else {
			// Remote Edit
			remoteFilePath := filepath.Join(panel.client.CurrentDir(), nameStr)
			panel.triggerRemoteEdit(remoteFilePath)
		}
	})

	// Right click context menu
	treeView.Connect("button-press-event", func(tv *gtk.TreeView, event *gdk.Event) bool {
		btnEvent := gdk.EventButtonNewFromEvent(event)
		if btnEvent.Button() == gdk.BUTTON_SECONDARY {
			path, _, _, _, ok := tv.GetPathAtPos(int(btnEvent.X()), int(btnEvent.Y()))
			if ok && path != nil {
				if sel, err := tv.GetSelection(); err == nil {
					sel.SelectPath(path)
				}
				iter, _ := listStore.GetIter(path)
				panel.showContextMenu(iter, btnEvent.Time())
				return true
			}
		}
		return false
	})

	upBtn.Connect("clicked", func() {
		if panel.client != nil {
			parentDir := filepath.Dir(panel.client.CurrentDir())
			panel.LoadDirectory(parentDir)
		}
	})

	refreshBtn.Connect("clicked", func() {
		if panel.client != nil {
			panel.LoadDirectory(panel.client.CurrentDir())
		}
	})

	pathEntry.Connect("activate", func() {
		target, _ := pathEntry.GetText()
		if target != "" {
			panel.LoadDirectory(target)
		}
	})

	return panel, nil
}

// AttachClient attaches active SFTP client for a session
func (sp *SFTPPanel) AttachClient(hostID string, client *sftp.Client, editorPref string) {
	sp.currentHostID = hostID
	sp.client = client
	sp.editorPref = editorPref
	if client != nil {
		sp.LoadDirectory(client.CurrentDir())
	} else {
		sp.ListStore.Clear()
		sp.PathEntry.SetText("")
	}
}

// LoadDirectory loads remote file list into TreeView
func (sp *SFTPPanel) LoadDirectory(path string) {
	if sp.client == nil {
		return
	}

	sp.StatusLabel.SetText("Загрузка каталога...")
	go func() {
		items, err := sp.client.ListDir(path)
		glib.IdleAdd(func() {
			if err != nil {
				sp.StatusLabel.SetText("Ошибка: " + err.Error())
				return
			}

			sp.client.SetCurrentDir(path)
			sp.PathEntry.SetText(path)
			sp.ListStore.Clear()

			for _, item := range items {
				iter := sp.ListStore.Append()
				icon := "text-x-generic"
				sizeStr := formatFileSize(item.Size)
				if item.IsDir {
					icon = "folder"
					sizeStr = "<DIR>"
				}

				_ = sp.ListStore.SetValue(iter, SFTPColName, item.Name)
				_ = sp.ListStore.SetValue(iter, SFTPColSize, sizeStr)
				_ = sp.ListStore.SetValue(iter, SFTPColTime, item.ModTime.Format("2006-01-02 15:04"))
				_ = sp.ListStore.SetValue(iter, SFTPColMode, item.Mode.String())
				_ = sp.ListStore.SetValue(iter, SFTPColIsDir, item.IsDir)
				_ = sp.ListStore.SetValue(iter, SFTPColIcon, icon)
			}

			sp.StatusLabel.SetText(fmt.Sprintf("Элементов: %d", len(items)))
		})
	}()
}

func (sp *SFTPPanel) triggerRemoteEdit(remotePath string) {
	if sp.client == nil || sp.watcherMgr == nil {
		return
	}

	sp.StatusLabel.SetText("Открытие в редакторе...")
	downloadFn := func(localPath string) error {
		return sp.client.DownloadFile(context.Background(), remotePath, localPath, nil)
	}

	uploadFn := func(ctx context.Context, localPath, remPath string) error {
		glib.IdleAdd(func() {
			sp.StatusLabel.SetText("Сохранение на сервер: " + filepath.Base(remPath))
		})
		err := sp.client.UploadFile(ctx, localPath, remPath, nil)
		glib.IdleAdd(func() {
			if err == nil {
				sp.StatusLabel.SetText("Файл сохранен: " + filepath.Base(remPath))
			} else {
				sp.StatusLabel.SetText("Ошибка сохранения: " + err.Error())
			}
		})
		return err
	}

	_ = sp.watcherMgr.OpenForEditing(sp.currentHostID, remotePath, downloadFn, uploadFn, sp.editorPref)
}

func (sp *SFTPPanel) showContextMenu(iter *gtk.TreeIter, eventTime uint32) {
	valName, _ := sp.ListStore.GetValue(iter, SFTPColName)
	nameStr, _ := valName.GetString()
	valIsDir, _ := sp.ListStore.GetValue(iter, SFTPColIsDir)
	isDirVal, _ := valIsDir.GoValue()
	isDir, _ := isDirVal.(bool)

	remotePath := filepath.Join(sp.client.CurrentDir(), nameStr)

	menu, _ := gtk.MenuNew()

	if !isDir {
		mEdit, _ := gtk.MenuItemNewWithLabel("Открыть во внешнем редакторе")
		mEdit.Connect("activate", func() {
			sp.triggerRemoteEdit(remotePath)
		})
		menu.Append(mEdit)
	}

	mDelete, _ := gtk.MenuItemNewWithLabel("Удалить")
	mDelete.Connect("activate", func() {
		_ = sp.client.Remove(remotePath)
		sp.LoadDirectory(sp.client.CurrentDir())
	})
	menu.Append(mDelete)

	mRefresh, _ := gtk.MenuItemNewWithLabel("Обновить")
	mRefresh.Connect("activate", func() {
		sp.LoadDirectory(sp.client.CurrentDir())
	})
	menu.Append(mRefresh)

	menu.ShowAll()
	menu.PopupAtPointer(nil)
}

func formatFileSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	} else if size < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
}
