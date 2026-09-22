package ui

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/darakcheeff/pac/internal/i18n"
	"github.com/darakcheeff/pac/internal/session"
	"github.com/darakcheeff/pac/internal/storage"
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

// toContainer safely casts an IWidget to *gtk.Container
func toContainer(iw gtk.IWidget) *gtk.Container {
	if iw == nil {
		return nil
	}
	if c, ok := iw.(*gtk.Container); ok {
		return c
	}
	return &gtk.Container{
		Widget: *iw.ToWidget(),
	}
}

// removeWidgetFromParent detaches a widget from its parent container, if any
func removeWidgetFromParent(iw gtk.IWidget) {
	if iw == nil {
		return
	}
	w := iw.ToWidget()
	parent, err := w.GetParent()
	if err == nil && parent != nil {
		c := toContainer(parent)
		c.Remove(w)
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
	Session           *session.Session
	Terminal          *vte.Terminal
	Box               *gtk.Box
	Search            *SearchBar
	TabItem           *TabItem
	ParentSessionID   string
	SplitDirection    string
	HasUnreadActivity bool
}

const (
	// Dark theme tab colors (vibrant, bright against dark tabs)
	ColorDarkTabAlive    = "#2ec27e"
	ColorDarkTabActivity = "#3584e4"
	ColorDarkTabDead     = "#e01b24"
	ColorDarkTabSep      = "#ffffff"

	// Light theme tab colors (deep, rich, saturated, high-contrast against light tabs)
	ColorLightTabAlive    = "#0e7039" // Saturated deep green
	ColorLightTabActivity = "#1251a3" // Saturated deep royal blue
	ColorLightTabDead     = "#b31b24" // Saturated deep crimson red
	ColorLightTabSep      = "#2e3436" // Dark charcoal separator
)

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
	Notebook               *gtk.Notebook
	IsDark                 bool
	items                  []*TabItem
	lastActiveSession      *session.Session
	plusContent            *gtk.Box
	plusBtn                *gtk.Button
	plusEventBox           *gtk.EventBox
	hasPlusTab             bool
	lastPlusClick          time.Time
	isClosingTab           bool
	OnTabChanged           func(sess *session.Session)
	OnTabClosed            func(sess *session.Session)
	OnSplitRequested       func(sess *session.Session, vertical bool)
	OnDuplicateRequested   func(sess *session.Session)
	OnReconnectRequested   func(sess *session.Session)
	OnToggleNotesRequested func()
	OnFindRequested        func(item *TabItem)
	OnEditHostRequested    func(host *storage.Host)
	OnSaveLogRequested     func(sess *session.Session)
	OnNewConnection        func()
	OnNewLocalTerminal     func()
	OnClusterAdmin         func()
}


func (tv *TabView) isPaneFocused(pane *TerminalPane) bool {
	if pane == nil || pane.TabItem == nil {
		return false
	}
	curTab := tv.GetCurrentTab()
	if curTab != pane.TabItem {
		return false
	}
	if len(pane.TabItem.Panes) <= 1 {
		return true
	}
	return pane.TabItem.FocusedPane == pane
}

func (tv *TabView) getPaneColor(p *TerminalPane) string {
	isDead := false
	if p != nil {
		isDead = (p.Terminal != nil && p.Terminal.IsDisconnected()) || (p.Session != nil && p.Session.IsClosed())
	}
	if tv.IsDark {
		if isDead {
			return ColorDarkTabDead
		}
		if p != nil && p.HasUnreadActivity {
			return ColorDarkTabActivity
		}
		return ColorDarkTabAlive
	}
	if isDead {
		return ColorLightTabDead
	}
	if p != nil && p.HasUnreadActivity {
		return ColorLightTabActivity
	}
	return ColorLightTabAlive
}

func (tv *TabView) getSepColor() string {
	if tv.IsDark {
		return ColorDarkTabSep
	}
	return ColorLightTabSep
}

// SetTheme updates TabView theme mode and refreshes all tab titles
func (tv *TabView) SetTheme(isDark bool) {
	tv.IsDark = isDark
	for _, item := range tv.items {
		tv.UpdateTabTitle(item)
	}
}

func (tv *TabView) getPaneTitle(p *TerminalPane) string {
	if p == nil {
		return "Terminal"
	}
	if p.Session != nil && p.Session.Title != "" {
		return p.Session.Title
	}
	if p.Session != nil && p.Session.Host != nil && p.Session.Host.Name != "" {
		return p.Session.Host.Name
	}
	return "Terminal"
}

// UpdateTabTitle formats tab title with each session colored individually according to state
func (tv *TabView) UpdateTabTitle(item *TabItem) {
	if item == nil || item.Label == nil {
		return
	}

	if len(item.Panes) == 0 {
		title := "Terminal"
		if item.Session != nil && item.Session.Title != "" {
			title = item.Session.Title
		}
		aliveColor := ColorDarkTabAlive
		if !tv.IsDark {
			aliveColor = ColorLightTabAlive
		}
		item.Label.SetMarkup(fmt.Sprintf(`<span foreground="%s">%s</span>`, aliveColor, glib.MarkupEscapeText(title)))
		return
	}

	var parts []string
	var plainParts []string
	for _, p := range item.Panes {
		pTitle := tv.getPaneTitle(p)
		plainParts = append(plainParts, pTitle)
		color := tv.getPaneColor(p)
		escaped := glib.MarkupEscapeText(pTitle)
		parts = append(parts, fmt.Sprintf(`<span foreground="%s">%s</span>`, color, escaped))
	}

	sep := fmt.Sprintf(` <span foreground="%s" weight="bold">+</span> `, tv.getSepColor())
	fullMarkup := strings.Join(parts, sep)
	item.Label.SetMarkup(fullMarkup)

	plainTitle := strings.Join(plainParts, " + ")
	if tv.Notebook != nil && item.ContentBox != nil && tv.Notebook.PageNum(item.ContentBox) >= 0 {
		tv.Notebook.SetMenuLabelText(item.ContentBox, plainTitle)
	}
}

