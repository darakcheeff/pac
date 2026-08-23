package ui

import (
	"log"

	"github.com/darakcheeff/pac/internal/session"
	"github.com/darakcheeff/pac/internal/ui/vte"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

// toPaned safely casts an IWidget to *gtk.Paned
func toPaned(iw gtk.IWidget) *gtk.Paned {
	if iw == nil {
		return nil
	}
	if p, ok := iw.(*gtk.Paned); ok {
		return p
	}
	return &gtk.Paned{
		Bin: gtk.Bin{
			Container: gtk.Container{
				Widget: *iw.ToWidget(),
			},
		},
	}
}

// areWidgetsEqual compares two IWidgets by underlying GObject pointer
func areWidgetsEqual(w1, w2 gtk.IWidget) bool {
	if w1 == nil || w2 == nil {
		return w1 == w2
	}
	return w1.ToWidget().GObject == w2.ToWidget().GObject
}

// TerminalPane represents an individual terminal pane inside a tab
type TerminalPane struct {
	Session  *session.Session
	Terminal *vte.Terminal
	Box      *gtk.Box
	Search   *SearchBar
	TabItem  *TabItem
}

// TabItem represents one open session tab inside the notebook (can hold multiple split panes)
type TabItem struct {
	ID          string
	Session     *session.Session
	Label       *gtk.Label
	TabBox      *gtk.Box
	EventBox    *gtk.EventBox
	ContentBox  *gtk.Box
	Panes       []*TerminalPane
	FocusedPane *TerminalPane
}

// TabView manages notebook tabs and terminal splits
type TabView struct {
	Notebook         *gtk.Notebook
	items            []*TabItem
	OnTabChanged     func(sess *session.Session)
	OnTabClosed      func(sess *session.Session)
	OnSplitRequested func(sess *session.Session, vertical bool)
}

// NewTabView initializes the GTK Notebook tab manager
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
		items:    make([]*TabItem, 0),
	}

	nb.Connect("switch-page", func(_ *gtk.Notebook, page *gtk.Widget, pageNum uint) {
		item := tv.GetCurrentTab()
		if item != nil {
			if item.FocusedPane != nil && item.FocusedPane.Terminal != nil {
				item.FocusedPane.Terminal.GrabFocus()
				if tv.OnTabChanged != nil {
					tv.OnTabChanged(item.FocusedPane.Session)
				}
			} else if len(item.Panes) > 0 && item.Panes[0].Terminal != nil {
				item.Panes[0].Terminal.GrabFocus()
				if tv.OnTabChanged != nil {
					tv.OnTabChanged(item.Panes[0].Session)
				}
			}
		}
	})

	return tv, nil
}

// createPane constructs a TerminalPane with its search bar and event listeners
func (tv *TabView) createPane(item *TabItem, sess *session.Session, term *vte.Terminal) *TerminalPane {
	box, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	box.SetHExpand(true)
	box.SetVExpand(true)

	searchBar, _ := NewSearchBar()
	searchBar.SetActiveTerminal(term)

	box.PackStart(searchBar.Box, false, false, 0)
	box.PackStart(term.Widget, true, true, 0)

	pane := &TerminalPane{
		Session:  sess,
		Terminal: term,
		Box:      box,
		Search:   searchBar,
		TabItem:  item,
	}

	// Focus and click handling
	term.Widget.Connect("button-press-event", func(_ *glib.Object, event *gdk.Event) bool {
		btnEvent := gdk.EventButtonNewFromEvent(event)
		item.FocusedPane = pane
		if tv.OnTabChanged != nil {
			tv.OnTabChanged(sess)
		}
		if btnEvent.Button() == gdk.BUTTON_SECONDARY {
			tv.showTerminalContextMenu(pane, btnEvent.Time())
			return true
		}
		return false
	})

	term.Widget.Connect("focus-in-event", func() {
		item.FocusedPane = pane
		if tv.OnTabChanged != nil {
			tv.OnTabChanged(sess)
		}
	})

	return pane
}

