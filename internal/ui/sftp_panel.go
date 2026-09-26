package ui

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/darakcheeff/pac/internal/i18n"
	"github.com/darakcheeff/pac/internal/engine/sftp"
	"github.com/darakcheeff/pac/internal/engine/watcher"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	"github.com/gotk3/gotk3/pango"
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
// SessionTransferState tracks an ongoing transfer for a specific session
type SessionTransferState struct {
	IsTransferring  bool
	TransferType    string // "upload", "download", "download_multi"
	FileName        string
	Fraction        float64
	Speed           float64
	StatusText      string
	CancelFunc      context.CancelFunc
	uiUpdatePending int32
}

// SessionSFTPData maintains isolated SFTP browsing state per tab session
type SessionSFTPData struct {
	Client      *sftp.Client
	HostID      string
	CurrentPath string
	Items       []sftp.FileItem
	Transfer    SessionTransferState
	LastStatus  string
	IsLoading   bool
}

// SFTPPanel represents the MobaXterm-style SFTP file browser panel
type SFTPPanel struct {
	Box           *gtk.Box
	PathEntry     *gtk.Entry
	TreeView      *gtk.TreeView
	ListStore     *gtk.ListStore
	ProgressBar   *gtk.ProgressBar
	ProgressBox   *gtk.Box
	StatusLabel   *gtk.Label
	UploadBtn     *gtk.Button
	DownloadBtn   *gtk.Button
	client        *sftp.Client
	watcherMgr    *watcher.RemoteEditManager
	currentHostID string
	currentSessID string
	editorPref    string
	sessions      map[string]*SessionSFTPData
	sessMu        sync.Mutex
}