// UpdateTabTitleForSession updates title of the tab holding the session
func (tv *TabView) UpdateTabTitleForSession(sess *session.Session) {
	if sess == nil {
		return
	}
	for _, item := range tv.items {
		for _, pane := range item.Panes {
			if pane.Session == sess {
				tv.UpdateTabTitle(item)
				return
			}
		}
	}
}

// NewTabView initializes the GTK Notebook tab manager
func NewTabView() (*TabView, error) {
	nb, err := gtk.NotebookNew()
	if err != nil {
		return nil, err
	}
	nb.SetScrollable(true)
	nb.SetShowBorder(true)
	nb.SetShowTabs(true)
	nb.PopupEnable()

	tv := &TabView{
		Notebook: nb,
		items:    make([]*TabItem, 0),
		IsDark:   true,
	}

	tv.initPlusTab()

	nb.AddEvents(int(gdk.SCROLL_MASK | gdk.SMOOTH_SCROLL_MASK))
	nb.Connect("scroll-event", func(_ *gtk.Notebook, event *gdk.Event) bool {
		return tv.handleTabScroll(event, true)
	})

	nb.Connect("switch-page", func(_ *gtk.Notebook, page *gtk.Widget, pageNum uint) {
		if tv.plusContent != nil && tv.hasPlusTab {
			plusIdx := tv.Notebook.PageNum(tv.plusContent)
			if plusIdx >= 0 && int(pageNum) == plusIdx {
				// Plus tab was navigated to (e.g. by GTK removing a prior tab or keyboard nav).
				// Do NOT automatically create a new terminal here!
				glib.IdleAdd(func() {
					if len(tv.items) > 0 {
						target := len(tv.items) - 1
						if target >= 0 {
							tv.Notebook.SetCurrentPage(target)
						}
					}
				})
				return
			}
		}

		item := tv.GetCurrentTab()
		if item != nil {
			if item.FocusedPane != nil {
				item.FocusedPane.HasUnreadActivity = false
			} else if len(item.Panes) > 0 {
				item.Panes[0].HasUnreadActivity = false
			}
			tv.UpdateTabTitle(item)

			if item.FocusedPane != nil && item.FocusedPane.Terminal != nil {
				item.FocusedPane.Terminal.GrabFocus()
				tv.lastActiveSession = item.FocusedPane.Session
				if tv.OnTabChanged != nil {
					tv.OnTabChanged(item.FocusedPane.Session)
				}
			} else if len(item.Panes) > 0 && item.Panes[0].Terminal != nil {
				item.Panes[0].Terminal.GrabFocus()
				tv.lastActiveSession = item.Panes[0].Session
				if tv.OnTabChanged != nil {
					tv.OnTabChanged(item.Panes[0].Session)
				}
			}
		} else {
			tv.lastActiveSession = nil
			if tv.OnTabChanged != nil {
				tv.OnTabChanged(nil)
			}
		}
	})

	return tv, nil
}

// triggerNewLocalTerminal executes OnNewLocalTerminal with a debounce to prevent double-clicks
func (tv *TabView) triggerNewLocalTerminal() {
	now := time.Now()
	if now.Sub(tv.lastPlusClick) < 300*time.Millisecond {
		return
	}
	tv.lastPlusClick = now
	if tv.OnNewLocalTerminal != nil {
		tv.OnNewLocalTerminal()
	}
}