// AddTab creates a new tab with custom header
func (tv *TabView) AddTab(sess *session.Session, term *vte.Terminal) (*TabItem, error) {
	contentBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	contentBox.SetHExpand(true)
	contentBox.SetVExpand(true)

	// EventBox wrapper for tab header
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

	item := &TabItem{
		ID:         sess.ID,
		Session:    sess,
		Label:      titleLabel,
		TabBox:     tabBox,
		EventBox:   eventBox,
		ContentBox: contentBox,
		Panes:      make([]*TerminalPane, 0),
	}

	pane := tv.createPane(item, sess, term)
	item.Panes = append(item.Panes, pane)
	item.FocusedPane = pane

	contentBox.PackStart(pane.Box, true, true, 0)
	contentBox.ShowAll()

	pageNum := tv.Notebook.AppendPage(contentBox, eventBox)
	tv.Notebook.SetTabReorderable(contentBox, true)
	tv.items = append(tv.items, item)

	closeBtn.Connect("clicked", func() {
		tv.CloseTab(item)
	})

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

	tv.Notebook.SetCurrentPage(pageNum)
	term.GrabFocus()
	return item, nil
}

// SplitActiveTab splits the focused pane in the tab (vertical = left/right, horizontal = top/bottom)
func (tv *TabView) SplitActiveTab(item *TabItem, newSess *session.Session, newTerm *vte.Terminal, vertical bool) error {
	if item == nil || item.ContentBox == nil {
		log.Printf("[TAB] SplitActiveTab: item or ContentBox is nil")
		return nil
	}

	targetPane := item.FocusedPane
	if targetPane == nil && len(item.Panes) > 0 {
		targetPane = item.Panes[0]
	}
	if targetPane == nil {
		log.Printf("[TAB] SplitActiveTab: no target pane available")
		return nil
	}

	log.Printf("[TAB] Splitting pane %q (vertical=%v)", targetPane.Session.Title, vertical)

	// Orientation: vertical = left/right (ORIENTATION_HORIZONTAL), horizontal = top/bottom (ORIENTATION_VERTICAL)
	orientation := gtk.ORIENTATION_VERTICAL
	if vertical {
		orientation = gtk.ORIENTATION_HORIZONTAL
	}

	paned, err := gtk.PanedNew(orientation)
	if err != nil {
		return err
	}
	paned.SetWideHandle(true)
	paned.SetHExpand(true)
	paned.SetVExpand(true)

	newPane := tv.createPane(item, newSess, newTerm)

	// Determine parent container of targetPane.Box
	parentObj, pErr := targetPane.Box.GetParent()
	if pErr != nil || parentObj == nil {
		log.Printf("[TAB] ERROR: cannot get parent of target pane: %v", pErr)
		return pErr
	}

	if areWidgetsEqual(parentObj, item.ContentBox) {
		// Target pane was directly in contentBox
		item.ContentBox.Remove(targetPane.Box)
		paned.Pack1(targetPane.Box, true, false)
		paned.Pack2(newPane.Box, true, false)
		item.ContentBox.PackStart(paned, true, true, 0)
	} else {
		// Target pane was inside an existing Paned
		parentPaned := toPaned(parentObj)
		c1, _ := parentPaned.GetChild1()

		if c1 != nil && areWidgetsEqual(c1, targetPane.Box) {
			parentPaned.Remove(targetPane.Box)
			paned.Pack1(targetPane.Box, true, false)
			paned.Pack2(newPane.Box, true, false)
			parentPaned.Pack1(paned, true, false)
		} else {
			parentPaned.Remove(targetPane.Box)
			paned.Pack1(targetPane.Box, true, false)
			paned.Pack2(newPane.Box, true, false)
			parentPaned.Pack2(paned, true, false)
		}
	}

	// Auto-center divider at 50%
	paned.Connect("size-allocate", func(_ *gtk.Paned, alloc *gdk.Rectangle) {
		if vertical {
			w := alloc.GetWidth()
			if w > 40 && paned.GetPosition() <= 10 {
				paned.SetPosition(w / 2)
			}
		} else {
			h := alloc.GetHeight()
			if h > 40 && paned.GetPosition() <= 10 {
				paned.SetPosition(h / 2)
			}
		}
	})

	item.Panes = append(item.Panes, newPane)
	item.FocusedPane = newPane

	item.ContentBox.ShowAll()
	newTerm.GrabFocus()
	return nil
}