func NewSFTPPanel(watcherMgr *watcher.RemoteEditManager) (*SFTPPanel, error) {
	box, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 2)
	if err != nil {
		return nil, err
	}
	box.SetMarginStart(4)
	box.SetMarginEnd(2)

	// Top toolbar (Path + Action buttons)
	topBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 2)
	topBox.SetMarginStart(2)
	topBox.SetMarginEnd(2)
	topBox.SetMarginTop(2)

	upBtn, _ := gtk.ButtonNewFromIconName("go-up-symbolic", gtk.ICON_SIZE_BUTTON)
	upBtn.SetTooltipText(i18n.T("Перейти в родительский каталог (Вверх)", "Go to parent directory (Up)"))
	topBox.PackStart(upBtn, false, false, 0)

	refreshBtn, _ := gtk.ButtonNewFromIconName("view-refresh-symbolic", gtk.ICON_SIZE_BUTTON)
	refreshBtn.SetTooltipText(i18n.T("Обновить список файлов и папок (F5)", "Refresh files and folders (F5)"))
	topBox.PackStart(refreshBtn, false, false, 0)

	mkdirBtn, _ := gtk.ButtonNewFromIconName("folder-new-symbolic", gtk.ICON_SIZE_BUTTON)
	mkdirBtn.SetTooltipText(i18n.T("Создать новую папку на сервере (F7)", "Create new folder on server (F7)"))
	topBox.PackStart(mkdirBtn, false, false, 0)

	uploadBtn, _ := gtk.ButtonNew()
	uploadBtn.SetImage(GetUploadImage(true))
	uploadBtn.SetTooltipText(i18n.T("Выгрузить файлы на сервер (Upload)", "Upload files to server"))
	topBox.PackStart(uploadBtn, false, false, 0)

	downloadBtn, _ := gtk.ButtonNew()
	downloadBtn.SetImage(GetDownloadImage(true))
	downloadBtn.SetTooltipText(i18n.T("Скачать выбранный файл (Download)", "Download selected file"))
	topBox.PackStart(downloadBtn, false, false, 0)

	pathEntry, _ := gtk.EntryNew()
	pathEntry.SetPlaceholderText("/remote/path")
	pathEntry.SetTooltipText(i18n.T("Текущий путь на удаленном сервере (введите путь и нажмите Enter)", "Current remote path (enter path and press Enter)"))
	pathEntry.SetHExpand(true)
	pathEntry.SetWidthChars(5)
	topBox.PackStart(pathEntry, true, true, 0)

	box.PackStart(topBox, false, false, 0)

	// File List TreeView: Name, Size, ModTime, Mode, IsDir, Icon
	listStore, _ := gtk.ListStoreNew(glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_BOOLEAN, glib.TYPE_STRING)
	treeView, _ := gtk.TreeViewNewWithModel(listStore)
	treeView.SetHeadersVisible(true)
	if sel, err := treeView.GetSelection(); err == nil {
		sel.SetMode(gtk.SELECTION_MULTIPLE)
	}

	// Column: Icon + Name
	colName, _ := gtk.TreeViewColumnNew()
	colName.SetTitle(i18n.T("Имя", "Name"))
	colName.SetResizable(true)
	colName.SetExpand(true)
	rPixbuf, _ := gtk.CellRendererPixbufNew()
	rName, _ := gtk.CellRendererTextNew()
	colName.PackStart(rPixbuf, false)
	colName.AddAttribute(rPixbuf, "icon-name", SFTPColIcon)
	_ = rName.SetProperty("ellipsize", pango.ELLIPSIZE_END)
	colName.PackStart(rName, true)
	colName.AddAttribute(rName, "text", SFTPColName)
	treeView.AppendColumn(colName)

	// Column: Size
	rSize, _ := gtk.CellRendererTextNew()
	colSize, _ := gtk.TreeViewColumnNewWithAttribute(i18n.T("Размер", "Size"), rSize, "text", SFTPColSize)
	colSize.SetResizable(true)
	colSize.SetMinWidth(30)
	treeView.AppendColumn(colSize)

	// Column: ModTime (Дата изменения)
	rTime, _ := gtk.CellRendererTextNew()
	colTime, _ := gtk.TreeViewColumnNewWithAttribute(i18n.T("Дата изменения", "Date Modified"), rTime, "text", SFTPColTime)
	colTime.SetResizable(true)
	colTime.SetMinWidth(30)
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

	statusLabel, _ := gtk.LabelNew(i18n.T("Готово", "Ready"))
	statusLabel.SetHAlign(gtk.ALIGN_START)
	statusLabel.SetEllipsize(pango.ELLIPSIZE_END)
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
		UploadBtn:   uploadBtn,
		DownloadBtn: downloadBtn,
		watcherMgr:  watcherMgr,
		sessions:    make(map[string]*SessionSFTPData),
	}

	// Setup Drag and Drop: internal moving into folders + external upload from desktop/file manager
	targetURI, _ := gtk.TargetEntryNew("text/uri-list", 0, 1)
	targetText, _ := gtk.TargetEntryNew("text/plain", 0, 2)
	treeView.DragSourceSet(gdk.BUTTON1_MASK, []gtk.TargetEntry{*targetText}, gdk.ACTION_MOVE)
	treeView.DragDestSet(gtk.DEST_DEFAULT_ALL, []gtk.TargetEntry{*targetURI, *targetText}, gdk.ACTION_COPY|gdk.ACTION_MOVE)

	treeView.Connect("drag-data-get", func(tv *gtk.TreeView, ctx *gdk.DragContext, data *gtk.SelectionData, info uint, time uint32) {
		files := panel.getSelectedFiles()
		var names []string
		for _, f := range files {
			names = append(names, f.name)
		}
		data.SetText(strings.Join(names, "\n"))
	})

	treeView.Connect("drag-data-received", func(tv *gtk.TreeView, context *gdk.DragContext, x, y int, data *gtk.SelectionData, info uint, time uint32) {
		if panel.client == nil {
			return
		}

		// A. External file drop from desktop / file manager (text/uri-list)
		uris := data.GetURIs()
		if len(uris) == 0 {
			raw := string(data.GetData())
			lines := strings.Split(raw, "\n")
			for _, l := range lines {
				l = strings.TrimSpace(l)
				if strings.HasPrefix(l, "file://") {
					uris = append(uris, l)
				}
			}
		}

		if len(uris) > 0 {
			for _, uStr := range uris {
				uStr = strings.TrimSpace(uStr)
				if uStr == "" {
					continue
				}
				u, err := url.Parse(uStr)
				localPath := ""
				if err == nil && u.Path != "" {
					localPath = filepath.Clean(u.Path)
				} else if strings.HasPrefix(uStr, "file://") {
					localPath = filepath.Clean(strings.TrimPrefix(uStr, "file://"))
				}
				if localPath != "" {
					panel.UploadLocalFile(localPath)
				}
			}
			return
		}

		// B. Internal drag & drop to move file/folder into a directory
		text := data.GetText()
		if text == "" {
			return
		}
		path, _, _, _, ok := tv.GetPathAtPos(x, y)
		if !ok || path == nil {
			return
		}
		iter, err := listStore.GetIter(path)
		if err != nil {
			return
		}
		valIsDir, _ := listStore.GetValue(iter, SFTPColIsDir)
		isDirVal, _ := valIsDir.GoValue()
		if isDir, ok := isDirVal.(bool); ok && isDir {
			valName, _ := listStore.GetValue(iter, SFTPColName)
			targetFolder, _ := valName.GetString()
			if targetFolder == "" || targetFolder == "." || targetFolder == ".." {
				return
			}
			destDir := filepath.Join(panel.client.CurrentDir(), targetFolder)
			lines := strings.Split(text, "\n")
			for _, l := range lines {
				fName := strings.TrimSpace(l)
				if fName != "" && fName != targetFolder {
					oldPath := filepath.Join(panel.client.CurrentDir(), fName)
					newPath := filepath.Join(destDir, fName)
					_ = panel.client.Rename(oldPath, newPath)
				}
			}
			panel.LoadDirectory(panel.client.CurrentDir())
		}
	})

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

	// Keyboard shortcuts (F2: Rename, Delete: Remove, F5: Refresh, F7: New folder)
	treeView.Connect("key-press-event", func(tv *gtk.TreeView, event *gdk.Event) bool {
		keyEvent := gdk.EventKeyNewFromEvent(event)
		switch keyEvent.KeyVal() {
		case gdk.KEY_F2:
			panel.renameSelectedFile()
			return true
		case gdk.KEY_Delete, gdk.KEY_KP_Delete:
			panel.deleteSelectedFiles()
			return true
		case gdk.KEY_F5:
			if panel.client != nil {
				panel.LoadDirectory(panel.client.CurrentDir())
			}
			return true
		case gdk.KEY_F7:
			panel.showCreateFolderDialog()
			return true
		}
		return false
	})

	// Right click context menu
	treeView.Connect("button-press-event", func(tv *gtk.TreeView, event *gdk.Event) bool {
		tv.GrabFocus()
		btnEvent := gdk.EventButtonNewFromEvent(event)
		if btnEvent.Button() == gdk.BUTTON_SECONDARY {
			path, _, _, _, ok := tv.GetPathAtPos(int(btnEvent.X()), int(btnEvent.Y()))
			if ok && path != nil {
				if sel, err := tv.GetSelection(); err == nil {
					if !sel.PathIsSelected(path) {
						sel.UnselectAll()
						sel.SelectPath(path)
					}
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
		files := panel.getSelectedFiles()
		if len(files) == 1 {
			panel.downloadSelectedFile(files[0].iter)
		} else if len(files) > 1 {
			panel.downloadMultipleFiles(files)
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

func (sp *SFTPPanel) populateStore(items []sftp.FileItem) {
	sp.TreeView.SetModel(nil)
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

	sp.TreeView.SetModel(sp.ListStore)
}

// AttachClient attaches active SFTP client for a specific tab session
func (sp *SFTPPanel) AttachClient(sessionID, hostID string, client *sftp.Client, editorPref string) {
	sp.sessMu.Lock()
	defer sp.sessMu.Unlock()

	if sp.client == client && sp.currentSessID == sessionID && sp.currentHostID == hostID {
		sp.editorPref = editorPref
		return
	}

	sp.currentSessID = sessionID
	sp.currentHostID = hostID
	sp.client = client
	sp.editorPref = editorPref

	if client == nil || sessionID == "" {
		sp.TreeView.SetModel(nil)
		sp.ListStore.Clear()
		sp.TreeView.SetModel(sp.ListStore)
		sp.PathEntry.SetText("")
		sp.ProgressBar.SetFraction(0.0)
		sp.StatusLabel.SetText(i18n.T("Готово", "Ready"))
		return
	}

	sData, exists := sp.sessions[sessionID]
	if !exists {
		sData = &SessionSFTPData{
			Client: client,
			HostID: hostID,
		}
		sp.sessions[sessionID] = sData
	} else {
		sData.Client = client
		sData.HostID = hostID
	}

	currentPath := sData.CurrentPath
	if currentPath == "" {
		currentPath = client.CurrentDir()
		sData.CurrentPath = currentPath
	}
	sp.PathEntry.SetText(currentPath)

	if sData.Transfer.IsTransferring {
		sp.ProgressBar.SetFraction(sData.Transfer.Fraction)
		sp.StatusLabel.SetText(sData.Transfer.StatusText)
	} else {
		sp.ProgressBar.SetFraction(0.0)
		if sData.LastStatus != "" {
			sp.StatusLabel.SetText(sData.LastStatus)
		} else if len(sData.Items) > 0 {
			sp.StatusLabel.SetText(i18n.Tf("Элементов: %d", "Items: %d", len(sData.Items)))
		} else {
			sp.StatusLabel.SetText(i18n.T("Готово", "Ready"))
		}
	}

	if sData.Items != nil {
		sp.populateStore(sData.Items)
	} else {
		go sp.loadDirectoryInternal(sessionID, client, currentPath)
	}
}

// GetCurrentSessionID returns the currently displayed session ID
func (sp *SFTPPanel) GetCurrentSessionID() string {
	sp.sessMu.Lock()
	defer sp.sessMu.Unlock()
	return sp.currentSessID
}

// RemoveSession cleans up state and cancels active transfers when a tab is closed
func (sp *SFTPPanel) RemoveSession(sessionID string) {
	sp.sessMu.Lock()
	if sData, ok := sp.sessions[sessionID]; ok {
		if sData.Transfer.CancelFunc != nil {
			sData.Transfer.CancelFunc()
		}
		delete(sp.sessions, sessionID)
	}
	wasCurrent := (sp.currentSessID == sessionID)
	if wasCurrent {
		sp.currentSessID = ""
		sp.client = nil
	}
	sp.sessMu.Unlock()

	if wasCurrent {
		glib.IdleAdd(func() {
			sp.TreeView.SetModel(nil)
			sp.ListStore.Clear()
			sp.TreeView.SetModel(sp.ListStore)
			sp.PathEntry.SetText("")
			sp.ProgressBar.SetFraction(0.0)
			sp.StatusLabel.SetText(i18n.T("Готово", "Ready"))
		})
	}
}

// InvalidateSessionCache invalidates cached file listing for a session (e.g. when cd in background)
func (sp *SFTPPanel) InvalidateSessionCache(sessionID string, newPath string) {
	sp.sessMu.Lock()
	defer sp.sessMu.Unlock()

	if sData, ok := sp.sessions[sessionID]; ok {
		sData.CurrentPath = newPath
		sData.Items = nil
	}
}

// LoadDirectory loads remote file list for currently active session
func (sp *SFTPPanel) LoadDirectory(path string) {
	sp.sessMu.Lock()
	sessionID := sp.currentSessID
	client := sp.client
	sp.sessMu.Unlock()

	if client == nil || sessionID == "" {
		return
	}
	go sp.loadDirectoryInternal(sessionID, client, path)
}

func (sp *SFTPPanel) loadDirectoryInternal(sessionID string, client *sftp.Client, path string) {
	sp.sessMu.Lock()
	sData, ok := sp.sessions[sessionID]
	if !ok {
		sData = &SessionSFTPData{Client: client}
		sp.sessions[sessionID] = sData
	}
	if sData.IsLoading {
		sp.sessMu.Unlock()
		return
	}
	sData.IsLoading = true
	isCurrent := (sp.currentSessID == sessionID)
	sp.sessMu.Unlock()

	if isCurrent {
		glib.IdleAdd(func() {
			sp.sessMu.Lock()
			curr := (sp.currentSessID == sessionID)
			sp.sessMu.Unlock()
			if curr {
				sp.StatusLabel.SetText(i18n.T("Загрузка каталога...", "Loading directory..."))
			}
		})
	}

	items, err := client.ListDir(path)

	sp.sessMu.Lock()
	if sd, ok := sp.sessions[sessionID]; ok {
		sd.IsLoading = false
		if err == nil {
			sd.Items = items
			sd.CurrentPath = path
			client.SetCurrentDir(path)
		}
	}
	sp.sessMu.Unlock()

	glib.IdleAdd(func() {
		sp.sessMu.Lock()
		isStillCurrent := (sp.currentSessID == sessionID && sp.client == client)
		sp.sessMu.Unlock()

		if !isStillCurrent {
			return
		}

		if err != nil {
			sp.StatusLabel.SetText(i18n.T("Ошибка: ", "Error: ") + err.Error())
			log.Printf("[SFTP] Error loading directory %q: %v", path, err)
			return
		}

		sp.PathEntry.SetText(path)
		sp.populateStore(items)
		sp.StatusLabel.SetText(i18n.Tf("Элементов: %d", "Items: %d", len(items)))
	})
}

// UploadLocalFile uploads local file to current remote directory with progress
func (sp *SFTPPanel) UploadLocalFile(localPath string) {
	sp.sessMu.Lock()
	sessionID := sp.currentSessID
	client := sp.client
	sData := sp.sessions[sessionID]
	if client == nil || sessionID == "" || sData == nil {
		sp.sessMu.Unlock()
		return
	}
	if sData.Transfer.IsTransferring {
		sp.sessMu.Unlock()
		sp.showError(i18n.T("Передача уже выполняется", "Transfer already in progress"),
			i18n.T("Пожалуйста, дождитесь окончания текущей передачи файлов в этой сессии.", "Please wait for the current file transfer in this session to complete."))
		return
	}

	fileName := filepath.Base(localPath)
	remoteDest := filepath.Join(client.CurrentDir(), fileName)

	ctx, cancel := context.WithCancel(context.Background())
	sData.Transfer = SessionTransferState{
		IsTransferring: true,
		TransferType:   "upload",
		FileName:       fileName,
		Fraction:       0.0,
		StatusText:     i18n.T("Выгрузка: ", "Uploading: ") + fileName,
		CancelFunc:     cancel,
	}
	sp.sessMu.Unlock()

	sp.StatusLabel.SetText(i18n.T("Выгрузка: ", "Uploading: ") + fileName)
	sp.ProgressBar.SetFraction(0.0)

	go func() {
		err := client.UploadFile(ctx, localPath, remoteDest, func(transferred, total int64, speed float64) {
			if total <= 0 {
				return
			}
			fraction := float64(transferred) / float64(total)
			speedStr := formatFileSize(int64(speed)) + "/s"
			statusText := fmt.Sprintf("%s %s (%.0f%%, %s)",
				i18n.T("Выгрузка: ", "Uploading: "),
				fileName,
				fraction*100,
				speedStr,
			)

			sp.sessMu.Lock()
			if sd, ok := sp.sessions[sessionID]; ok {
				sd.Transfer.Fraction = fraction
				sd.Transfer.StatusText = statusText
			}
			sp.sessMu.Unlock()

			if atomic.CompareAndSwapInt32(&sData.Transfer.uiUpdatePending, 0, 1) {
				glib.IdleAdd(func() {
					atomic.StoreInt32(&sData.Transfer.uiUpdatePending, 0)
					sp.sessMu.Lock()
					isCurrent := (sp.currentSessID == sessionID)
					sp.sessMu.Unlock()
					if isCurrent {
						sp.ProgressBar.SetFraction(fraction)
						sp.StatusLabel.SetText(statusText)
					}
				})
			}
		})

		glib.IdleAdd(func() {
			var finalStatus string
			if err == nil {
				finalStatus = i18n.T("Выгрузка завершена: ", "Upload completed: ") + fileName
			} else if err == context.Canceled {
				finalStatus = i18n.T("Выгрузка отменена: ", "Upload canceled: ") + fileName
			} else {
				finalStatus = i18n.T("Ошибка выгрузки: ", "Upload error: ") + err.Error()
			}

			sp.sessMu.Lock()
			if sd, ok := sp.sessions[sessionID]; ok {
				sd.Transfer.IsTransferring = false
				sd.Transfer.Fraction = 0.0
				sd.Transfer.StatusText = ""
				sd.Transfer.CancelFunc = nil
				sd.LastStatus = finalStatus
			}
			isCurrent := (sp.currentSessID == sessionID)
			sp.sessMu.Unlock()

			if isCurrent {
				sp.ProgressBar.SetFraction(0.0)
				sp.StatusLabel.SetText(finalStatus)
				if err == nil {
					sp.LoadDirectory(client.CurrentDir())
				} else if err != context.Canceled {
					sp.showError(i18n.T("Ошибка выгрузки файла", "Upload Error"),
						i18n.Tf("Не удалось выгрузить файл \"%s\":\n\n%s", "Failed to upload file \"%s\":\n\n%s", fileName, err.Error()))
				}
			} else if err == nil {
				go sp.loadDirectoryInternal(sessionID, client, client.CurrentDir())
			}
		})
	}()
}

func (sp *SFTPPanel) showUploadFileChooser() {
	dlg, _ := gtk.FileChooserDialogNewWith2Buttons(
		i18n.T("Выберите файл для выгрузки на сервер", "Select file to upload"),
		nil,
		gtk.FILE_CHOOSER_ACTION_OPEN,
		i18n.T("Отмена", "Cancel"), gtk.RESPONSE_CANCEL,
		i18n.T("Выгрузить", "Upload"), gtk.RESPONSE_ACCEPT,
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
	sp.sessMu.Lock()
	sessionID := sp.currentSessID
	client := sp.client
	sData := sp.sessions[sessionID]
	sp.sessMu.Unlock()

	if client == nil || sessionID == "" || sData == nil {
		return
	}
	if sData.Transfer.IsTransferring {
		sp.showError(i18n.T("Передача уже выполняется", "Transfer already in progress"),
			i18n.T("Пожалуйста, дождитесь окончания текущей передачи файлов в этой сессии.", "Please wait for the current file transfer in this session to complete."))
		return
	}

	valName, _ := sp.ListStore.GetValue(iter, SFTPColName)
	nameStr, _ := valName.GetString()
	remotePath := filepath.Join(client.CurrentDir(), nameStr)

	dlg, _ := gtk.FileChooserDialogNewWith2Buttons(
		i18n.T("Сохранить файл на локальный компьютер", "Save file to local computer"),
		nil,
		gtk.FILE_CHOOSER_ACTION_SAVE,
		i18n.T("Отмена", "Cancel"), gtk.RESPONSE_CANCEL,
		i18n.T("Скачать", "Download"), gtk.RESPONSE_ACCEPT,
	)
	dlg.SetCurrentName(nameStr)
	dlg.SetDoOverwriteConfirmation(true)

	if dlg.Run() == gtk.RESPONSE_ACCEPT {
		localPath := dlg.GetFilename()
		dlg.Destroy()

		ctx, cancel := context.WithCancel(context.Background())
		sp.sessMu.Lock()
		sData.Transfer = SessionTransferState{
			IsTransferring: true,
			TransferType:   "download",
			FileName:       nameStr,
			Fraction:       0.0,
			StatusText:     i18n.T("Скачивание: ", "Downloading: ") + nameStr,
			CancelFunc:     cancel,
		}
		sp.sessMu.Unlock()

		sp.StatusLabel.SetText(i18n.T("Скачивание: ", "Downloading: ") + nameStr)
		sp.ProgressBar.SetFraction(0.0)

		go func() {
			err := client.DownloadFile(ctx, remotePath, localPath, func(transferred, total int64, speed float64) {
				if total <= 0 {
					return
				}
				fraction := float64(transferred) / float64(total)
				speedStr := formatFileSize(int64(speed)) + "/s"
				statusText := fmt.Sprintf("%s %s (%.0f%%, %s)",
					i18n.T("Скачивание: ", "Downloading: "),
					nameStr,
					fraction*100,
					speedStr,
				)

				sp.sessMu.Lock()
				if sd, ok := sp.sessions[sessionID]; ok {
					sd.Transfer.Fraction = fraction
					sd.Transfer.StatusText = statusText
				}
				sp.sessMu.Unlock()

				if atomic.CompareAndSwapInt32(&sData.Transfer.uiUpdatePending, 0, 1) {
					glib.IdleAdd(func() {
						atomic.StoreInt32(&sData.Transfer.uiUpdatePending, 0)
						sp.sessMu.Lock()
						isCurrent := (sp.currentSessID == sessionID)
						sp.sessMu.Unlock()
						if isCurrent {
							sp.ProgressBar.SetFraction(fraction)
							sp.StatusLabel.SetText(statusText)
						}
					})
				}
			})

			glib.IdleAdd(func() {
				var finalStatus string
				if err == nil {
					finalStatus = i18n.T("Скачивание завершено: ", "Download completed: ") + nameStr
				} else if err == context.Canceled {
					finalStatus = i18n.T("Скачивание отменено: ", "Download canceled: ") + nameStr
				} else {
					finalStatus = i18n.T("Ошибка скачивания: ", "Download error: ") + err.Error()
				}

				sp.sessMu.Lock()
				if sd, ok := sp.sessions[sessionID]; ok {
					sd.Transfer.IsTransferring = false
					sd.Transfer.Fraction = 0.0
					sd.Transfer.StatusText = ""
					sd.Transfer.CancelFunc = nil
					sd.LastStatus = finalStatus
				}
				isCurrent := (sp.currentSessID == sessionID)
				sp.sessMu.Unlock()

				if isCurrent {
					sp.ProgressBar.SetFraction(0.0)
					sp.StatusLabel.SetText(finalStatus)
					if err != nil && err != context.Canceled {
						sp.showError(i18n.T("Ошибка загрузки файла", "Download Error"),
							i18n.Tf("Не удалось скачать файл \"%s\":\n\n%s", "Failed to download file \"%s\":\n\n%s", nameStr, err.Error()))
					}
				}
			})
		}()
	} else {
		dlg.Destroy()
	}
}

func (sp *SFTPPanel) showCreateFolderDialog() {
	if sp.client == nil {
		return
	}

	dlg, _ := gtk.DialogNew()
	dlg.SetTitle(i18n.T("Создать папку на сервере", "Create Folder on Server"))
	dlg.SetModal(true)
	dlg.SetDefaultSize(320, 120)

	contentArea, _ := dlg.GetContentArea()
	vbox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 6)
	vbox.SetMarginStart(12)
	vbox.SetMarginEnd(12)
	vbox.SetMarginTop(12)
	vbox.SetMarginBottom(12)

	lbl, _ := gtk.LabelNew(i18n.T("Имя новой папки:", "New folder name:"))
	lbl.SetHAlign(gtk.ALIGN_START)
	vbox.PackStart(lbl, false, false, 0)

	entry, _ := gtk.EntryNew()
	entry.SetActivatesDefault(true)
	vbox.PackStart(entry, false, false, 0)
	contentArea.Add(vbox)

	_, _ = dlg.AddButton(i18n.T("Отмена", "Cancel"), gtk.RESPONSE_CANCEL)
	btnOk, _ := dlg.AddButton(i18n.T("Создать", "Create"), gtk.RESPONSE_OK)
	btnOk.SetCanDefault(true)
	dlg.SetDefault(btnOk)

	dlg.ShowAll()
	dlg.Present()

	if dlg.Run() == gtk.RESPONSE_OK {
		folderName, _ := entry.GetText()
		if folderName != "" {
			newPath := filepath.Join(sp.client.CurrentDir(), folderName)
			err := sp.client.Mkdir(newPath)
			if err == nil {
				sp.LoadDirectory(sp.client.CurrentDir())
			} else {
				sp.StatusLabel.SetText(i18n.T("Ошибка создания папки: ", "Folder creation error: ") + err.Error())
				sp.showError(i18n.T("Ошибка создания папки", "Folder Creation Error"),
					i18n.Tf("Не удалось создать папку \"%s\":\n\n%s", "Failed to create folder \"%s\":\n\n%s", folderName, err.Error()))
			}
		}
	}
	dlg.Destroy()
}

func (sp *SFTPPanel) showRenameDialog(oldName string) {
	if sp.client == nil {
		return
	}

	oldPath := filepath.Join(sp.client.CurrentDir(), oldName)

	dlg, _ := gtk.DialogNew()
	dlg.SetTitle(i18n.T("Переименовать файл / папку", "Rename File / Folder"))
	dlg.SetModal(true)
	dlg.SetDefaultSize(320, 120)

	contentArea, _ := dlg.GetContentArea()
	vbox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 6)
	vbox.SetMarginStart(12)
	vbox.SetMarginEnd(12)
	vbox.SetMarginTop(12)
	vbox.SetMarginBottom(12)

	lbl, _ := gtk.LabelNew(i18n.T("Новое имя:", "New name:"))
	lbl.SetHAlign(gtk.ALIGN_START)
	vbox.PackStart(lbl, false, false, 0)

	entry, _ := gtk.EntryNew()
	entry.SetText(oldName)
	entry.SetActivatesDefault(true)
	vbox.PackStart(entry, false, false, 0)
	contentArea.Add(vbox)

	_, _ = dlg.AddButton(i18n.T("Отмена", "Cancel"), gtk.RESPONSE_CANCEL)
	btnOk, _ := dlg.AddButton(i18n.T("Сохранить", "Save"), gtk.RESPONSE_OK)
	btnOk.SetCanDefault(true)
	dlg.SetDefault(btnOk)

	dlg.ShowAll()
	dlg.Present()

	if dlg.Run() == gtk.RESPONSE_OK {
		newName, _ := entry.GetText()
		if newName != "" && newName != oldName {
			newPath := filepath.Join(sp.client.CurrentDir(), newName)
			err := sp.client.Rename(oldPath, newPath)
			if err == nil {
				sp.LoadDirectory(sp.client.CurrentDir())
			} else {
				sp.StatusLabel.SetText(i18n.T("Ошибка переименования: ", "Rename error: ") + err.Error())
				sp.showError(i18n.T("Ошибка переименования", "Rename Error"),
					i18n.Tf("Не удалось переименовать \"%s\" в \"%s\":\n\n%s", "Failed to rename \"%s\" to \"%s\":\n\n%s", oldName, newName, err.Error()))
			}
		}
	}
	dlg.Destroy()
}

func (sp *SFTPPanel) triggerRemoteEdit(remotePath string) {
	sp.sessMu.Lock()
	sessionID := sp.currentSessID
	hostID := sp.currentHostID
	sp.sessMu.Unlock()

	if sp.client == nil || sp.watcherMgr == nil || sessionID == "" {
		return
	}

	sp.StatusLabel.SetText(i18n.T("Открытие в редакторе...", "Opening in editor..."))
	downloadFn := func(localPath string) error {
		return sp.client.DownloadFile(context.Background(), remotePath, localPath, nil)
	}

	uploadFn := func(ctx context.Context, localPath, remPath string) error {
		glib.IdleAdd(func() {
			sp.sessMu.Lock()
			isCurrent := (sp.currentSessID == sessionID)
			sp.sessMu.Unlock()
			if isCurrent {
				sp.StatusLabel.SetText(i18n.T("Сохранение на сервер: ", "Saving to server: ") + filepath.Base(remPath))
			}
		})
		err := sp.client.UploadFile(ctx, localPath, remPath, nil)
		glib.IdleAdd(func() {
			sp.sessMu.Lock()
			isCurrent := (sp.currentSessID == sessionID)
			sp.sessMu.Unlock()
			if isCurrent {
				if err == nil {
					sp.StatusLabel.SetText(i18n.T("Файл сохранен: ", "File saved: ") + filepath.Base(remPath))
				} else {
					sp.StatusLabel.SetText(i18n.T("Ошибка сохранения: ", "Save error: ") + err.Error())
				}
			}
		})
		return err
	}

	_ = sp.watcherMgr.OpenForEditing(hostID, remotePath, downloadFn, uploadFn, sp.editorPref)
}

type sftpSelectedFile struct {
	name  string
	isDir bool
	path  string
	iter  *gtk.TreeIter
}

func (sp *SFTPPanel) getSelectedFiles() []sftpSelectedFile {
	sel, err := sp.TreeView.GetSelection()
	if err != nil {
		return nil
	}
	var files []sftpSelectedFile
	sel.SelectedForEach(func(model *gtk.TreeModel, path *gtk.TreePath, iter *gtk.TreeIter) {
		valName, _ := sp.ListStore.GetValue(iter, SFTPColName)
		nameStr, _ := valName.GetString()
		valIsDir, _ := sp.ListStore.GetValue(iter, SFTPColIsDir)
		isDirVal, _ := valIsDir.GoValue()
		isDir, _ := isDirVal.(bool)
		if nameStr != "" && sp.client != nil {
			remotePath := filepath.Join(sp.client.CurrentDir(), nameStr)
			files = append(files, sftpSelectedFile{
				name:  nameStr,
				isDir: isDir,
				path:  remotePath,
				iter:  iter,
			})
		}
	})
	return files
}

func (sp *SFTPPanel) renameSelectedFile() {
	files := sp.getSelectedFiles()
	if len(files) != 1 {
		return
	}
	sp.showRenameDialog(files[0].name)
}

func (sp *SFTPPanel) deleteSelectedFiles() {
	sp.sessMu.Lock()
	sessionID := sp.currentSessID
	client := sp.client
	sp.sessMu.Unlock()

	if client == nil || sessionID == "" {
		return
	}
	files := sp.getSelectedFiles()
	if len(files) == 0 {
		return
	}

	var confirmMsg string
	if len(files) == 1 {
		confirmMsg = i18n.Tf("Вы уверены, что хотите удалить '%s'?", "Are you sure you want to delete '%s'?", files[0].name)
	} else {
		confirmMsg = i18n.Tf("Вы уверены, что хотите удалить выбранные элементы (%d шт.)?", "Are you sure you want to delete selected items (%d)?", len(files))
	}

	dlg := gtk.MessageDialogNew(nil, gtk.DIALOG_MODAL, gtk.MESSAGE_QUESTION, gtk.BUTTONS_YES_NO, "%s", confirmMsg)
	if dlg.Run() == gtk.RESPONSE_YES {
		dlg.Destroy()
		sp.StatusLabel.SetText(i18n.T("Удаление...", "Deleting..."))
		go func() {
			var errCount int
			for _, f := range files {
				if err := client.Remove(f.path); err != nil {
					errCount++
				}
			}
			glib.IdleAdd(func() {
				sp.sessMu.Lock()
				isCurrent := (sp.currentSessID == sessionID)
				sp.sessMu.Unlock()

				if isCurrent {
					if errCount > 0 {
						sp.StatusLabel.SetText(i18n.Tf("Ошибок при удалении: %d", "Errors while deleting: %d", errCount))
						sp.showError(i18n.T("Ошибка удаления", "Deletion Error"),
							i18n.Tf("При удалении элементов произошло ошибок: %d", "Errors while deleting items: %d", errCount))
					} else {
						sp.StatusLabel.SetText(i18n.T("Удаление завершено", "Deletion completed"))
					}
					sp.LoadDirectory(client.CurrentDir())
				} else {
					go sp.loadDirectoryInternal(sessionID, client, client.CurrentDir())
				}
			})
		}()
	} else {
		dlg.Destroy()
	}
}

func (sp *SFTPPanel) downloadMultipleFiles(files []sftpSelectedFile) {
	sp.sessMu.Lock()
	sessionID := sp.currentSessID
	client := sp.client
	sData := sp.sessions[sessionID]
	sp.sessMu.Unlock()

	if client == nil || len(files) == 0 || sessionID == "" || sData == nil {
		return
	}
	if sData.Transfer.IsTransferring {
		sp.showError(i18n.T("Передача уже выполняется", "Transfer already in progress"),
			i18n.T("Пожалуйста, дождитесь окончания текущей передачи файлов в этой сессии.", "Please wait for the current file transfer in this session to complete."))
		return
	}

	dlg, _ := gtk.FileChooserDialogNewWith2Buttons(
		i18n.T("Выберите папку для сохранения файлов", "Select destination folder to save files"),
		nil,
		gtk.FILE_CHOOSER_ACTION_SELECT_FOLDER,
		i18n.T("Отмена", "Cancel"), gtk.RESPONSE_CANCEL,
		i18n.T("Выбрать", "Select"), gtk.RESPONSE_ACCEPT,
	)
	if dlg.Run() == gtk.RESPONSE_ACCEPT {
		targetDir := dlg.GetFilename()
		dlg.Destroy()

		ctx, cancel := context.WithCancel(context.Background())
		sp.sessMu.Lock()
		sData.Transfer = SessionTransferState{
			IsTransferring: true,
			TransferType:   "download_multi",
			FileName:       fmt.Sprintf("%d files", len(files)),
			Fraction:       0.0,
			StatusText:     i18n.T("Скачивание файлов...", "Downloading files..."),
			CancelFunc:     cancel,
		}
		sp.sessMu.Unlock()

		sp.StatusLabel.SetText(i18n.T("Скачивание файлов...", "Downloading files..."))
		sp.ProgressBar.SetFraction(0.0)

		go func() {
			var nonDirFiles []sftpSelectedFile
			for _, f := range files {
				if !f.isDir {
					nonDirFiles = append(nonDirFiles, f)
				}
			}
			totalCount := len(nonDirFiles)
			for i, f := range nonDirFiles {
				select {
				case <-ctx.Done():
					break
				default:
				}
				localDest := filepath.Join(targetDir, f.name)
				statusText := fmt.Sprintf("%s (%d/%d): %s",
					i18n.T("Скачивание: ", "Downloading: "),
					i+1, totalCount, f.name)

				sp.sessMu.Lock()
				if sd, ok := sp.sessions[sessionID]; ok {
					sd.Transfer.Fraction = float64(i) / float64(totalCount)
					sd.Transfer.StatusText = statusText
				}
				sp.sessMu.Unlock()

				glib.IdleAdd(func() {
					sp.sessMu.Lock()
					isCurrent := (sp.currentSessID == sessionID)
					sp.sessMu.Unlock()
					if isCurrent {
						sp.ProgressBar.SetFraction(float64(i) / float64(totalCount))
						sp.StatusLabel.SetText(statusText)
					}
				})

				_ = client.DownloadFile(ctx, f.path, localDest, nil)
			}

			glib.IdleAdd(func() {
				sp.sessMu.Lock()
				if sd, ok := sp.sessions[sessionID]; ok {
					sd.Transfer.IsTransferring = false
					sd.Transfer.Fraction = 0.0
					sd.Transfer.StatusText = ""
					sd.Transfer.CancelFunc = nil
					sd.LastStatus = i18n.T("Скачивание завершено", "Download completed")
				}
				isCurrent := (sp.currentSessID == sessionID)
				sp.sessMu.Unlock()

				if isCurrent {
					sp.ProgressBar.SetFraction(0.0)
					sp.StatusLabel.SetText(i18n.T("Скачивание завершено", "Download completed"))
				}
			})
		}()
	} else {
		dlg.Destroy()
	}
}

func (sp *SFTPPanel) showContextMenu(iter *gtk.TreeIter, eventTime uint32) {
	selectedFiles := sp.getSelectedFiles()
	if len(selectedFiles) > 1 {
		menu, _ := gtk.MenuNew()

		mDownload, _ := gtk.MenuItemNewWithLabel(i18n.Tf("Скачать выбранные файлы (%d)...", "Download selected files (%d)...", len(selectedFiles)))
		mDownload.Connect("activate", func() {
			sp.downloadMultipleFiles(selectedFiles)
		})
		menu.Append(mDownload)

		mDelete, _ := gtk.MenuItemNewWithLabel(i18n.Tf("Удалить выбранные элементы (%d шт.) (Delete)", "Delete selected items (%d) (Delete)", len(selectedFiles)))
		mDelete.Connect("activate", func() {
			sp.deleteSelectedFiles()
		})
		menu.Append(mDelete)

		sep, _ := gtk.SeparatorMenuItemNew()
		menu.Append(sep)

		mUpload, _ := gtk.MenuItemNewWithLabel(i18n.T("Выгрузить файл на сервер (Upload)...", "Upload file to server..."))
		mUpload.Connect("activate", func() {
			sp.showUploadFileChooser()
		})
		menu.Append(mUpload)

		mMkdir, _ := gtk.MenuItemNewWithLabel(i18n.T("Создать новую папку (F7)...", "Create new folder (F7)..."))
		mMkdir.Connect("activate", func() {
			sp.showCreateFolderDialog()
		})
		menu.Append(mMkdir)

		mRefresh, _ := gtk.MenuItemNewWithLabel(i18n.T("Обновить каталог (F5)", "Refresh directory (F5)"))
		mRefresh.Connect("activate", func() {
			sp.LoadDirectory(sp.client.CurrentDir())
		})
		menu.Append(mRefresh)

		menu.ShowAll()
		menu.PopupAtPointer(nil)
		return
	}

	valName, _ := sp.ListStore.GetValue(iter, SFTPColName)
	nameStr, _ := valName.GetString()
	valIsDir, _ := sp.ListStore.GetValue(iter, SFTPColIsDir)
	isDirVal, _ := valIsDir.GoValue()
	isDir, _ := isDirVal.(bool)

	remotePath := filepath.Join(sp.client.CurrentDir(), nameStr)

	menu, _ := gtk.MenuNew()

	if !isDir {
		mEdit, _ := gtk.MenuItemNewWithLabel(i18n.T("Открыть во внешнем редакторе", "Open in external editor"))
		mEdit.Connect("activate", func() {
			sp.triggerRemoteEdit(remotePath)
		})
		menu.Append(mEdit)

		mDownload, _ := gtk.MenuItemNewWithLabel(i18n.T("Скачать файл (Download)", "Download file"))
		mDownload.Connect("activate", func() {
			sp.downloadSelectedFile(iter)
		})
		menu.Append(mDownload)
	}

	mRename, _ := gtk.MenuItemNewWithLabel(i18n.T("Переименовать (F2)", "Rename (F2)"))
	mRename.Connect("activate", func() {
		sp.showRenameDialog(nameStr)
		})
	menu.Append(mRename)

	mDelete, _ := gtk.MenuItemNewWithLabel(i18n.T("Удалить (Delete)", "Delete (Delete)"))
	mDelete.Connect("activate", func() {
		sp.deleteSelectedFiles()
	})
	menu.Append(mDelete)

	sep, _ := gtk.SeparatorMenuItemNew()
	menu.Append(sep)

	mUpload, _ := gtk.MenuItemNewWithLabel(i18n.T("Выгрузить файл на сервер (Upload)...", "Upload file to server..."))
	mUpload.Connect("activate", func() {
		sp.showUploadFileChooser()
	})
	menu.Append(mUpload)

	mMkdir, _ := gtk.MenuItemNewWithLabel(i18n.T("Создать новую папку (F7)...", "Create new folder (F7)..."))
	mMkdir.Connect("activate", func() {
		sp.showCreateFolderDialog()
	})
	menu.Append(mMkdir)

	mRefresh, _ := gtk.MenuItemNewWithLabel(i18n.T("Обновить каталог (F5)", "Refresh directory (F5)"))
	mRefresh.Connect("activate", func() {
		sp.LoadDirectory(sp.client.CurrentDir())
	})
	menu.Append(mRefresh)

	menu.ShowAll()
	menu.PopupAtPointer(nil)
}

func (sp *SFTPPanel) showEmptyAreaContextMenu(eventTime uint32) {
	menu, _ := gtk.MenuNew()

	mUpload, _ := gtk.MenuItemNewWithLabel(i18n.T("Выгрузить файл на сервер (Upload)...", "Upload file to server..."))
	mUpload.Connect("activate", func() {
		sp.showUploadFileChooser()
	})
	menu.Append(mUpload)

	mMkdir, _ := gtk.MenuItemNewWithLabel(i18n.T("Создать новую папку (F7)...", "Create new folder (F7)..."))
	mMkdir.Connect("activate", func() {
		sp.showCreateFolderDialog()
	})
	menu.Append(mMkdir)

	mRefresh, _ := gtk.MenuItemNewWithLabel(i18n.T("Обновить каталог (F5)", "Refresh directory (F5)"))
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

func (sp *SFTPPanel) showError(title, msg string) {
	dlg := gtk.MessageDialogNew(nil, gtk.DIALOG_MODAL, gtk.MESSAGE_ERROR, gtk.BUTTONS_OK, "%s", msg)
	dlg.SetTitle(title)
	dlg.Run()
	dlg.Destroy()
}

// UpdateTheme updates SFTP button icons based on dark/light mode
func (p *SFTPPanel) UpdateTheme(isDark bool) {
	if p == nil {
		return
	}
	if p.UploadBtn != nil {
		p.UploadBtn.SetImage(GetUploadImage(isDark))
		p.UploadBtn.ShowAll()
	}
	if p.DownloadBtn != nil {
		p.DownloadBtn.SetImage(GetDownloadImage(isDark))
		p.DownloadBtn.ShowAll()
	}
}
