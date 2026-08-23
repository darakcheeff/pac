package ui

import (
	"fmt"

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
		Box:       box,
		TreeView:  treeView,
		TreeStore: treeStore,
		store:     store,
	}

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
			}
		}
		return false
	})

	ht.Reload()
	return ht, nil
}

// Reload populates the tree from database
func (ht *HostTree) Reload() {
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
		_ = ht.TreeStore.SetValue(iter, ColName, g.Name)
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
}

func (ht *HostTree) showContextMenu(iter *gtk.TreeIter, eventTime uint32) {
	valType, _ := ht.TreeStore.GetValue(iter, ColType)
	typeStr, _ := valType.GetString()
	valID, _ := ht.TreeStore.GetValue(iter, ColID)
	idStr, _ := valID.GetString()

	menu, _ := gtk.MenuNew()

	if typeStr == "host" {
		host, err := ht.store.GetHost(idStr)
		if err != nil {
			return
		}

		// Connect item
		mConnect, _ := gtk.MenuItemNewWithLabel("Подключиться")
		mConnect.Connect("activate", func() {
			if ht.OnConnectHost != nil {
				ht.OnConnectHost(host)
			}
		})
		menu.Append(mConnect)

		// Edit item
		mEdit, _ := gtk.MenuItemNewWithLabel("Свойства / Редактировать")
		mEdit.Connect("activate", func() {
			if ht.OnEditHost != nil {
				ht.OnEditHost(host)
			}
		})
		menu.Append(mEdit)

		// Duplicate item
		mDup, _ := gtk.MenuItemNewWithLabel("Дублировать хост")
		mDup.Connect("activate", func() {
			dup := *host
			dup.ID = fmt.Sprintf("host-%d", eventTime)
			dup.Name = host.Name + " (копия)"
			_ = ht.store.SaveHost(&dup)
			ht.Reload()
		})
		menu.Append(mDup)

		sep, _ := gtk.SeparatorMenuItemNew()
		menu.Append(sep)

		// Delete item
		mDel, _ := gtk.MenuItemNewWithLabel("Удалить")
		mDel.Connect("activate", func() {
			_ = ht.store.DeleteHost(host.ID)
			ht.Reload()
		})
		menu.Append(mDel)

	} else {
		// Group context menu
		mAddHost, _ := gtk.MenuItemNewWithLabel("Добавить новое подключение")
		mAddHost.Connect("activate", func() {
			if ht.OnAddHost != nil {
				ht.OnAddHost(idStr)
			}
		})
		menu.Append(mAddHost)

		mAddGroup, _ := gtk.MenuItemNewWithLabel("Создать подпапку")
		mAddGroup.Connect("activate", func() {
			if ht.OnAddGroup != nil {
				ht.OnAddGroup(idStr)
			}
		})
		menu.Append(mAddGroup)

		if idStr != "root" {
			sep, _ := gtk.SeparatorMenuItemNew()
			menu.Append(sep)

			mDelGroup, _ := gtk.MenuItemNewWithLabel("Удалить папку")
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
