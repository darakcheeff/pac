package ui

import (
	"sync"
	"time"

	"github.com/darakcheeff/pac/internal/storage"
	"github.com/gotk3/gotk3/gtk"
)

// NotesPanel represents the right-side collapsible session notes panel
type NotesPanel struct {
	Box         *gtk.Box
	TextView    *gtk.TextView
	TextBuffer  *gtk.TextBuffer
	HeaderLabel *gtk.Label
	currentHost *storage.Host
	store       *storage.Store
	saveTimer   *time.Timer
	mu          sync.Mutex
}

func NewNotesPanel(store *storage.Store) (*NotesPanel, error) {
	box, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 4)
	if err != nil {
		return nil, err
	}
	box.SetMarginStart(6)
	box.SetMarginEnd(6)
	box.SetMarginTop(6)
	box.SetMarginBottom(6)
	box.SetSizeRequest(260, -1)

	// Header Bar
	headerBox, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 4)
	titleLabel, _ := gtk.LabelNew("Заметки к сессии")
	titleLabel.SetHAlign(gtk.ALIGN_START)
	titleLabel.SetHExpand(true)

	clearBtn, _ := gtk.ButtonNewFromIconName("edit-clear-symbolic", gtk.ICON_SIZE_BUTTON)
	clearBtn.SetTooltipText("Очистить заметку")

	headerBox.PackStart(titleLabel, true, true, 0)
	headerBox.PackEnd(clearBtn, false, false, 0)
	box.PackStart(headerBox, false, false, 0)

	// Scrollable Text View
	scrolled, _ := gtk.ScrolledWindowNew(nil, nil)
	scrolled.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	scrolled.SetShadowType(gtk.SHADOW_IN)
	scrolled.SetVExpand(true)

	textView, _ := gtk.TextViewNew()
	textView.SetWrapMode(gtk.WRAP_WORD_CHAR)
	textView.SetLeftMargin(6)
	textView.SetRightMargin(6)
	textView.SetTopMargin(6)
	textView.SetBottomMargin(6)

	buf, _ := textView.GetBuffer()
	scrolled.Add(textView)
	box.PackStart(scrolled, true, true, 0)

	np := &NotesPanel{
		Box:         box,
		TextView:    textView,
		TextBuffer:  buf,
		HeaderLabel: titleLabel,
		store:       store,
	}

	// Auto-save on change (debounced 500ms)
	buf.Connect("changed", func() {
		np.scheduleAutoSave()
	})

	clearBtn.Connect("clicked", func() {
		buf.SetText("")
	})

	return np, nil
}

// LoadHostNotes loads notes for the given host
func (np *NotesPanel) LoadHostNotes(host *storage.Host) {
	np.mu.Lock()
	np.currentHost = host
	np.mu.Unlock()

	if host == nil {
		np.HeaderLabel.SetText("Заметки (нет активной сессии)")
		np.TextBuffer.SetText("")
		np.TextView.SetSensitive(false)
		return
	}

	np.TextView.SetSensitive(true)
	np.HeaderLabel.SetText("Заметки: " + host.Name)

	noteContent, err := np.store.GetNote(host.ID)
	if err == nil {
		np.TextBuffer.SetText(noteContent)
	} else {
		np.TextBuffer.SetText(host.Notes)
	}
}

func (np *NotesPanel) scheduleAutoSave() {
	np.mu.Lock()
	defer np.mu.Unlock()

	if np.currentHost == nil {
		return
	}

	if np.saveTimer != nil {
		np.saveTimer.Stop()
	}

	hostID := np.currentHost.ID
	startIter := np.TextBuffer.GetStartIter()
	endIter := np.TextBuffer.GetEndIter()
	text, _ := np.TextBuffer.GetText(startIter, endIter, true)

	np.saveTimer = time.AfterFunc(500*time.Millisecond, func() {
		_ = np.store.SaveNote(hostID, text)
	})
}