// ClosePane removes an individual split pane from the tab hierarchy
func (tv *TabView) ClosePane(pane *TerminalPane) {
	if pane == nil || pane.TabItem == nil {
		return
	}
	item := pane.TabItem
	if len(item.Panes) <= 1 {
		tv.CloseTab(item)
		return
	}

	parentObj, pErr := pane.Box.GetParent()
	if pErr != nil || parentObj == nil {
		return
	}

	parentPaned := toPaned(parentObj)
	c1, _ := parentPaned.GetChild1()
	c2, _ := parentPaned.GetChild2()

	var sibling gtk.IWidget
	if c1 != nil && areWidgetsEqual(c1, pane.Box) {
		sibling = c2
	} else {
		sibling = c1
	}

	grandParentObj, _ := parentPaned.GetParent()
	parentPaned.Remove(pane.Box)
	if sibling != nil {
		parentPaned.Remove(sibling)
	}

	if grandParentObj != nil {
		if areWidgetsEqual(grandParentObj, item.ContentBox) {
			item.ContentBox.Remove(parentPaned)
			if sibling != nil {
				item.ContentBox.PackStart(sibling, true, true, 0)
			}
		} else {
			grandPaned := toPaned(grandParentObj)
			gc1, _ := grandPaned.GetChild1()
			if gc1 != nil && areWidgetsEqual(gc1, parentPaned) {
				grandPaned.Remove(parentPaned)
				if sibling != nil {
					grandPaned.Pack1(sibling, true, false)
				}
			} else {
				grandPaned.Remove(parentPaned)
				if sibling != nil {
					grandPaned.Pack2(sibling, true, false)
				}
			}
		}
	}

	// Remove from item.Panes
	newPanes := make([]*TerminalPane, 0, len(item.Panes)-1)
	for _, p := range item.Panes {
		if p != pane {
			newPanes = append(newPanes, p)
		}
	}
	item.Panes = newPanes

	if pane.Session != nil {
		_ = pane.Session.Close()
	}

	if len(item.Panes) > 0 {
		item.FocusedPane = item.Panes[0]
		if item.FocusedPane.Terminal != nil {
			item.FocusedPane.Terminal.GrabFocus()
		}
	}

	item.ContentBox.ShowAll()
}

// UnsplitTab moves all split panes except the first one into their own individual tabs
func (tv *TabView) UnsplitTab(item *TabItem) {
	if item == nil || len(item.Panes) <= 1 {
		return
	}

	primaryPane := item.Panes[0]
	extraPanes := item.Panes[1:]

	// Clear contentBox and restore primary pane alone
	children := item.ContentBox.GetChildren()
	if children != nil {
		for l := children; l != nil; l = l.Next() {
			if obj, ok := l.Data().(*glib.Object); ok {
				w := &gtk.Widget{InitiallyUnowned: glib.InitiallyUnowned{Object: obj}}
				item.ContentBox.Remove(w)
			}
		}
	}

	item.Panes = []*TerminalPane{primaryPane}
	item.FocusedPane = primaryPane
	item.ContentBox.PackStart(primaryPane.Box, true, true, 0)
	item.ContentBox.ShowAll()

	// Open extra panes as standalone tabs
	for _, extra := range extraPanes {
		_, _ = tv.AddTab(extra.Session, extra.Terminal)
	}
}

// CloseTab closes entire tab and all underlying split sessions
func (tv *TabView) CloseTab(item *TabItem) {
	if item == nil {
		return
	}

	// Close all pane sessions
	for _, p := range item.Panes {
		if p.Session != nil {
			if tv.OnTabClosed != nil {
				tv.OnTabClosed(p.Session)
			}
			_ = p.Session.Close()
		}
	}

	pageNum := tv.Notebook.PageNum(item.ContentBox)
	if pageNum >= 0 {
		tv.Notebook.RemovePage(pageNum)
	}

	newItems := make([]*TabItem, 0, len(tv.items)-1)
	for _, it := range tv.items {
		if it != item {
			newItems = append(newItems, it)
		}
	}
	tv.items = newItems
}

// GetCurrentTab returns active selected TabItem
func (tv *TabView) GetCurrentTab() *TabItem {
	pageNum := tv.Notebook.GetCurrentPage()
	if pageNum < 0 || pageNum >= len(tv.items) {
		return nil
	}
	widget, err := tv.Notebook.GetNthPage(pageNum)
	if err != nil || widget == nil {
		return nil
	}
	for _, item := range tv.items {
		if areWidgetsEqual(item.ContentBox, widget) {
			return item
		}
	}
	return nil
}

