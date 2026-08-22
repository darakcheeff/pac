package ui

import (
	"github.com/darakcheeff/pac/internal/session"
	"github.com/darakcheeff/pac/internal/ui/vte"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

// TabItem represents one open session tab inside the notebook
type TabItem struct {
	ID         string
	Session    *session.Session
	Terminal   *vte.Terminal
	Label      *gtk.Label
	TabBox     *gtk.Box
	ContentBox *gtk.Box
	Search     *SearchBar
}

// TabView manages notebook tabs and splits
type TabView struct {
	Notebook         *gtk.Notebook
	tabs             map[int]*TabItem
	OnTabChanged     func(sess *session.Session)
	OnTabClosed      func(sess *session.Session)
	OnSplitRequested func(sess *session.Session, vertical bool)
}

func NewTabView() (*TabView, error) {
	nb, err := gtk.NotebookNew()
	if err != nil {
		return nil, err
	}
	nb.SetScrollable(true)
	nb.SetShowBorder(true)

	tv := &TabView{
		Notebook: nb,
		tabs:     make(map[int]*TabItem),
	}

	nb.Connect("switch-page", func(_ *gtk.Notebook, page *gtk.Widget, pageNum uint) {
		item, exists := tv.tabs[int(pageNum)]
		if exists && tv.OnTabChanged != nil {
			tv.OnTabChanged(item.Session)
		}
	})

	return tv, nil
}

// AddTab creates a new tab with custom header (title, close btn, rename, dnd)
func (tv *TabView) AddTab(sess *session.Session, term *vte.Terminal) (*TabItem, error) {
	contentBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}

	searchBar, _ := NewSearchBar()
	searchBar.SetActiveTerminal(term)
	contentBox.PackStart(searchBar.Box, false, false, 0)
	contentBox.PackStart(term.Widget, true, true, 0)

	// Custom Tab Header Box
	tabBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	icon, _ := gtk.ImageNewFromIconName("utilities-terminal-symbolic", gtk.ICON_SIZE_MENU)
	tabBox.PackStart(icon, false, false, 0)

	titleLabel, _ := gtk.LabelNew(sess.Title)
	tabBox.PackStart(titleLabel, true, true, 0)

	closeBtn, _ := gtk.ButtonNewFromIconName("window-close-symbolic", gtk.ICON_SIZE_MENU)
	closeBtn.SetRelief(gtk.RELIEF_NONE)
	closeBtn.SetTooltipText("Закрыть вкладку (Ctrl+W)")
	tabBox.PackEnd(closeBtn, false, false, 0)

	tabBox.ShowAll()
	contentBox.ShowAll()
	searchBar.Hide()

	pageNum := tv.Notebook.AppendPage(contentBox, tabBox)
	tv.Notebook.SetTabReorderable(contentBox, true)

	item := &TabItem{
		ID:         sess.ID,
		Session:    sess,
		Terminal:   term,
		Label:      titleLabel,
		TabBox:     tabBox,
		ContentBox: contentBox,
		Search:     searchBar,
	}
	tv.tabs[pageNum] = item

	// Close tab button action
	closeBtn.Connect("clicked", func() {
		tv.CloseTab(item)
	})

	// Double click to inline rename
	tabBox.Connect("button-press-event", func(_ *gtk.Box, event *gdk.Event) bool {
		btnEvent := gdk.EventButtonNewFromEvent(event)
		if btnEvent.Type() == gdk.EVENT_2BUTTON_PRESS {
			tv.showRenameDialog(item)
			return true
		} else if btnEvent.Button() == gdk.BUTTON_SECONDARY {
			tv.showTabContextMenu(item, btnEvent.Time())
			return true
		}
		return false
	})

	// Context menu on terminal right click
	term.Widget.Connect("button-press-event", func(_ *gtk.Widget, event *gdk.Event) bool {
		btnEvent := gdk.EventButtonNewFromEvent(event)
		if btnEvent.Button() == gdk.BUTTON_SECONDARY {
			tv.showTerminalContextMenu(item, btnEvent.Time())
			return true
		}
		return false
	})

	tv.Notebook.SetCurrentPage(pageNum)
	return item, nil
}

