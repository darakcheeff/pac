package ui

import (
	"sync"
	"time"

	"github.com/darakcheeff/pac/internal/session"
	"github.com/darakcheeff/pac/internal/storage"
	"github.com/gotk3/gotk3/gtk"
)

// NotesPanel represents the right-side collapsible session notes panel
type NotesPanel struct {
	Box            *gtk.Box
	TextView       *gtk.TextView
	TextBuffer     *gtk.TextBuffer
	HeaderLabel    *gtk.Label
	currentSession *session.Session
	store          *storage.Store
	saveTimer      *time.Timer
	isUpdating     bool
	lastSavedText  string
	mu             sync.Mutex
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
	titleLabel, _ := gtk.LabelNew("Заметки к вкладке")
	titleLabel.SetHAlign(gtk.ALIGN_START)
	titleLabel.SetHExpand(true)

	clearBtn, _ := gtk.ButtonNewFromIconName("edit-clear-symbolic", gtk.ICON_SIZE_BUTTON)
	clearBtn.SetTooltipText("Очистить текст заметки")

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

	// Auto-save on change (debounced 500ms) with diff check
	buf.Connect("changed", func() {
		if !np.isUpdating {
			np.scheduleAutoSave()
		}
	})

	clearBtn.Connect("clicked", func() {
		np.TextBuffer.SetText("")
	})

	return np, nil
}

// LoadSessionNotes loads notes for the active open session tab
func (np *NotesPanel) LoadSessionNotes(sess *session.Session) {
	np.mu.Lock()
	np.currentSession = sess
	np.isUpdating = true
	np.mu.Unlock()

	defer func() {
		np.mu.Lock()
		np.isUpdating = false
		np.mu.Unlock()
	}()

	if sess == nil {
		np.HeaderLabel.SetText("Заметки (нет открытых вкладок)")
		np.TextBuffer.SetText("")
		np.TextView.SetSensitive(false)
		np.lastSavedText = ""
		return
	}

	np.TextView.SetSensitive(true)
	np.HeaderLabel.SetText("Заметки: " + sess.Title)
	np.lastSavedText = sess.Notes
	np.TextBuffer.SetText(sess.Notes)
}

func (np *NotesPanel) scheduleAutoSave() {
	np.mu.Lock()
	defer np.mu.Unlock()

	if np.currentSession == nil {
		return
	}

	startIter := np.TextBuffer.GetStartIter()
	endIter := np.TextBuffer.GetEndIter()
	text, _ := np.TextBuffer.GetText(startIter, endIter, true)

	if text == np.lastSavedText {
		// No change, do not write to disk
		return
	}
	np.currentSession.Notes = text

	if np.saveTimer != nil {
		np.saveTimer.Stop()
	}

	sess := np.currentSession
	np.saveTimer = time.AfterFunc(500*time.Millisecond, func() {
		np.mu.Lock()
		if sess.Host != nil && text != np.lastSavedText {
			np.lastSavedText = text
			_ = np.store.SaveNote(sess.Host.ID, text)
		}
		np.mu.Unlock()
	})
}
