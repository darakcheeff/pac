package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

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
	box.SetSizeRequest(10, -1)

	// Top toolbar (Path + Action buttons)
	topBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 2)
	topBox.SetMarginStart(4)
	topBox.SetMarginEnd(4)
	topBox.SetMarginTop(4)

	upBtn, _ := gtk.ButtonNewFromIconName("go-up-symbolic", gtk.ICON_SIZE_BUTTON)
	upBtn.SetTooltipText("Перейти в родительский каталог (Вверх)")
	topBox.PackStart(upBtn, false, false, 0)

	refreshBtn, _ := gtk.ButtonNewFromIconName("view-refresh-symbolic", gtk.ICON_SIZE_BUTTON)
	refreshBtn.SetTooltipText("Обновить список файлов и папок (F5)")
	topBox.PackStart(refreshBtn, false, false, 0)

	mkdirBtn, _ := gtk.ButtonNewFromIconName("folder-new-symbolic", gtk.ICON_SIZE_BUTTON)
	mkdirBtn.SetTooltipText("Создать новую папку на сервере (F7)")
	topBox.PackStart(mkdirBtn, false, false, 0)

	uploadBtn, _ := gtk.ButtonNew()
	uploadBtn.SetImage(GetUploadImage())
	uploadBtn.SetTooltipText("Выгрузить файлы с локального компьютера на сервер (Upload)")
	topBox.PackStart(uploadBtn, false, false, 0)

	downloadBtn, _ := gtk.ButtonNew()
	downloadBtn.SetImage(GetDownloadImage())
	downloadBtn.SetTooltipText("Скачать выбранный файл на локальный компьютер (Download)")
	topBox.PackStart(downloadBtn, false, false, 0)

	pathEntry, _ := gtk.EntryNew()
	pathEntry.SetPlaceholderText("/remote/path")
	pathEntry.SetTooltipText("Текущий путь на удаленном сервере (введите путь и нажмите Enter)")
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

	// Column: ModTime (Дата изменения)
	rTime, _ := gtk.CellRendererTextNew()
	colTime, _ := gtk.TreeViewColumnNewWithAttribute("Дата изменения", rTime, "text", SFTPColTime)
	colTime.SetResizable(true)
	colTime.SetMinWidth(120)
	treeView.AppendColumn(colTime)

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

	// Setup Drag and Drop Destination (drag files from local file manager into SFTP view to upload)
	if tEntry, err := gtk.TargetEntryNew("text/uri-list", gtk.TARGET_OTHER_APP, 0); err == nil {
		treeView.DragDestSet(gtk.DEST_DEFAULT_ALL, []gtk.TargetEntry{*tEntry}, gdk.ACTION_COPY)
		treeView.Connect("drag-data-received", func(tv *gtk.TreeView, context *gdk.DragContext, x, y int, data *gtk.SelectionData, info uint, time uint32) {
			uriList := string(data.GetData())
			lines := strings.Split(uriList, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "file://") {
					localPath := strings.TrimPrefix(line, "file://")
					panel.UploadLocalFile(localPath)
				}
			}
		})
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
			} else {
				// Empty area right click
				panel.showEmptyAreaContextMenu(btnEvent.Time())
				return true
			}
		}
		return false
	})

	// Top toolbar button actions
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

	mkdirBtn.Connect("clicked", func() {
		panel.showCreateFolderDialog()
	})

	uploadBtn.Connect("clicked", func() {
		panel.showUploadFileChooser()
	})

	downloadBtn.Connect("clicked", func() {
		if sel, err := treeView.GetSelection(); err == nil {
			if _, iter, ok := sel.GetSelected(); ok {
				panel.downloadSelectedFile(iter)
			}
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

// UploadLocalFile uploads local file to current remote directory with progress
func (sp *SFTPPanel) UploadLocalFile(localPath string) {
	if sp.client == nil {
		return
	}
	fileName := filepath.Base(localPath)
	remoteDest := filepath.Join(sp.client.CurrentDir(), fileName)

	sp.StatusLabel.SetText("Выгрузка: " + fileName)
	sp.ProgressBar.SetFraction(0.0)

	go func() {
		err := sp.client.UploadFile(context.Background(), localPath, remoteDest, func(transferred, total int64, speed float64) {
			if total > 0 {
				fraction := float64(transferred) / float64(total)
				glib.IdleAdd(func() {
					sp.ProgressBar.SetFraction(fraction)
				})
			}
		})
		glib.IdleAdd(func() {
			sp.ProgressBar.SetFraction(0.0)
			if err == nil {
				sp.StatusLabel.SetText("Выгрузка завершена: " + fileName)
				sp.LoadDirectory(sp.client.CurrentDir())
			} else {
				sp.StatusLabel.SetText("Ошибка выгрузки: " + err.Error())
			}
		})
	}()
}

func (sp *SFTPPanel) showUploadFileChooser() {
	dlg, _ := gtk.FileChooserDialogNewWith2Buttons(
		"Выберите файл для выгрузки на сервер",
		nil,
		gtk.FILE_CHOOSER_ACTION_OPEN,
		"Отмена", gtk.RESPONSE_CANCEL,
		"Выгрузить", gtk.RESPONSE_ACCEPT,
	)
	dlg.SetSelectMultiple(true)
	if dlg.Run() == gtk.RESPONSE_ACCEPT {
		filenames, err := dlg.GetFilenames()
		if err == nil {
			for _, path := range filenames {
				sp.UploadLocalFile(path)
			}
		}
	}
	dlg.Destroy()
}

func (sp *SFTPPanel) downloadSelectedFile(iter *gtk.TreeIter) {
	if sp.client == nil {
		return
	}
	valName, _ := sp.ListStore.GetValue(iter, SFTPColName)
	nameStr, _ := valName.GetString()
	remotePath := filepath.Join(sp.client.CurrentDir(), nameStr)

	dlg, _ := gtk.FileChooserDialogNewWith2Buttons(
		"Сохранить файл на локальный компьютер",
		nil,
		gtk.FILE_CHOOSER_ACTION_SAVE,
		"Отмена", gtk.RESPONSE_CANCEL,
		"Скачать", gtk.RESPONSE_ACCEPT,
	)
	dlg.SetCurrentName(nameStr)
	dlg.SetDoOverwriteConfirmation(true)

	if dlg.Run() == gtk.RESPONSE_ACCEPT {
		localPath := dlg.GetFilename()
		sp.StatusLabel.SetText("Скачивание: " + nameStr)
		sp.ProgressBar.SetFraction(0.0)

		go func() {
			err := sp.client.DownloadFile(context.Background(), remotePath, localPath, func(transferred, total int64, speed float64) {
				if total > 0 {
					fraction := float64(transferred) / float64(total)
					glib.IdleAdd(func() {
						sp.ProgressBar.SetFraction(fraction)
					})
				}
			})
			glib.IdleAdd(func() {
				sp.ProgressBar.SetFraction(0.0)
				if err == nil {
					sp.StatusLabel.SetText("Скачивание завершено: " + nameStr)
				} else {
					sp.StatusLabel.SetText("Ошибка скачивания: " + err.Error())
				}
			})
		}()
	}
	dlg.Destroy()
}

func (sp *SFTPPanel) showCreateFolderDialog() {
	if sp.client == nil {
		return
	}

	dlg, _ := gtk.DialogNew()
	dlg.SetTitle("Создать папку на сервере")
	dlg.SetModal(true)
	dlg.SetDefaultSize(320, 120)

	contentArea, _ := dlg.GetContentArea()
	vbox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 6)
	vbox.SetMarginStart(12)
	vbox.SetMarginEnd(12)
	vbox.SetMarginTop(12)
	vbox.SetMarginBottom(12)

	lbl, _ := gtk.LabelNew("Имя новой папки:")
	lbl.SetHAlign(gtk.ALIGN_START)
	vbox.PackStart(lbl, false, false, 0)

	entry, _ := gtk.EntryNew()
	entry.SetActivatesDefault(true)
	vbox.PackStart(entry, false, false, 0)
	contentArea.Add(vbox)

	_, _ = dlg.AddButton("Отмена", gtk.RESPONSE_CANCEL)
	btnOk, _ := dlg.AddButton("Создать", gtk.RESPONSE_OK)
	btnOk.SetCanDefault(true)
	dlg.SetDefault(btnOk)

	dlg.ShowAll()

	if dlg.Run() == gtk.RESPONSE_OK {
		folderName, _ := entry.GetText()
		if folderName != "" {
			newPath := filepath.Join(sp.client.CurrentDir(), folderName)
			err := sp.client.Mkdir(newPath)
			if err == nil {
				sp.LoadDirectory(sp.client.CurrentDir())
			} else {
				sp.StatusLabel.SetText("Ошибка создания папки: " + err.Error())
			}
		}
	}
	dlg.Destroy()
}

func (sp *SFTPPanel) showRenameDialog(iter *gtk.TreeIter) {
	if sp.client == nil {
		return
	}
	valName, _ := sp.ListStore.GetValue(iter, SFTPColName)
	oldName, _ := valName.GetString()
	oldPath := filepath.Join(sp.client.CurrentDir(), oldName)

	dlg, _ := gtk.DialogNew()
	dlg.SetTitle("Переименовать файл / папку")
	dlg.SetModal(true)
	dlg.SetDefaultSize(320, 120)

	contentArea, _ := dlg.GetContentArea()
	vbox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 6)
	vbox.SetMarginStart(12)
	vbox.SetMarginEnd(12)
	vbox.SetMarginTop(12)
	vbox.SetMarginBottom(12)

	lbl, _ := gtk.LabelNew("Новое имя:")
	lbl.SetHAlign(gtk.ALIGN_START)
	vbox.PackStart(lbl, false, false, 0)

	entry, _ := gtk.EntryNew()
	entry.SetText(oldName)
	entry.SetActivatesDefault(true)
	vbox.PackStart(entry, false, false, 0)
	contentArea.Add(vbox)

	_, _ = dlg.AddButton("Отмена", gtk.RESPONSE_CANCEL)
	btnOk, _ := dlg.AddButton("Сохранить", gtk.RESPONSE_OK)
	btnOk.SetCanDefault(true)
	dlg.SetDefault(btnOk)

	dlg.ShowAll()

	if dlg.Run() == gtk.RESPONSE_OK {
		newName, _ := entry.GetText()
		if newName != "" && newName != oldName {
			newPath := filepath.Join(sp.client.CurrentDir(), newName)
			err := sp.client.Rename(oldPath, newPath)
			if err == nil {
				sp.LoadDirectory(sp.client.CurrentDir())
			} else {
				sp.StatusLabel.SetText("Ошибка переименования: " + err.Error())
			}
		}
	}
	dlg.Destroy()
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

		mDownload, _ := gtk.MenuItemNewWithLabel("Скачать файл на компьютер (Download)")
		mDownload.Connect("activate", func() {
			sp.downloadSelectedFile(iter)
		})
		menu.Append(mDownload)
	}

	mRename, _ := gtk.MenuItemNewWithLabel("Переименовать (F2)")
	mRename.Connect("activate", func() {
		sp.showRenameDialog(iter)
	})
	menu.Append(mRename)

	mDelete, _ := gtk.MenuItemNewWithLabel("Удалить (Delete)")
	mDelete.Connect("activate", func() {
		dlg := gtk.MessageDialogNew(nil, gtk.DIALOG_MODAL, gtk.MESSAGE_QUESTION, gtk.BUTTONS_YES_NO,
			"Вы уверены, что хотите удалить '%s'?", nameStr)
		if dlg.Run() == gtk.RESPONSE_YES {
			_ = sp.client.Remove(remotePath)
			sp.LoadDirectory(sp.client.CurrentDir())
		}
		dlg.Destroy()
	})
	menu.Append(mDelete)

	sep, _ := gtk.SeparatorMenuItemNew()
	menu.Append(sep)

	mUpload, _ := gtk.MenuItemNewWithLabel("Выгрузить файл на сервер (Upload)...")
	mUpload.Connect("activate", func() {
		sp.showUploadFileChooser()
	})
	menu.Append(mUpload)

	mMkdir, _ := gtk.MenuItemNewWithLabel("Создать новую папку (F7)...")
	mMkdir.Connect("activate", func() {
		sp.showCreateFolderDialog()
	})
	menu.Append(mMkdir)

	mRefresh, _ := gtk.MenuItemNewWithLabel("Обновить каталог (F5)")
	mRefresh.Connect("activate", func() {
		sp.LoadDirectory(sp.client.CurrentDir())
	})
	menu.Append(mRefresh)

	menu.ShowAll()
	menu.PopupAtPointer(nil)
}

func (sp *SFTPPanel) showEmptyAreaContextMenu(eventTime uint32) {
	menu, _ := gtk.MenuNew()

	mUpload, _ := gtk.MenuItemNewWithLabel("Выгрузить файл на сервер (Upload)...")
	mUpload.Connect("activate", func() {
		sp.showUploadFileChooser()
	})
	menu.Append(mUpload)

	mMkdir, _ := gtk.MenuItemNewWithLabel("Создать новую папку (F7)...")
	mMkdir.Connect("activate", func() {
		sp.showCreateFolderDialog()
	})
	menu.Append(mMkdir)

	mRefresh, _ := gtk.MenuItemNewWithLabel("Обновить каталог (F5)")
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
