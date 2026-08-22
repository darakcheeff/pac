package ui

import (
	"context"
	"fmt"

	"github.com/darakcheeff/pac/internal/session"
	"github.com/darakcheeff/pac/internal/ui/vte"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

// SearchBar represents the floating/in-tab find bar
type SearchBar struct {
	Box        *gtk.Box
	Entry      *gtk.SearchEntry
	PrevBtn    *gtk.Button
	NextBtn    *gtk.Button
	CaseCheck  *gtk.CheckButton
	CloseBtn   *gtk.Button
	activeTerm *vte.Terminal
}

func NewSearchBar() (*SearchBar, error) {
	box, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	if err != nil {
		return nil, err
	}
	box.SetMarginStart(6)
	box.SetMarginEnd(6)
	box.SetMarginTop(2)
	box.SetMarginBottom(2)

	entry, _ := gtk.SearchEntryNew()
	entry.SetPlaceholderText("Поиск в терминале...")
	entry.SetSizeRequest(220, -1)
	box.PackStart(entry, false, false, 0)

	prevBtn, _ := gtk.ButtonNewFromIconName("go-up-symbolic", gtk.ICON_SIZE_BUTTON)
	prevBtn.SetTooltipText("Предыдущее совпадение (Shift+Enter)")
	box.PackStart(prevBtn, false, false, 0)

	nextBtn, _ := gtk.ButtonNewFromIconName("go-down-symbolic", gtk.ICON_SIZE_BUTTON)
	nextBtn.SetTooltipText("Следующее совпадение (Enter)")
	box.PackStart(nextBtn, false, false, 0)

	caseCheck, _ := gtk.CheckButtonNewWithLabel("С учетом регистра")
	box.PackStart(caseCheck, false, false, 0)

	closeBtn, _ := gtk.ButtonNewFromIconName("window-close-symbolic", gtk.ICON_SIZE_BUTTON)
	closeBtn.SetTooltipText("Закрыть поиск (Escape)")
	box.PackEnd(closeBtn, false, false, 0)

	sb := &SearchBar{
		Box:       box,
		Entry:     entry,
		PrevBtn:   prevBtn,
		NextBtn:   nextBtn,
		CaseCheck: caseCheck,
		CloseBtn:  closeBtn,
	}

	updatePattern := func() {
		if sb.activeTerm == nil {
			return
		}
		text, _ := entry.GetText()
		caseSens := caseCheck.GetActive()
		sb.activeTerm.SearchSetPattern(text, caseSens)
		sb.activeTerm.SearchFindNext()
	}

	entry.Connect("search-changed", func() {
		updatePattern()
	})

	entry.Connect("activate", func() {
		if sb.activeTerm != nil {
			sb.activeTerm.SearchFindNext()
		}
	})

	prevBtn.Connect("clicked", func() {
		if sb.activeTerm != nil {
			sb.activeTerm.SearchFindPrevious()
		}
	})

	nextBtn.Connect("clicked", func() {
		if sb.activeTerm != nil {
			sb.activeTerm.SearchFindNext()
		}
	})

	caseCheck.Connect("toggled", func() {
		updatePattern()
	})

	closeBtn.Connect("clicked", func() {
		sb.Hide()
	})

	entry.Connect("key-press-event", func(_ *gtk.SearchEntry, event *gdk.Event) bool {
		keyEvent := gdk.EventKeyNewFromEvent(event)
		if keyEvent.KeyVal() == gdk.KEY_Escape {
			sb.Hide()
			return true
		}
		return false
	})

	return sb, nil
}

func (sb *SearchBar) SetActiveTerminal(term *vte.Terminal) {
	sb.activeTerm = term
}

func (sb *SearchBar) Show() {
	sb.Box.ShowAll()
	sb.Entry.GrabFocus()
}

func (sb *SearchBar) Hide() {
	if sb.activeTerm != nil {
		sb.activeTerm.SearchSetPattern("", false)
	}
	sb.Box.Hide()
}

// ShowGlobalSearchDialog shows multi-session search window
func ShowGlobalSearchDialog(parent *gtk.Window, manager *session.Manager, onSelectSession func(sessionID string)) {
	dlg, _ := gtk.DialogNew()
	dlg.SetTitle("Глобальный поиск по всем активным сессиям")
	dlg.SetTransientFor(parent)
	dlg.SetModal(true)
	dlg.SetDefaultSize(650, 420)

	contentArea, _ := dlg.GetContentArea()
	box, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 6)
	box.SetMarginStart(10)
	box.SetMarginEnd(10)
	box.SetMarginTop(10)
	box.SetMarginBottom(10)

	// Top search box
	topBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 6)
	entry, _ := gtk.SearchEntryNew()
	entry.SetPlaceholderText("Поиск строки во всех сессиях...")
	entry.SetHExpand(true)
	topBox.PackStart(entry, true, true, 0)

	caseCheck, _ := gtk.CheckButtonNewWithLabel("С учетом регистра")
	topBox.PackStart(caseCheck, false, false, 0)
	box.PackStart(topBox, false, false, 0)

	// Results TreeView
	listStore, _ := gtk.ListStoreNew(glib.TYPE_STRING, glib.TYPE_STRING, glib.TYPE_INT, glib.TYPE_STRING)
	treeView, _ := gtk.TreeViewNewWithModel(listStore)
	treeView.SetHeadersVisible(true)

	// Column 1: Host / Session
	rHost, _ := gtk.CellRendererTextNew()
	cHost, _ := gtk.TreeViewColumnNewWithAttribute("Сессия / Хост", rHost, "text", 1)
	cHost.SetResizable(true)
	cHost.SetMinWidth(150)
	treeView.AppendColumn(cHost)

	// Column 2: Line Number
	rLine, _ := gtk.CellRendererTextNew()
	cLine, _ := gtk.TreeViewColumnNewWithAttribute("Строка", rLine, "text", 2)
	cLine.SetMinWidth(60)
	treeView.AppendColumn(cLine)

	// Column 3: Matching Text
	rText, _ := gtk.CellRendererTextNew()
	cText, _ := gtk.TreeViewColumnNewWithAttribute("Найденный текст", rText, "text", 3)
	cText.SetResizable(true)
	cText.SetExpand(true)
	treeView.AppendColumn(cText)

	scrolled, _ := gtk.ScrolledWindowNew(nil, nil)
	scrolled.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	scrolled.SetShadowType(gtk.SHADOW_IN)
	scrolled.SetVExpand(true)
	scrolled.Add(treeView)
	box.PackStart(scrolled, true, true, 0)

	statusLabel, _ := gtk.LabelNew("Введите текст для поиска")
	statusLabel.SetHAlign(gtk.ALIGN_START)
	box.PackStart(statusLabel, false, false, 0)

	contentArea.Add(box)

	performSearch := func() {
		query, _ := entry.GetText()
		listStore.Clear()
		if query == "" {
			statusLabel.SetText("Введите текст для поиска")
			return
		}

		matches := manager.GlobalSearch(context.Background(), query, caseCheck.GetActive())
		for _, m := range matches {
			iter := listStore.Append()
			_ = listStore.SetValue(iter, 0, m.SessionID)
			_ = listStore.SetValue(iter, 1, m.HostName)
			_ = listStore.SetValue(iter, 2, m.LineNumber)
			_ = listStore.SetValue(iter, 3, m.LineText)
		}
		statusLabel.SetText(fmt.Sprintf("Найдено совпадений: %d", len(matches)))
	}

	entry.Connect("search-changed", func() {
		performSearch()
	})

	caseCheck.Connect("toggled", func() {
		performSearch()
	})

	treeView.Connect("row-activated", func(tv *gtk.TreeView, path *gtk.TreePath, column *gtk.TreeViewColumn) {
		iter, err := listStore.GetIter(path)
		if err == nil {
			val, _ := listStore.GetValue(iter, 0)
			sessID, _ := val.GetString()
			if sessID != "" && onSelectSession != nil {
				onSelectSession(sessID)
				dlg.Response(gtk.RESPONSE_OK)
			}
		}
	})

	_, _ = dlg.AddButton("Закрыть", gtk.RESPONSE_CLOSE)
	dlg.Connect("response", func(_ *gtk.Dialog, responseId gtk.ResponseType) {
		dlg.Destroy()
	})

	dlg.ShowAll()
	entry.GrabFocus()
}
