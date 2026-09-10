package dialogs

import (
	"github.com/darakcheeff/pac/internal/i18n"
	"github.com/darakcheeff/pac/internal/storage"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

// ShowImportHostsDialog displays a modal list with checkboxes allowing the user to select which hosts to import
func ShowImportHostsDialog(parent *gtk.Window, hosts []*storage.Host, onConfirm func(selected []*storage.Host)) {
	dlg, err := gtk.DialogNew()
	if err != nil {
		return
	}
	dlg.SetTitle(i18n.T("Выбор хостов для импорта", "Select Hosts to Import"))
	dlg.SetTransientFor(parent)
	dlg.SetModal(true)
	dlg.SetDefaultSize(640, 440)

	contentArea, _ := dlg.GetContentArea()

	mainBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 8)
	mainBox.SetMarginStart(12)
	mainBox.SetMarginEnd(12)
	mainBox.SetMarginTop(12)
	mainBox.SetMarginBottom(8)

	lblHeader, _ := gtk.LabelNew(i18n.Tf("Найдено хостов для импорта: %d. Выберите подключения для импорта в папку asbru:",
		"Found %d hosts to import. Select connections to import into asbru folder:", len(hosts)))
	lblHeader.SetHAlign(gtk.ALIGN_START)
	lblHeader.SetLineWrap(true)
	mainBox.PackStart(lblHeader, false, false, 0)

	// ListStore: 0: Selected (bool), 1: Name (string), 2: Protocol (string), 3: Host (string), 4: User (string), 5: Index (int)
	listStore, _ := gtk.ListStoreNew(glib.TYPE_BOOLEAN, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_INT)

	for i, h := range hosts {
		iter := listStore.Append()
		_ = listStore.SetValue(iter, 0, true) // selected by default
		_ = listStore.SetValue(iter, 1, h.Name)
		_ = listStore.SetValue(iter, 2, string(h.Protocol))
		_ = listStore.SetValue(iter, 3, h.Host)
		_ = listStore.SetValue(iter, 4, h.Username)
		_ = listStore.SetValue(iter, 5, i)
	}

	treeView, _ := gtk.TreeViewNewWithModel(listStore)
	treeView.SetHeadersVisible(true)

	// Column 0: Checkbox
	colCheck, _ := gtk.TreeViewColumnNew()
	colCheck.SetTitle(i18n.T("Выбор", "Select"))
	rToggle, _ := gtk.CellRendererToggleNew()
	rToggle.SetActivatable(true)
	rToggle.Connect("toggled", func(_ *gtk.CellRendererToggle, pathStr string) {
		path, err := gtk.TreePathNewFromString(pathStr)
		if err == nil {
			iter, err := listStore.GetIter(path)
			if err == nil {
				val, _ := listStore.GetValue(iter, 0)
				goVal, _ := val.GoValue()
				curr, _ := goVal.(bool)
				_ = listStore.SetValue(iter, 0, !curr)
			}
		}
	})
	colCheck.PackStart(rToggle, false)
	colCheck.AddAttribute(rToggle, "active", 0)
	treeView.AppendColumn(colCheck)

	// Column 1: Name
	colName, _ := gtk.TreeViewColumnNew()
	colName.SetTitle(i18n.T("Имя", "Name"))
	rName, _ := gtk.CellRendererTextNew()
	colName.PackStart(rName, true)
	colName.AddAttribute(rName, "text", 1)
	treeView.AppendColumn(colName)

	// Column 2: Protocol
	colProto, _ := gtk.TreeViewColumnNew()
	colProto.SetTitle(i18n.T("Протокол", "Protocol"))
	rProto, _ := gtk.CellRendererTextNew()
	colProto.PackStart(rProto, false)
	colProto.AddAttribute(rProto, "text", 2)
	treeView.AppendColumn(colProto)

	// Column 3: Host / IP
	colHost, _ := gtk.TreeViewColumnNew()
	colHost.SetTitle(i18n.T("Хост / IP", "Host / IP"))
	rHost, _ := gtk.CellRendererTextNew()
	colHost.PackStart(rHost, true)
	colHost.AddAttribute(rHost, "text", 3)
	treeView.AppendColumn(colHost)

	// Column 4: User
	colUser, _ := gtk.TreeViewColumnNew()
	colUser.SetTitle(i18n.T("Пользователь", "User"))
	rUser, _ := gtk.CellRendererTextNew()
	colUser.PackStart(rUser, false)
	colUser.AddAttribute(rUser, "text", 4)
	treeView.AppendColumn(colUser)

	scrolled, _ := gtk.ScrolledWindowNew(nil, nil)
	scrolled.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	scrolled.SetShadowType(gtk.SHADOW_IN)
	scrolled.Add(treeView)
	mainBox.PackStart(scrolled, true, true, 0)

	// Select All / Deselect All buttons bar
	btnBar, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 6)
	btnSelectAll, _ := gtk.ButtonNewWithLabel(i18n.T("Выбрать все", "Select All"))
	btnSelectAll.Connect("clicked", func() {
		iter, valid := listStore.GetIterFirst()
		for valid {
			_ = listStore.SetValue(iter, 0, true)
			valid = listStore.IterNext(iter)
		}
	})
	btnBar.PackStart(btnSelectAll, false, false, 0)

	btnDeselectAll, _ := gtk.ButtonNewWithLabel(i18n.T("Снять выбор", "Deselect All"))
	btnDeselectAll.Connect("clicked", func() {
		iter, valid := listStore.GetIterFirst()
		for valid {
			_ = listStore.SetValue(iter, 0, false)
			valid = listStore.IterNext(iter)
		}
	})
	btnBar.PackStart(btnDeselectAll, false, false, 0)
	mainBox.PackStart(btnBar, false, false, 0)

	contentArea.Add(mainBox)

	_, _ = dlg.AddButton(i18n.T("Отмена", "Cancel"), gtk.RESPONSE_CANCEL)
	btnImport, _ := dlg.AddButton(i18n.T("Импортировать", "Import"), gtk.RESPONSE_OK)
	btnImport.SetCanDefault(true)
	dlg.SetDefault(btnImport)

	dlg.ShowAll()

	if dlg.Run() == gtk.RESPONSE_OK {
		var selected []*storage.Host
		iter, valid := listStore.GetIterFirst()
		for valid {
			valSel, _ := listStore.GetValue(iter, 0)
			goSel, _ := valSel.GoValue()
			if isSel, _ := goSel.(bool); isSel {
				valIdx, _ := listStore.GetValue(iter, 5)
				goIdx, _ := valIdx.GoValue()
				if idx, ok := goIdx.(int); ok && idx >= 0 && idx < len(hosts) {
					selected = append(selected, hosts[idx])
				}
			}
			valid = listStore.IterNext(iter)
		}

		if len(selected) > 0 && onConfirm != nil {
			onConfirm(selected)
		}
	}

	dlg.Destroy()
}
