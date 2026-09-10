package ui

import (
	"encoding/json"
	"fmt"

	"github.com/darakcheeff/pac/internal/i18n"
	"github.com/darakcheeff/pac/internal/storage"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
	"github.com/gotk3/gotk3/pango"
)

const (
	ColID       = 0
	ColName     = 1
	ColIcon     = 2
	ColType     = 3 // "group" or "host"
	ColProtocol = 4
)

// HostTree represents the left-side host and folder navigation tree
type HostTree struct {
	Box           *gtk.Box
	TreeView      *gtk.TreeView
	TreeStore     *gtk.TreeStore
	store         *storage.Store
	OnConnectHost func(host *storage.Host)
	OnEditHost    func(host *storage.Host)
	OnDeleteHost  func(hostID string)
	OnAddHost     func(parentGroupID string)
	OnAddGroup    func(parentGroupID string)
	OnRenameGroup func(groupID string)
	OnImportOld   func()

	collapsedGroups map[string]bool
	isReloading     bool
}

func NewHostTree(store *storage.Store) (*HostTree, error) {
	box, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 2)
	if err != nil {
		return nil, err
	}
	box.SetSizeRequest(10, -1)

	// TreeStore: ID (string), Name (string), Icon (string), Type (string), Protocol (string)
	treeStore, err := gtk.TreeStoreNew(glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING)
	if err != nil {
		return nil, err
	}

	treeView, err := gtk.TreeViewNewWithModel(treeStore)
	if err != nil {
		return nil, err
	}
	treeView.SetHeadersVisible(false)
	treeView.SetEnableTreeLines(true)

	// Column: Icon + Name
	col, _ := gtk.TreeViewColumnNew()
	rPixbuf, _ := gtk.CellRendererPixbufNew()
	rText, _ := gtk.CellRendererTextNew()

	col.PackStart(rPixbuf, false)
	col.AddAttribute(rPixbuf, "icon-name", ColIcon)
	_ = rText.SetProperty("ellipsize", pango.ELLIPSIZE_END)
	col.PackStart(rText, true)
	col.AddAttribute(rText, "text", ColName)

	treeView.AppendColumn(col)

	// Scroll container
	scrolled, _ := gtk.ScrolledWindowNew(nil, nil)
	scrolled.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	scrolled.SetShadowType(gtk.SHADOW_IN)
	scrolled.Add(treeView)
	box.PackStart(scrolled, true, true, 0)

	ht := &HostTree{
		Box:             box,
		TreeView:        treeView,
		TreeStore:       treeStore,
		store:           store,
		collapsedGroups: make(map[string]bool),
	}

	if val, err := store.GetSetting("tree_collapsed_groups"); err == nil && val != "" {
		_ = json.Unmarshal([]byte(val), &ht.collapsedGroups)
	}

	treeView.Connect("row-collapsed", func(tv *gtk.TreeView, iter *gtk.TreeIter, path *gtk.TreePath) {
		if ht.isReloading {
			return
		}
		valType, _ := ht.TreeStore.GetValue(iter, ColType)
		typeStr, _ := valType.GetString()
		if typeStr == "group" {
			valID, _ := ht.TreeStore.GetValue(iter, ColID)
			groupID, _ := valID.GetString()
			if groupID != "" {
				ht.collapsedGroups[groupID] = true
				ht.saveCollapsedState()
			}
		}
	})

	treeView.Connect("row-expanded", func(tv *gtk.TreeView, iter *gtk.TreeIter, path *gtk.TreePath) {
		if ht.isReloading {
			return
		}
		valType, _ := ht.TreeStore.GetValue(iter, ColType)
		typeStr, _ := valType.GetString()
		if typeStr == "group" {
			valID, _ := ht.TreeStore.GetValue(iter, ColID)
			groupID, _ := valID.GetString()
			if groupID != "" {
				delete(ht.collapsedGroups, groupID)
				ht.saveCollapsedState()
			}
		}
	})

	// Double click to connect
	treeView.Connect("row-activated", func(tv *gtk.TreeView, path *gtk.TreePath, column *gtk.TreeViewColumn) {
		iter, err := treeStore.GetIter(path)
		if err != nil {
			return
		}
		valType, _ := treeStore.GetValue(iter, ColType)
		typeStr, _ := valType.GetString()
		if typeStr == "host" {
			valID, _ := treeStore.GetValue(iter, ColID)
			hostID, _ := valID.GetString()
			host, err := store.GetHost(hostID)
			if err == nil && ht.OnConnectHost != nil {
				ht.OnConnectHost(host)
			}
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
				iter, _ := treeStore.GetIter(path)
				ht.showContextMenu(iter, btnEvent.Time())
				return true
			} else {
				ht.showContextMenu(nil, btnEvent.Time())
				return true
			}
		}
		return false
	})

	ht.Reload()
	return ht, nil
}