// FindTabBySession finds TabItem by Session ID or session instance
func (tv *TabView) FindTabBySession(sess *session.Session) *TabItem {
	if sess == nil {
		return nil
	}
	for _, item := range tv.items {
		for _, p := range item.Panes {
			if p.Session == sess || p.Session.ID == sess.ID {
				return item
			}
		}
		if item.Session == sess || item.Session.ID == sess.ID {
			return item
		}
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
	entry.SetActivatesDefault(true)
	contentArea.Add(entry)

	_, _ = dlg.AddButton("Отмена", gtk.RESPONSE_CANCEL)
	btnOk, _ := dlg.AddButton("Сохранить", gtk.RESPONSE_OK)
	btnOk.SetCanDefault(true)
	dlg.SetDefault(btnOk)

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

	mSplitH, _ := gtk.MenuItemNewWithLabel("Разделить по горизонтали (сверху / снизу)")
	mSplitH.Connect("activate", func() {
		if tv.OnSplitRequested != nil {
			sess := item.Session
			if item.FocusedPane != nil {
				sess = item.FocusedPane.Session
			}
			tv.OnSplitRequested(sess, false)
		}
	})
	menu.Append(mSplitH)

	mSplitV, _ := gtk.MenuItemNewWithLabel("Разделить по вертикали (слева / справа)")
	mSplitV.Connect("activate", func() {
		if tv.OnSplitRequested != nil {
			sess := item.Session
			if item.FocusedPane != nil {
				sess = item.FocusedPane.Session
			}
			tv.OnSplitRequested(sess, true)
		}
	})
	menu.Append(mSplitV)

	if len(item.Panes) > 1 {
		mUnsplit, _ := gtk.MenuItemNewWithLabel("Разгруппировать сплит в отдельные вкладки")
		mUnsplit.Connect("activate", func() {
			tv.UnsplitTab(item)
		})
		menu.Append(mUnsplit)
	}

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

func (tv *TabView) showTerminalContextMenu(pane *TerminalPane, eventTime uint32) {
	menu, _ := gtk.MenuNew()

	mCopy, _ := gtk.MenuItemNewWithLabel("Копировать (Ctrl+Shift+C)")
	mCopy.Connect("activate", func() {
		pane.Terminal.CopyClipboard()
	})
	menu.Append(mCopy)

	mPaste, _ := gtk.MenuItemNewWithLabel("Вставить (Ctrl+Shift+V)")
	mPaste.Connect("activate", func() {
		pane.Terminal.PasteClipboard()
	})
	menu.Append(mPaste)

	sep1, _ := gtk.SeparatorMenuItemNew()
	menu.Append(sep1)

	mSplitH, _ := gtk.MenuItemNewWithLabel("Разделить по горизонтали (сверху / снизу)")
	mSplitH.Connect("activate", func() {
		if tv.OnSplitRequested != nil {
			tv.OnSplitRequested(pane.Session, false)
		}
	})
	menu.Append(mSplitH)

	mSplitV, _ := gtk.MenuItemNewWithLabel("Разделить по вертикали (слева / справа)")
	mSplitV.Connect("activate", func() {
		if tv.OnSplitRequested != nil {
			tv.OnSplitRequested(pane.Session, true)
		}
	})
	menu.Append(mSplitV)

	if len(pane.TabItem.Panes) > 1 {
		mClosePane, _ := gtk.MenuItemNewWithLabel("Закрыть этот терминал")
		mClosePane.Connect("activate", func() {
			tv.ClosePane(pane)
		})
		menu.Append(mClosePane)
	}

	sep2, _ := gtk.SeparatorMenuItemNew()
	menu.Append(sep2)

	mFind, _ := gtk.MenuItemNewWithLabel("Поиск в терминале... (Ctrl+Shift+F)")
	mFind.Connect("activate", func() {
		pane.Search.Show()
	})
	menu.Append(mFind)

	mClear, _ := gtk.MenuItemNewWithLabel("Очистить терминал")
	mClear.Connect("activate", func() {
		pane.Terminal.Reset(true)
	})
	menu.Append(mClear)

	menu.ShowAll()
	menu.PopupAtPointer(nil)
}