// initPlusTab creates the browser-style '+' new tab button that sits immediately to the right of open tabs
func (tv *TabView) initPlusTab() {
	plusBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	plusBox.SetHExpand(false)
	plusBox.SetVExpand(false)

	eventBox, _ := gtk.EventBoxNew()
	eventBox.SetEvents(int(gdk.BUTTON_PRESS_MASK | gdk.BUTTON_RELEASE_MASK))

	btnPlus, _ := gtk.ButtonNewFromIconName("list-add-symbolic", gtk.ICON_SIZE_MENU)
	btnPlus.SetRelief(gtk.RELIEF_NONE)
	btnPlus.SetTooltipText(i18n.T("Создать новую вкладку (Локальный терминал)", "Create new tab (Local Terminal)"))
	btnPlus.SetMarginStart(0)
	btnPlus.SetMarginEnd(0)
	btnPlus.SetMarginTop(0)
	btnPlus.SetMarginBottom(0)

	btnPlus.Connect("clicked", func() {
		tv.triggerNewLocalTerminal()
	})

	eventBox.Connect("button-press-event", func(_ *gtk.EventBox, event *gdk.Event) bool {
		btnEvent := gdk.EventButtonNewFromEvent(event)
		if btnEvent.Button() == gdk.BUTTON_PRIMARY {
			tv.triggerNewLocalTerminal()
			return true
		}
		return false
	})

	eventBox.Add(btnPlus)
	eventBox.ShowAll()

	tv.plusContent = plusBox
	tv.plusBtn = btnPlus
	tv.plusEventBox = eventBox
	tv.hasPlusTab = true

	tv.Notebook.AppendPage(plusBox, eventBox)
	tv.Notebook.SetTabReorderable(plusBox, false)
	tv.Notebook.SetTabDetachable(plusBox, false)
	tv.Notebook.SetMenuLabelText(plusBox, "")
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

	term.OnContentsChanged = func() {
		glib.IdleAdd(func() {
			if pane == nil || pane.TabItem == nil {
				return
			}
			if !tv.isPaneFocused(pane) {
				if !pane.HasUnreadActivity {
					pane.HasUnreadActivity = true
					tv.UpdateTabTitle(pane.TabItem)
				}
			}
		})
	}

	// Focus and click handling
	term.Widget.Connect("button-press-event", func(_ *glib.Object, event *gdk.Event) bool {
		btnEvent := gdk.EventButtonNewFromEvent(event)
		wasFocused := pane.TabItem != nil && pane.TabItem.FocusedPane == pane
		if pane.TabItem != nil {
			pane.TabItem.FocusedPane = pane
		}
		if pane.HasUnreadActivity {
			pane.HasUnreadActivity = false
			if pane.TabItem != nil {
				tv.UpdateTabTitle(pane.TabItem)
			}
		}
		if (!wasFocused || tv.lastActiveSession != sess) && tv.OnTabChanged != nil {
			tv.lastActiveSession = sess
			tv.OnTabChanged(sess)
		}
		if btnEvent.Button() == gdk.BUTTON_SECONDARY {
			tv.showTerminalContextMenu(pane, btnEvent.Time())
			return true
		}
		return false
	})

	term.Widget.Connect("focus-in-event", func() {
		wasFocused := pane.TabItem != nil && pane.TabItem.FocusedPane == pane
		if pane.TabItem != nil {
			pane.TabItem.FocusedPane = pane
		}
		if pane.HasUnreadActivity {
			pane.HasUnreadActivity = false
			if pane.TabItem != nil {
				tv.UpdateTabTitle(pane.TabItem)
			}
		}
		if (!wasFocused || tv.lastActiveSession != sess) && tv.OnTabChanged != nil {
			tv.lastActiveSession = sess
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
	eventBox.SetEvents(int(gdk.BUTTON_PRESS_MASK | gdk.BUTTON_RELEASE_MASK | gdk.SCROLL_MASK | gdk.SMOOTH_SCROLL_MASK))

	tabBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	tabBox.SetMarginStart(2)
	tabBox.SetMarginEnd(2)


	titleLabel, _ := gtk.LabelNew(sess.Title)
	tabBox.PackStart(titleLabel, true, true, 0)

	closeBtn, _ := gtk.ButtonNewFromIconName("window-close-symbolic", gtk.ICON_SIZE_MENU)
	closeBtn.SetRelief(gtk.RELIEF_NONE)
	closeBtn.SetTooltipText(i18n.T("Закрыть вкладку", "Close tab"))
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
	tv.UpdateTabTitle(item)

	contentBox.PackStart(pane.Box, true, true, 0)
	contentBox.ShowAll()

	plusIdx := -1
	if tv.plusContent != nil && tv.hasPlusTab {
		plusIdx = tv.Notebook.PageNum(tv.plusContent)
	}
	var pageNum int
	if plusIdx >= 0 {
		pageNum = tv.Notebook.InsertPage(contentBox, eventBox, plusIdx)
	} else {
		pageNum = tv.Notebook.AppendPage(contentBox, eventBox)
	}
	tv.Notebook.SetTabReorderable(contentBox, true)
	tv.items = append(tv.items, item)

	closeBtn.Connect("clicked", func() {
		tv.CloseTab(item)
	})

	eventBox.Connect("scroll-event", func(_ *gtk.EventBox, event *gdk.Event) bool {
		return tv.handleTabScroll(event, false)
	})

	eventBox.Connect("button-press-event", func(_ *gtk.EventBox, event *gdk.Event) bool {
		btnEvent := gdk.EventButtonNewFromEvent(event)
		if btnEvent.Type() == gdk.EVENT_2BUTTON_PRESS && btnEvent.Button() == gdk.BUTTON_PRIMARY {
			tv.showRenameDialog(item)
			return true
		} else if btnEvent.Button() == gdk.BUTTON_SECONDARY {
			tv.showTabContextMenu(item, btnEvent.Time())
			return true
		} else if btnEvent.Button() == gdk.BUTTON_MIDDLE {
			tv.CloseTab(item)
			return true
		}
		return false
	})

	tv.Notebook.SetCurrentPage(pageNum)
	term.GrabFocus()
	return item, nil
}

// handleTabScroll cycles through open tabs using mouse wheel over tab headers or tab bar
func (tv *TabView) handleTabScroll(event *gdk.Event, checkY bool) bool {
	scrollEvent := gdk.EventScrollNewFromEvent(event)
	// If checkY is true, only handle scrolls within notebook tab strip (top 50px)
	if checkY && scrollEvent.Y() > 50 {
		return false
	}

	nPages := len(tv.items)
	if nPages <= 1 {
		return false
	}

	curr := tv.Notebook.GetCurrentPage()
	if curr >= nPages {
		curr = 0
	}
	var dir int // -1 for previous, +1 for next

	switch scrollEvent.Direction() {
	case gdk.SCROLL_UP, gdk.SCROLL_LEFT:
		dir = -1
	case gdk.SCROLL_DOWN, gdk.SCROLL_RIGHT:
		dir = 1
	case gdk.SCROLL_SMOOTH:
		dy := scrollEvent.DeltaY()
		dx := scrollEvent.DeltaX()
		if dy < -0.01 || dx < -0.01 {
			dir = -1
		} else if dy > 0.01 || dx > 0.01 {
			dir = 1
		}
	}

	if dir == 0 {
		return false
	}

	nextPage := curr + dir
	if nextPage < 0 {
		nextPage = nPages - 1
	} else if nextPage >= nPages {
		nextPage = 0
	}

	tv.Notebook.SetCurrentPage(nextPage)
	return true
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
	if vertical {
		newPane.SplitDirection = "vertical"
	} else {
		newPane.SplitDirection = "horizontal"
	}
	if targetPane != nil && targetPane.Session != nil {
		newPane.ParentSessionID = targetPane.Session.ID
	}

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
	paned.Connect("size-allocate", func() {
		if vertical {
			w := paned.GetAllocatedWidth()
			if w > 40 && paned.GetPosition() <= 10 {
				paned.SetPosition(w / 2)
			}
		} else {
			h := paned.GetAllocatedHeight()
			if h > 40 && paned.GetPosition() <= 10 {
				paned.SetPosition(h / 2)
			}
		}
	})

	item.Panes = append(item.Panes, newPane)
	item.FocusedPane = newPane
	tv.UpdateTabTitle(item)

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
	tv.UpdateTabTitle(item)

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

// AddTabWithPane creates a new TabItem using an existing TerminalPane (used for unsplitting / detaching)
func (tv *TabView) AddTabWithPane(pane *TerminalPane) (*TabItem, error) {
	if pane == nil || pane.Session == nil {
		return nil, fmt.Errorf("invalid pane")
	}

	contentBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 0)
	if err != nil {
		return nil, err
	}
	contentBox.SetHExpand(true)
	contentBox.SetVExpand(true)

	eventBox, _ := gtk.EventBoxNew()
	eventBox.SetEvents(int(gdk.BUTTON_PRESS_MASK | gdk.BUTTON_RELEASE_MASK | gdk.SCROLL_MASK | gdk.SMOOTH_SCROLL_MASK))

	tabBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	tabBox.SetMarginStart(2)
	tabBox.SetMarginEnd(2)


	titleLabel, _ := gtk.LabelNew(pane.Session.Title)
	tabBox.PackStart(titleLabel, true, true, 0)

	closeBtn, _ := gtk.ButtonNewFromIconName("window-close-symbolic", gtk.ICON_SIZE_MENU)
	closeBtn.SetRelief(gtk.RELIEF_NONE)
	closeBtn.SetTooltipText(i18n.T("Закрыть вкладку", "Close tab"))
	tabBox.PackEnd(closeBtn, false, false, 0)

	eventBox.Add(tabBox)
	eventBox.ShowAll()

	item := &TabItem{
		ID:          pane.Session.ID,
		Session:     pane.Session,
		Label:       titleLabel,
		TabBox:      tabBox,
		EventBox:    eventBox,
		ContentBox:  contentBox,
		Panes:       []*TerminalPane{pane},
		FocusedPane: pane,
	}

	pane.TabItem = item
	pane.SplitDirection = ""
	pane.ParentSessionID = ""
	tv.UpdateTabTitle(item)

	// Ensure pane.Box has no lingering parent before packing
	removeWidgetFromParent(pane.Box)

	contentBox.PackStart(pane.Box, true, true, 0)
	contentBox.ShowAll()

	plusIdx := -1
	if tv.plusContent != nil && tv.hasPlusTab {
		plusIdx = tv.Notebook.PageNum(tv.plusContent)
	}
	var pageNum int
	if plusIdx >= 0 {
		pageNum = tv.Notebook.InsertPage(contentBox, eventBox, plusIdx)
	} else {
		pageNum = tv.Notebook.AppendPage(contentBox, eventBox)
	}
	tv.Notebook.SetTabReorderable(contentBox, true)
	tv.items = append(tv.items, item)

	closeBtn.Connect("clicked", func() {
		tv.CloseTab(item)
	})

	eventBox.Connect("scroll-event", func(_ *gtk.EventBox, event *gdk.Event) bool {
		return tv.handleTabScroll(event, false)
	})

	eventBox.Connect("button-press-event", func(_ *gtk.EventBox, event *gdk.Event) bool {
		btnEvent := gdk.EventButtonNewFromEvent(event)
		if btnEvent.Type() == gdk.EVENT_2BUTTON_PRESS && btnEvent.Button() == gdk.BUTTON_PRIMARY {
			tv.showRenameDialog(item)
			return true
		} else if btnEvent.Button() == gdk.BUTTON_SECONDARY {
			tv.showTabContextMenu(item, btnEvent.Time())
			return true
		} else if btnEvent.Button() == gdk.BUTTON_MIDDLE {
			tv.CloseTab(item)
			return true
		}
		return false
	})

	tv.Notebook.SetCurrentPage(pageNum)
	if pane.Terminal != nil {
		pane.Terminal.GrabFocus()
	}
	return item, nil
}

// UnsplitTab detaches the currently active/focused split pane into its own individual tab
func (tv *TabView) UnsplitTab(item *TabItem) {
	if item == nil || len(item.Panes) <= 1 {
		log.Printf("[TAB] UnsplitTab: item is nil or has <= 1 panes")
		return
	}

	pane := item.FocusedPane
	if pane == nil {
		pane = item.Panes[len(item.Panes)-1]
	}

	log.Printf("[TAB] Detaching active pane %q from tab %q (remaining panes: %d)", pane.Session.Title, item.Session.Title, len(item.Panes)-1)

	parentObj, pErr := pane.Box.GetParent()
	if pErr != nil || parentObj == nil {
		log.Printf("[TAB] ERROR: cannot get parent of pane.Box: %v", pErr)
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

	// Remove detached pane from item.Panes
	newPanes := make([]*TerminalPane, 0, len(item.Panes)-1)
	for _, p := range item.Panes {
		if p != pane {
			newPanes = append(newPanes, p)
		}
	}
	item.Panes = newPanes

	// If only 1 pane left in original tab, clear split markers
	if len(item.Panes) == 1 {
		item.Panes[0].SplitDirection = ""
		item.Panes[0].ParentSessionID = ""
	}

	// Update focus and session of original tab
	if len(item.Panes) > 0 {
		item.FocusedPane = item.Panes[0]
		if item.Session == pane.Session {
			item.Session = item.Panes[0].Session
			item.ID = item.Session.ID
		}
		tv.UpdateTabTitle(item)
	}

	item.ContentBox.ShowAll()

	// Open detached pane as its own new tab
	newTab, err := tv.AddTabWithPane(pane)
	if err != nil {
		log.Printf("[TAB] ERROR adding tab for detached pane %q: %v", pane.Session.Title, err)
		return
	}

	pageNum := tv.Notebook.PageNum(newTab.ContentBox)
	if pageNum >= 0 {
		tv.Notebook.SetCurrentPage(pageNum)
	}
	if pane.Terminal != nil {
		pane.Terminal.GrabFocus()
	}

	log.Printf("[TAB] Pane %q detached into new tab successfully. Original tab %q now has %d pane(s).", pane.Session.Title, item.Session.Title, len(item.Panes))
}

// CloseTab closes entire tab and all underlying split sessions
func (tv *TabView) CloseTab(item *TabItem) {
	if item == nil {
		return
	}

	tv.isClosingTab = true
	defer func() {
		tv.isClosingTab = false
	}()

	// Remove from tv.items FIRST so len(tv.items) and GetCurrentTab reflect new state immediately
	newItems := make([]*TabItem, 0, len(tv.items)-1)
	for _, it := range tv.items {
		if it != item {
			newItems = append(newItems, it)
		}
	}
	tv.items = newItems

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

	if len(tv.items) == 0 {
		tv.lastActiveSession = nil
		if tv.OnTabChanged != nil {
			tv.OnTabChanged(nil)
		}
	} else {
		cur := tv.GetCurrentTab()
		if cur != nil && cur.FocusedPane != nil && cur.FocusedPane.Session != nil {
			tv.lastActiveSession = cur.FocusedPane.Session
			if tv.OnTabChanged != nil {
				tv.OnTabChanged(cur.FocusedPane.Session)
			}
		}
	}
}

// CloseOtherTabs closes all tabs except the specified one
func (tv *TabView) CloseOtherTabs(keepItem *TabItem) {
	for _, it := range append([]*TabItem(nil), tv.items...) {
		if it != keepItem {
			tv.CloseTab(it)
		}
	}
}

// CloseTabsToRight closes all tabs to the right of the specified one
func (tv *TabView) CloseTabsToRight(fromItem *TabItem) {
	idx := -1
	for i, it := range tv.items {
		if it == fromItem {
			idx = i
			break
		}
	}
	if idx >= 0 && idx < len(tv.items)-1 {
		for i := len(tv.items) - 1; i > idx; i-- {
			tv.CloseTab(tv.items[i])
		}
	}
}

// CloseAllTabs closes all open tabs
func (tv *TabView) CloseAllTabs() {
	for _, it := range append([]*TabItem(nil), tv.items...) {
		tv.CloseTab(it)
	}
}

// GetCurrentTab returns active selected TabItem
func (tv *TabView) GetCurrentTab() *TabItem {
	pageNum := tv.Notebook.GetCurrentPage()
	if pageNum < 0 {
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

// SelectSession switches notebook to the tab containing session with specified ID
func (tv *TabView) SelectSession(sessID string) {
	for idx, item := range tv.items {
		if item.Session != nil && item.Session.ID == sessID {
			tv.Notebook.SetCurrentPage(idx)
			return
		}
		for _, p := range item.Panes {
			if p.Session != nil && p.Session.ID == sessID {
				tv.Notebook.SetCurrentPage(idx)
				return
			}
		}
	}
}

func (tv *TabView) showRenameDialog(item *TabItem) {
	dlg, _ := gtk.DialogNew()
	dlg.SetTitle(i18n.T("Переименовать вкладку", "Rename Tab"))
	dlg.SetModal(true)
	dlg.SetDefaultSize(300, 100)

	contentArea, _ := dlg.GetContentArea()
	entry, _ := gtk.EntryNew()
	entry.SetText(item.Session.Title)
	entry.SetActivatesDefault(true)
	contentArea.Add(entry)

	_, _ = dlg.AddButton(i18n.T("Отмена", "Cancel"), gtk.RESPONSE_CANCEL)
	btnOk, _ := dlg.AddButton(i18n.T("Сохранить", "Save"), gtk.RESPONSE_OK)
	btnOk.SetCanDefault(true)
	dlg.SetDefault(btnOk)

	dlg.ShowAll()

	if dlg.Run() == gtk.RESPONSE_OK {
		newTitle, _ := entry.GetText()
		if newTitle != "" {
			item.Session.Title = newTitle
			tv.UpdateTabTitle(item)
		}
	}
	dlg.Destroy()
}

func (tv *TabView) showTabContextMenu(item *TabItem, eventTime uint32) {
	menu, _ := gtk.MenuNew()

	// 1. Goto TAB ▸
	mGoto, _ := gtk.MenuItemNewWithLabel(i18n.T("Перейти к вкладке", "Goto TAB"))
	gotoSubmenu, _ := gtk.MenuNew()
	for idx, it := range tv.items {
		tabIdx := idx
		mTab, _ := gtk.MenuItemNewWithLabel(fmt.Sprintf("%d: %s", tabIdx+1, it.Session.Title))
		mTab.Connect("activate", func() {
			tv.Notebook.SetCurrentPage(tabIdx)
		})
		gotoSubmenu.Append(mTab)
	}
	mGoto.SetSubmenu(gotoSubmenu)
	menu.Append(mGoto)

	// 2. Detach TAB to a new Window
	mDetach, _ := gtk.MenuItemNewWithLabel(i18n.T("Открепить вкладку в новое окно", "Detach TAB to a new Window"))
	mDetach.SetSensitive(false)
	menu.Append(mDetach)

	// 3. Split ▸
	mSplit, _ := gtk.MenuItemNewWithLabel(i18n.T("Разделить", "Split"))
	splitSubmenu, _ := gtk.MenuNew()

	mSplitV, _ := gtk.MenuItemNewWithLabel(i18n.T("По вертикали", "Vertically"))
	mSplitV.SetSubmenu(tv.buildSplitSubmenu(item, item.Session, true))
	splitSubmenu.Append(mSplitV)

	mSplitH, _ := gtk.MenuItemNewWithLabel(i18n.T("По горизонтали", "Horizontally"))
	mSplitH.SetSubmenu(tv.buildSplitSubmenu(item, item.Session, false))
	splitSubmenu.Append(mSplitH)

	if len(item.Panes) > 1 {
		mUnsplit, _ := gtk.MenuItemNewWithLabel(i18n.T("Разгруппировать (↔)", "Unsplit (↔)"))
		mUnsplit.Connect("activate", func() {
			tv.UnsplitTab(item)
		})
		splitSubmenu.Append(mUnsplit)
	}

	mSplit.SetSubmenu(splitSubmenu)
	menu.Append(mSplit)

	// 4. Add to Cluster
	mAddCluster, _ := gtk.MenuItemNewWithLabel(i18n.T("Добавить в кластер", "Add to Cluster"))
	mAddCluster.Connect("activate", func() {
		if tv.OnClusterAdmin != nil {
			tv.OnClusterAdmin()
		}
	})
	menu.Append(mAddCluster)

	// 5. Remove from Cluster
	mRemCluster, _ := gtk.MenuItemNewWithLabel(i18n.T("Удалить из кластера", "Remove from Cluster"))
	mRemCluster.SetSensitive(false)
	menu.Append(mRemCluster)

	// 6. Cluster Admin...
	mClusterAdmin, _ := gtk.MenuItemNewWithLabel(i18n.T("Управление кластерами...", "Cluster Admin..."))
	mClusterAdmin.Connect("activate", func() {
		if tv.OnClusterAdmin != nil {
			tv.OnClusterAdmin()
		}
	})
	menu.Append(mClusterAdmin)

	// 7. Find...
	mFind, _ := gtk.MenuItemNewWithLabel(i18n.T("Поиск...", "Find..."))
	mFind.Connect("activate", func() {
		if tv.OnFindRequested != nil {
			tv.OnFindRequested(item)
		}
	})
	menu.Append(mFind)

	// 8. Save session log...
	mSaveLog, _ := gtk.MenuItemNewWithLabel(i18n.T("Сохранить журнал сессии...", "Save session log..."))
	mSaveLog.Connect("activate", func() {
		if tv.OnSaveLogRequested != nil {
			tv.OnSaveLogRequested(item.Session)
		}
	})
	menu.Append(mSaveLog)

	// 9. Edit session...
	mEditSession, _ := gtk.MenuItemNewWithLabel(i18n.T("Редактировать сессию...", "Edit session..."))
	mEditSession.Connect("activate", func() {
		if tv.OnEditHostRequested != nil && item.Session != nil && item.Session.Host != nil {
			tv.OnEditHostRequested(item.Session.Host)
		}
	})
	menu.Append(mEditSession)

	// 10. Temporary TAB Label change...
	mRename, _ := gtk.MenuItemNewWithLabel(i18n.T("Переименовать вкладку...", "Rename Tab..."))
	mRename.Connect("activate", func() {
		tv.showRenameDialog(item)
	})
	menu.Append(mRename)

	// 11. New connection
	mNewConn, _ := gtk.MenuItemNewWithLabel(i18n.T("Новое подключение...", "New connection..."))
	mNewConn.Connect("activate", func() {
		if tv.OnNewConnection != nil {
			tv.OnNewConnection()
		}
	})
	menu.Append(mNewConn)

	// 12. Duplicate connection
	mDuplicate, _ := gtk.MenuItemNewWithLabel(i18n.T("Дублировать подключение", "Duplicate connection"))
	mDuplicate.Connect("activate", func() {
		if tv.OnDuplicateRequested != nil {
			tv.OnDuplicateRequested(item.Session)
		}
	})
	menu.Append(mDuplicate)

	// 13. Disconnect session
	mDisconnect, _ := gtk.MenuItemNewWithLabel(i18n.T("Отключить сессию", "Disconnect session"))
	mDisconnect.Connect("activate", func() {
		if item.Session != nil {
			_ = item.Session.Close()
		}
	})
	menu.Append(mDisconnect)

	// 14. Restart session
	mRestart, _ := gtk.MenuItemNewWithLabel(i18n.T("Перезапустить сессию", "Restart session"))
	mRestart.Connect("activate", func() {
		if tv.OnReconnectRequested != nil {
			tv.OnReconnectRequested(item.Session)
		}
	})
	menu.Append(mRestart)

	// 15. Close terminal
	mClose, _ := gtk.MenuItemNewWithLabel(i18n.T("Закрыть терминал", "Close terminal"))
	mClose.Connect("activate", func() {
		tv.CloseTab(item)
	})
	menu.Append(mClose)

	// 16. Close other terminals
	mCloseOthers, _ := gtk.MenuItemNewWithLabel(i18n.T("Закрыть другие терминалы", "Close other terminals"))
	mCloseOthers.Connect("activate", func() {
		tv.CloseOtherTabs(item)
	})
	if len(tv.items) <= 1 {
		mCloseOthers.SetSensitive(false)
	}
	menu.Append(mCloseOthers)

	// 17. Close all terminals
	mCloseAll, _ := gtk.MenuItemNewWithLabel(i18n.T("Закрыть все терминалы", "Close all terminals"))
	mCloseAll.Connect("activate", func() {
		tv.CloseAllTabs()
	})
	menu.Append(mCloseAll)

	menu.ShowAll()
	menu.PopupAtPointer(nil)
}

func (tv *TabView) showTerminalContextMenu(pane *TerminalPane, eventTime uint32) {
	menu, _ := gtk.MenuNew()

	mCopy, _ := gtk.MenuItemNewWithLabel(i18n.T("Копировать", "Copy"))
	mCopy.Connect("activate", func() {
		pane.Terminal.CopyClipboard()
	})
	menu.Append(mCopy)

	mPaste, _ := gtk.MenuItemNewWithLabel(i18n.T("Вставить", "Paste"))
	mPaste.Connect("activate", func() {
		pane.Terminal.PasteClipboard()
	})
	menu.Append(mPaste)

	mReset, _ := gtk.MenuItemNewWithLabel(i18n.T("Сбросить терминал (Reset)", "Reset terminal"))
	mReset.Connect("activate", func() {
		pane.Terminal.ResetTerminal()
	})
	menu.Append(mReset)

	sep1, _ := gtk.SeparatorMenuItemNew()
	menu.Append(sep1)

	mSplitH, _ := gtk.MenuItemNewWithLabel(i18n.T("Разделить по горизонтали (сверху / снизу)", "Split horizontally (top / bottom)"))
	mSplitH.SetSubmenu(tv.buildSplitSubmenu(pane.TabItem, pane.Session, false))
	menu.Append(mSplitH)

	mSplitV, _ := gtk.MenuItemNewWithLabel(i18n.T("Разделить по вертикали (слева / справа)", "Split vertically (left / right)"))
	mSplitV.SetSubmenu(tv.buildSplitSubmenu(pane.TabItem, pane.Session, true))
	menu.Append(mSplitV)

	if len(pane.TabItem.Panes) > 1 {
		mDetach, _ := gtk.MenuItemNewWithLabel(i18n.T("Разгруппировать: вынести в новую вкладку (↔)", "Detach to new tab (↔)"))
		mDetach.Connect("activate", func() {
			pane.TabItem.FocusedPane = pane
			tv.UnsplitTab(pane.TabItem)
		})
		menu.Append(mDetach)

		mClosePane, _ := gtk.MenuItemNewWithLabel(i18n.T("Закрыть этот терминал", "Close this terminal"))
		mClosePane.Connect("activate", func() {
			tv.ClosePane(pane)
		})
		menu.Append(mClosePane)
	}

	sep2, _ := gtk.SeparatorMenuItemNew()
	menu.Append(sep2)

	mFind, _ := gtk.MenuItemNewWithLabel(i18n.T("Поиск в терминале...", "Find in terminal..."))
	mFind.Connect("activate", func() {
		pane.Search.Show()
	})
	menu.Append(mFind)

	mClear, _ := gtk.MenuItemNewWithLabel(i18n.T("Очистить терминал", "Clear terminal"))
	mClear.Connect("activate", func() {
		pane.Terminal.Reset(true)
	})
	menu.Append(mClear)

	menu.ShowAll()
	menu.PopupAtPointer(nil)
}

// UpdateSessionForTerminal updates session reference in tab and panes for a terminal
func (tv *TabView) UpdateSessionForTerminal(term *vte.Terminal, newSess *session.Session) {
	for _, item := range tv.items {
		for _, pane := range item.Panes {
			if pane.Terminal == term {
				pane.Session = newSess
				pane.HasUnreadActivity = false
				if item.Session == nil || item.Panes[0] == pane {
					item.Session = newSess
				}
				if item.FocusedPane == pane && tv.OnTabChanged != nil {
					tv.OnTabChanged(newSess)
				}
				tv.UpdateTabTitle(item)
				return
			}
		}
	}
}

func (tv *TabView) buildSplitSubmenu(currentTab *TabItem, sess *session.Session, vertical bool) *gtk.Menu {
	sub, _ := gtk.MenuNew()

	mNewTerm, _ := gtk.MenuItemNewWithLabel(i18n.T("Новый терминал", "New terminal"))
	mNewTerm.Connect("activate", func() {
		if tv.OnSplitRequested != nil {
			targetSess := sess
			if currentTab != nil && currentTab.FocusedPane != nil && currentTab.FocusedPane.Session != nil {
				targetSess = currentTab.FocusedPane.Session
			}
			tv.OnSplitRequested(targetSess, vertical)
		}
	})
	sub.Append(mNewTerm)

	var otherTabs []*TabItem
	for _, t := range tv.items {
		if t != currentTab {
			otherTabs = append(otherTabs, t)
		}
	}

	if len(otherTabs) > 0 {
		sep, _ := gtk.SeparatorMenuItemNew()
		sub.Append(sep)

		for _, other := range otherTabs {
			targetOther := other
			title := "Tab"
			if targetOther.Session != nil && targetOther.Session.Title != "" {
				title = targetOther.Session.Title
			}
			mOther, _ := gtk.MenuItemNewWithLabel(i18n.Tf("Объединить с вкладкой: %s", "Merge with tab: %s", title))
			mOther.Connect("activate", func() {
				_ = tv.MergeTabInto(currentTab, targetOther, vertical)
			})
			sub.Append(mOther)
		}
	}

	return sub
}

// MergeTabInto combines sourceTab into targetTab as a split view, closing sourceTab
func (tv *TabView) MergeTabInto(targetTab, sourceTab *TabItem, vertical bool) error {
	if targetTab == nil || sourceTab == nil || targetTab == sourceTab {
		return fmt.Errorf("invalid tabs for merge")
	}

	targetPane := targetTab.FocusedPane
	if targetPane == nil && len(targetTab.Panes) > 0 {
		targetPane = targetTab.Panes[0]
	}
	if targetPane == nil {
		return fmt.Errorf("no target pane")
	}

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

	// Remove sourceTab from Notebook without closing its session
	sourcePageNum := tv.Notebook.PageNum(sourceTab.ContentBox)
	if sourcePageNum >= 0 {
		tv.Notebook.RemovePage(sourcePageNum)
	}

	// Remove sourceTab from tv.items
	newItems := make([]*TabItem, 0, len(tv.items)-1)
	for _, it := range tv.items {
		if it != sourceTab {
			newItems = append(newItems, it)
		}
	}
	tv.items = newItems

	var sourceWidget gtk.IWidget
	if len(sourceTab.Panes) == 1 {
		sourcePane := sourceTab.Panes[0]
		sourceTab.ContentBox.Remove(sourcePane.Box)
		sourcePane.TabItem = targetTab
		if vertical {
			sourcePane.SplitDirection = "vertical"
		} else {
			sourcePane.SplitDirection = "horizontal"
		}
		if targetPane.Session != nil {
			sourcePane.ParentSessionID = targetPane.Session.ID
		}
		targetTab.Panes = append(targetTab.Panes, sourcePane)
		targetTab.FocusedPane = sourcePane
		sourceWidget = sourcePane.Box
	} else {
		for _, p := range sourceTab.Panes {
			p.TabItem = targetTab
			targetTab.Panes = append(targetTab.Panes, p)
		}
		if sourceTab.FocusedPane != nil {
			targetTab.FocusedPane = sourceTab.FocusedPane
		}
		sourceWidget = sourceTab.ContentBox
	}

	parentObj, pErr := targetPane.Box.GetParent()
	if pErr != nil || parentObj == nil {
		return fmt.Errorf("cannot get parent of target pane: %v", pErr)
	}

	if areWidgetsEqual(parentObj, targetTab.ContentBox) {
		targetTab.ContentBox.Remove(targetPane.Box)
		paned.Pack1(targetPane.Box, true, false)
		paned.Pack2(sourceWidget, true, false)
		targetTab.ContentBox.PackStart(paned, true, true, 0)
	} else {
		parentPaned := toPaned(parentObj)
		c1, _ := parentPaned.GetChild1()
		if c1 != nil && areWidgetsEqual(c1, targetPane.Box) {
			parentPaned.Remove(targetPane.Box)
			paned.Pack1(targetPane.Box, true, false)
			paned.Pack2(sourceWidget, true, false)
			parentPaned.Pack1(paned, true, false)
		} else {
			parentPaned.Remove(targetPane.Box)
			paned.Pack1(targetPane.Box, true, false)
			paned.Pack2(sourceWidget, true, false)
			parentPaned.Pack2(paned, true, false)
		}
	}

	paned.Connect("size-allocate", func() {
		if vertical {
			w := paned.GetAllocatedWidth()
			if w > 40 && paned.GetPosition() <= 10 {
				paned.SetPosition(w / 2)
			}
		} else {
			h := paned.GetAllocatedHeight()
			if h > 40 && paned.GetPosition() <= 10 {
				paned.SetPosition(h / 2)
			}
		}
	})

	targetTab.ContentBox.ShowAll()
	tv.UpdateTabTitle(targetTab)
	if targetTab.FocusedPane != nil && targetTab.FocusedPane.Terminal != nil {
		targetTab.FocusedPane.Terminal.GrabFocus()
		if tv.OnTabChanged != nil && targetTab.FocusedPane.Session != nil {
			tv.lastActiveSession = targetTab.FocusedPane.Session
			tv.OnTabChanged(targetTab.FocusedPane.Session)
		}
	}

	return nil
}
