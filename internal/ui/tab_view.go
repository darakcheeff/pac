package ui

import (
	"github.com/darakcheeff/pac/internal/session"
	"github.com/darakcheeff/pac/internal/ui/vte"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

// SplitPane represents a split terminal container inside a tab
type SplitPane struct {
	Session  *session.Session
	Terminal *vte.Terminal
	Box      *gtk.Box
	Search   *SearchBar
}

// TabItem represents one open session tab inside the notebook
type TabItem struct {
	ID         string
	Session    *session.Session
	Terminal   *vte.Terminal
	Label      *gtk.Label
	TabBox     *gtk.Box
	EventBox   *gtk.EventBox
	ContentBox *gtk.Box
	Search     *SearchBar
	Paned      *gtk.Paned
	SplitChild *SplitPane
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
	nb.PopupEnable()

	tv := &TabView{
		Notebook: nb,
		tabs:     make(map[int]*TabItem),
	}

	nb.Connect("switch-page", func(_ *gtk.Notebook, page *gtk.Widget, pageNum uint) {
		item, exists := tv.tabs[int(pageNum)]
		if exists {
			if item.Terminal != nil {
				item.Terminal.GrabFocus()
			}
			if tv.OnTabChanged != nil {
				tv.OnTabChanged(item.Session)
			}
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

	// EventBox wrapper for tab header to receive click & right-click events
	eventBox, _ := gtk.EventBoxNew()
	eventBox.SetEvents(int(gdk.BUTTON_PRESS_MASK | gdk.BUTTON_RELEASE_MASK))

	tabBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	tabBox.SetMarginStart(2)
	tabBox.SetMarginEnd(2)

	icon, _ := gtk.ImageNewFromIconName("utilities-terminal-symbolic", gtk.ICON_SIZE_MENU)
	tabBox.PackStart(icon, false, false, 0)

	titleLabel, _ := gtk.LabelNew(sess.Title)
	tabBox.PackStart(titleLabel, true, true, 0)

	closeBtn, _ := gtk.ButtonNewFromIconName("window-close-symbolic", gtk.ICON_SIZE_MENU)
	closeBtn.SetRelief(gtk.RELIEF_NONE)
	closeBtn.SetTooltipText("Закрыть вкладку (Ctrl+W)")
	tabBox.PackEnd(closeBtn, false, false, 0)

	eventBox.Add(tabBox)
	eventBox.ShowAll()
	contentBox.ShowAll()
	searchBar.Hide()

	pageNum := tv.Notebook.AppendPage(contentBox, eventBox)
	tv.Notebook.SetTabReorderable(contentBox, true)

	item := &TabItem{
		ID:         sess.ID,
		Session:    sess,
		Terminal:   term,
		Label:      titleLabel,
		TabBox:     tabBox,
		EventBox:   eventBox,
		ContentBox: contentBox,
		Search:     searchBar,
	}
	tv.tabs[pageNum] = item

	// Close tab button action
	closeBtn.Connect("clicked", func() {
		tv.CloseTab(item)
	})

	// Header click actions on EventBox (Left Double Click = Rename, Right Click = Tab Menu)
	eventBox.Connect("button-press-event", func(_ *gtk.EventBox, event *gdk.Event) bool {
		btnEvent := gdk.EventButtonNewFromEvent(event)
		if btnEvent.Type() == gdk.EVENT_2BUTTON_PRESS && btnEvent.Button() == gdk.BUTTON_PRIMARY {
			tv.showRenameDialog(item)
			return true
		} else if btnEvent.Button() == gdk.BUTTON_SECONDARY {
			tv.showTabContextMenu(item, btnEvent.Time())
			return true
		}
		return false
	})

	// Terminal right-click context menu
	term.Widget.Connect("button-press-event", func(_ *glib.Object, event *gdk.Event) bool {
		btnEvent := gdk.EventButtonNewFromEvent(event)
		if btnEvent.Button() == gdk.BUTTON_SECONDARY {
			tv.showTerminalContextMenu(item, btnEvent.Time())
			return true
		}
		return false
	})

	tv.Notebook.SetCurrentPage(pageNum)
	term.GrabFocus()
	return item, nil
}

// SplitActiveTab splits current tab into two terminals (horizontal or vertical)
func (tv *TabView) SplitActiveTab(item *TabItem, newSess *session.Session, newTerm *vte.Terminal, vertical bool) error {
	if item == nil || item.ContentBox == nil {
		return nil
	}

	// Create paned container
	orientation := gtk.ORIENTATION_HORIZONTAL
	if !vertical {
		orientation = gtk.ORIENTATION_VERTICAL
	}

	paned, err := gtk.PanedNew(orientation)
	if err != nil {
		return err
	}

	// Remove term from contentBox
	item.ContentBox.Remove(item.Terminal.Widget)

	// Create SplitChild
	splitSearch, _ := NewSearchBar()
	splitSearch.SetActiveTerminal(newTerm)
	splitBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	splitBox.PackStart(splitSearch.Box, false, false, 0)
	splitBox.PackStart(newTerm.Widget, true, true, 0)
	splitSearch.Hide()

	// Pack original terminal in Pack1 and new terminal in Pack2
	paned.Pack1(item.Terminal.Widget, true, false)
	paned.Pack2(splitBox, true, false)
	paned.SetPosition(400)

	item.ContentBox.PackStart(paned, true, true, 0)
	item.Paned = paned
	item.SplitChild = &SplitPane{
		Session:  newSess,
		Terminal: newTerm,
		Box:      splitBox,
		Search:   splitSearch,
	}

	newTerm.Widget.Connect("button-press-event", func(_ *glib.Object, event *gdk.Event) bool {
		btnEvent := gdk.EventButtonNewFromEvent(event)
		if btnEvent.Button() == gdk.BUTTON_SECONDARY {
			tv.showSplitContextMenu(item, btnEvent.Time())
			return true
		}
		return false
	})

	item.ContentBox.ShowAll()
	splitSearch.Hide()
	newTerm.GrabFocus()
	return nil
}

// UnsplitTab restores tab to single terminal and detaches split child into its own tab
func (tv *TabView) UnsplitTab(item *TabItem) {
	if item == nil || item.Paned == nil || item.SplitChild == nil {
		return
	}

	childSess := item.SplitChild.Session
	childTerm := item.SplitChild.Terminal

	// Remove paned and restore single terminal
	item.ContentBox.Remove(item.Paned)
	item.ContentBox.PackStart(item.Terminal.Widget, true, true, 0)
	item.Paned = nil
	item.SplitChild = nil
	item.ContentBox.ShowAll()

	// Open child as new tab
	_, _ = tv.AddTab(childSess, childTerm)
}

// CloseSplit closes split pane without closing original tab
func (tv *TabView) CloseSplit(item *TabItem) {
	if item == nil || item.Paned == nil || item.SplitChild == nil {
		return
	}

	childSess := item.SplitChild.Session
	item.ContentBox.Remove(item.Paned)
	item.ContentBox.PackStart(item.Terminal.Widget, true, true, 0)
	item.Paned = nil
	item.SplitChild = nil
	item.ContentBox.ShowAll()

	if childSess != nil {
		_ = childSess.Close()
	}
}

// CloseTab closes tab and session
func (tv *TabView) CloseTab(item *TabItem) {
	pageNum := tv.Notebook.PageNum(item.ContentBox)
	if pageNum >= 0 {
		tv.Notebook.RemovePage(pageNum)
		delete(tv.tabs, pageNum)
	}

	if item.SplitChild != nil && item.SplitChild.Session != nil {
		_ = item.SplitChild.Session.Close()
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
	dlg.SetDefaultSize(320, 120)

	contentArea, _ := dlg.GetContentArea()
	vbox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 6)
	vbox.SetMarginStart(12)
	vbox.SetMarginEnd(12)
	vbox.SetMarginTop(12)
	vbox.SetMarginBottom(12)

	lbl, _ := gtk.LabelNew("Новое имя вкладки:")
	lbl.SetHAlign(gtk.ALIGN_START)
	vbox.PackStart(lbl, false, false, 0)

	entry, _ := gtk.EntryNew()
	entry.SetText(item.Session.Title)
	entry.SetActivatesDefault(true)
	vbox.PackStart(entry, false, false, 0)

	contentArea.Add(vbox)

	_, _ = dlg.AddButton("Отмена", gtk.RESPONSE_CANCEL)
	btnSave, _ := dlg.AddButton("Сохранить", gtk.RESPONSE_OK)
	btnSave.SetCanDefault(true)
	dlg.SetDefault(btnSave)

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

	mRename, _ := gtk.MenuItemNewWithLabel("Переименовать вкладку (F2)")
	mRename.Connect("activate", func() {
		tv.showRenameDialog(item)
	})
	menu.Append(mRename)

	sep1, _ := gtk.SeparatorMenuItemNew()
	menu.Append(sep1)

	mSplitH, _ := gtk.MenuItemNewWithLabel("Разделить экран по горизонтали (Split Horizontal)")
	mSplitH.Connect("activate", func() {
		if tv.OnSplitRequested != nil {
			tv.OnSplitRequested(item.Session, false)
		}
	})
	menu.Append(mSplitH)

	mSplitV, _ := gtk.MenuItemNewWithLabel("Разделить экран по вертикали (Split Vertical)")
	mSplitV.Connect("activate", func() {
		if tv.OnSplitRequested != nil {
			tv.OnSplitRequested(item.Session, true)
		}
	})
	menu.Append(mSplitV)

	if item.Paned != nil && item.SplitChild != nil {
		mUnsplit, _ := gtk.MenuItemNewWithLabel("Разгруппировать / Вынести сплит в новую вкладку")
		mUnsplit.Connect("activate", func() {
			tv.UnsplitTab(item)
		})
		menu.Append(mUnsplit)

		mCloseSplit, _ := gtk.MenuItemNewWithLabel("Закрыть соседний сплит")
		mCloseSplit.Connect("activate", func() {
			tv.CloseSplit(item)
		})
		menu.Append(mCloseSplit)
	}

	sep2, _ := gtk.SeparatorMenuItemNew()
	menu.Append(sep2)

	mClose, _ := gtk.MenuItemNewWithLabel("Закрыть вкладку (Ctrl+W)")
	mClose.Connect("activate", func() {
		tv.CloseTab(item)
	})
	menu.Append(mClose)

	menu.ShowAll()
	menu.PopupAtPointer(nil)
}

func (tv *TabView) showSplitContextMenu(item *TabItem, eventTime uint32) {
	menu, _ := gtk.MenuNew()

	mUnsplit, _ := gtk.MenuItemNewWithLabel("Разгруппировать / Вынести этот терминал в новую вкладку")
	mUnsplit.Connect("activate", func() {
		tv.UnsplitTab(item)
	})
	menu.Append(mUnsplit)

	mCloseSplit, _ := gtk.MenuItemNewWithLabel("Закрыть этот сплит")
	mCloseSplit.Connect("activate", func() {
		tv.CloseSplit(item)
	})
	menu.Append(mCloseSplit)

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

	if item.Paned != nil && item.SplitChild != nil {
		mUnsplit, _ := gtk.MenuItemNewWithLabel("Разгруппировать сплит в новую вкладку")
		mUnsplit.Connect("activate", func() {
			tv.UnsplitTab(item)
		})
		menu.Append(mUnsplit)
	}

	mReset, _ := gtk.MenuItemNewWithLabel("Сброс и очистка истории")
	mReset.Connect("activate", func() {
		item.Terminal.Reset(true)
	})
	menu.Append(mReset)

	menu.ShowAll()
	menu.PopupAtPointer(nil)
}