func (ht *HostTree) saveCollapsedState() {
	if data, err := json.Marshal(ht.collapsedGroups); err == nil {
		_ = ht.store.SaveSetting("tree_collapsed_groups", string(data))
	}
}

// Reload populates the tree from database
func (ht *HostTree) Reload() {
	ht.isReloading = true
	defer func() {
		ht.isReloading = false
	}()

	ht.TreeStore.Clear()

	groups, err := ht.store.GetAllGroups()
	if err != nil {
		return
	}
	hosts, err := ht.store.GetAllHosts()
	if err != nil {
		return
	}

	groupMap := make(map[string]*gtk.TreeIter)

	// Add groups
	for _, g := range groups {
		var parentIter *gtk.TreeIter
		if g.ParentID != "" && g.ParentID != "root" {
			parentIter = groupMap[g.ParentID]
		}
		iter := ht.TreeStore.Append(parentIter)
		icon := "folder"
		if g.Icon != "" {
			icon = g.Icon
		}
		_ = ht.TreeStore.SetValue(iter, ColID, g.ID)
		groupName := g.Name
		if g.ID == "root" {
			groupName = i18n.T("Все подключения", "All Connections")
		}
		_ = ht.TreeStore.SetValue(iter, ColName, groupName)
		_ = ht.TreeStore.SetValue(iter, ColIcon, icon)
		_ = ht.TreeStore.SetValue(iter, ColType, "group")
		_ = ht.TreeStore.SetValue(iter, ColProtocol, "")
		groupMap[g.ID] = iter
	}

	// Add hosts
	for _, h := range hosts {
		parentIter := groupMap[h.GroupID]
		if parentIter == nil {
			parentIter = groupMap["root"]
		}

		iter := ht.TreeStore.Append(parentIter)
		icon := "utilities-terminal"
		switch h.Protocol {
		case storage.ProtoSSH:
			icon = "network-server"
		case storage.ProtoSFTP:
			icon = "folder-remote"
		case storage.ProtoTelnet:
			icon = "network-workgroup"
		case storage.ProtoSerial:
			icon = "media-flash"
		case storage.ProtoLocal:
			icon = "utilities-terminal"
		}

		_ = ht.TreeStore.SetValue(iter, ColID, h.ID)
		_ = ht.TreeStore.SetValue(iter, ColName, h.Name)
		_ = ht.TreeStore.SetValue(iter, ColIcon, icon)
		_ = ht.TreeStore.SetValue(iter, ColType, "host")
		_ = ht.TreeStore.SetValue(iter, ColProtocol, string(h.Protocol))
	}

	ht.TreeView.ExpandAll()

	// Reapply collapsed state
	for gid, collapsed := range ht.collapsedGroups {
		if collapsed {
			if iter, ok := groupMap[gid]; ok && iter != nil {
				if path, err := ht.TreeStore.GetPath(iter); err == nil && path != nil {
					ht.TreeView.CollapseRow(path)
				}
			}
		}
	}
}