// CloseTab closes tab and session
func (tv *TabView) CloseTab(item *TabItem) {
	pageNum := tv.Notebook.PageNum(item.ContentBox)
	if pageNum >= 0 {
		tv.Notebook.RemovePage(pageNum)
		delete(tv.tabs, pageNum)
	}

	if tv.OnTabClosed != nil {
		tv.OnTabClosed(item.Session)
	}
}

// GetCurrentTab returns active TabItem
func (tv *TabView) GetCurrentTab() *TabItem {
	curPage := tv.Notebook.GetCurrentPage()
	if curPage >= 0 {
		return tv.tabs[curPage]
	}
	return nil
}

func (tv *TabView) showRenameDialog(item *TabItem) {
	dlg, _ := gtk.DialogNew()
	dlg.SetTitle("Переименовать вкладку")
	dlg.SetModal(true)
	dlg.SetDefaultSize(300, 100)

	contentArea, _ := dlg.GetContentArea()
	entry, _ := gtk.EntryNew()
	entry.SetText(item.Session.Title)
	contentArea.Add(entry)

	_, _ = dlg.AddButton("Отмена", gtk.RESPONSE_CANCEL)
	_, _ = dlg.AddButton("Сохранить", gtk.RESPONSE_OK)
	dlg.ShowAll()

	if dlg.Run() == gtk.RESPONSE_OK {
		newTitle, _ := entry.GetText()
		if newTitle != "" {
			item.Session.Title = newTitle
			item.Label.SetText(newTitle)
		}
	}
	dlg.Destroy()
}

func (tv *TabView) showTabContextMenu(item *TabItem, eventTime uint32) {
	menu, _ := gtk.MenuNew()

	mRename, _ := gtk.MenuItemNewWithLabel("Переименовать вкладку...")
	mRename.Connect("activate", func() {
		tv.showRenameDialog(item)
	})
	menu.Append(mRename)

	mSplitH, _ := gtk.MenuItemNewWithLabel("Разделить по горизонтали")
	mSplitH.Connect("activate", func() {
		if tv.OnSplitRequested != nil {
			tv.OnSplitRequested(item.Session, false)
		}
	})
	menu.Append(mSplitH)

	mSplitV, _ := gtk.MenuItemNewWithLabel("Разделить по вертикали")
	mSplitV.Connect("activate", func() {
		if tv.OnSplitRequested != nil {
			tv.OnSplitRequested(item.Session, true)
		}
	})
	menu.Append(mSplitV)

	sep, _ := gtk.SeparatorMenuItemNew()
	menu.Append(sep)

	mClose, _ := gtk.MenuItemNewWithLabel("Закрыть вкладку")
	mClose.Connect("activate", func() {
		tv.CloseTab(item)
	})
	menu.Append(mClose)

	menu.ShowAll()
	menu.PopupAtPointer(nil)
}

func (tv *TabView) showTerminalContextMenu(item *TabItem, eventTime uint32) {
	menu, _ := gtk.MenuNew()

	mCopy, _ := gtk.MenuItemNewWithLabel("Копировать")
	mCopy.Connect("activate", func() {
		item.Terminal.CopyClipboard()
	})
	menu.Append(mCopy)

	mPaste, _ := gtk.MenuItemNewWithLabel("Вставить")
	mPaste.Connect("activate", func() {
		item.Terminal.PasteClipboard()
	})
	menu.Append(mPaste)

	mSelectAll, _ := gtk.MenuItemNewWithLabel("Выделить всё")
	mSelectAll.Connect("activate", func() {
		item.Terminal.SelectAll()
	})
	menu.Append(mSelectAll)

	sep, _ := gtk.SeparatorMenuItemNew()
	menu.Append(sep)

	mFind, _ := gtk.MenuItemNewWithLabel("Найти в терминале (Ctrl+Shift+F)")
	mFind.Connect("activate", func() {
		item.Search.Show()
	})
	menu.Append(mFind)

	mReset, _ := gtk.MenuItemNewWithLabel("Сброс и очистка истории")
	mReset.Connect("activate", func() {
		item.Terminal.Reset(true)
	})
	menu.Append(mReset)

	menu.ShowAll()
	menu.PopupAtPointer(nil)
}