func (ht *HostTree) showContextMenu(iter *gtk.TreeIter, eventTime uint32) {
	typeStr := "group"
	idStr := "root"
	if iter != nil {
		if valType, err := ht.TreeStore.GetValue(iter, ColType); err == nil && valType != nil {
			typeStr, _ = valType.GetString()
		}
		if valID, err := ht.TreeStore.GetValue(iter, ColID); err == nil && valID != nil {
			idStr, _ = valID.GetString()
		}
	}

	menu, _ := gtk.MenuNew()

	if typeStr == "host" {
		host, err := ht.store.GetHost(idStr)
		if err != nil {
			return
		}

		// Connect item
		mConnect, _ := gtk.MenuItemNewWithLabel(i18n.T("Подключиться", "Connect"))
		mConnect.Connect("activate", func() {
			if ht.OnConnectHost != nil {
				ht.OnConnectHost(host)
			}
		})
		menu.Append(mConnect)

		// Edit item
		mEdit, _ := gtk.MenuItemNewWithLabel(i18n.T("Свойства / Редактировать", "Properties / Edit"))
		mEdit.Connect("activate", func() {
			if ht.OnEditHost != nil {
				ht.OnEditHost(host)
			}
		})
		menu.Append(mEdit)

		// Duplicate item
		mDup, _ := gtk.MenuItemNewWithLabel(i18n.T("Дублировать хост", "Duplicate Host"))
		mDup.Connect("activate", func() {
			dup := *host
			dup.ID = fmt.Sprintf("host-%d", eventTime)
			dup.Name = host.Name + i18n.T(" (копия)", " (copy)")
			_ = ht.store.SaveHost(&dup)
			ht.Reload()
		})
		menu.Append(mDup)

		sep, _ := gtk.SeparatorMenuItemNew()
		menu.Append(sep)

		// Delete item
		mDel, _ := gtk.MenuItemNewWithLabel(i18n.T("Удалить", "Delete"))
		mDel.Connect("activate", func() {
			_ = ht.store.DeleteHost(host.ID)
			ht.Reload()
		})
		menu.Append(mDel)

	} else {
		// Group context menu
		mAddHost, _ := gtk.MenuItemNewWithLabel(i18n.T("Добавить новое подключение", "Add New Connection"))
		mAddHost.Connect("activate", func() {
			if ht.OnAddHost != nil {
				ht.OnAddHost(idStr)
			}
		})
		menu.Append(mAddHost)

		groupLabel := i18n.T("Создать подпапку", "Create Subfolder")
		if idStr == "root" {
			groupLabel = i18n.T("Создать папку", "Create Folder")
		}
		mAddGroup, _ := gtk.MenuItemNewWithLabel(groupLabel)
		mAddGroup.Connect("activate", func() {
			if ht.OnAddGroup != nil {
				ht.OnAddGroup(idStr)
			}
		})
		menu.Append(mAddGroup)

		sepImp, _ := gtk.SeparatorMenuItemNew()
		menu.Append(sepImp)

		mImportOld, _ := gtk.MenuItemNewWithLabel(i18n.T("Импорт из Ásbrú v6 (asbru.conf)...", "Import from Ásbrú v6 (asbru.conf)..."))
		mImportOld.Connect("activate", func() {
			if ht.OnImportOld != nil {
				ht.OnImportOld()
			}
		})
		menu.Append(mImportOld)

		if idStr != "root" {
			sep, _ := gtk.SeparatorMenuItemNew()
			menu.Append(sep)

			mRenameGroup, _ := gtk.MenuItemNewWithLabel(i18n.T("Переименовать папку", "Rename Folder"))
			mRenameGroup.Connect("activate", func() {
			if ht.OnRenameGroup != nil {
				ht.OnRenameGroup(idStr)
			}
			})
			menu.Append(mRenameGroup)

			mDelGroup, _ := gtk.MenuItemNewWithLabel(i18n.T("Удалить папку", "Delete Folder"))
			mDelGroup.Connect("activate", func() {
				_ = ht.store.DeleteGroup(idStr)
				ht.Reload()
			})
			menu.Append(mDelGroup)
		}
	}

	menu.ShowAll()
	menu.PopupAtPointer(nil)
}
