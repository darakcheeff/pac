package ui

import (
	"github.com/darakcheeff/pac/internal/session"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

// BroadcastBar represents the cluster input bar
type BroadcastBar struct {
	Box         *gtk.Box
	Entry       *gtk.Entry
	TargetCombo *gtk.ComboBoxText
	SendBtn     *gtk.Button
	CloseBtn    *gtk.Button
	manager     *session.Manager
}

func NewBroadcastBar(manager *session.Manager) (*BroadcastBar, error) {
	box, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 6)
	if err != nil {
		return nil, err
	}
	box.SetMarginStart(6)
	box.SetMarginEnd(6)
	box.SetMarginTop(4)
	box.SetMarginBottom(4)

	label, _ := gtk.LabelNew("Кластерный ввод:")
	box.PackStart(label, false, false, 0)

	combo, _ := gtk.ComboBoxTextNew()
	combo.Append("all", "Во все открытые сессии")
	combo.SetActiveID("all")
	box.PackStart(combo, false, false, 0)

	entry, _ := gtk.EntryNew()
	entry.SetPlaceholderText("Введите команду и нажмите Enter для отправки...")
	entry.SetHExpand(true)
	box.PackStart(entry, true, true, 0)

	sendBtn, _ := gtk.ButtonNewFromIconName("mail-send-symbolic", gtk.ICON_SIZE_BUTTON)
	sendBtn.SetTooltipText("Отправить команду (Enter)")
	box.PackStart(sendBtn, false, false, 0)

	closeBtn, _ := gtk.ButtonNewFromIconName("window-close-symbolic", gtk.ICON_SIZE_BUTTON)
	closeBtn.SetTooltipText("Скрыть панель")
	box.PackEnd(closeBtn, false, false, 0)

	bar := &BroadcastBar{
		Box:         box,
		Entry:       entry,
		TargetCombo: combo,
		SendBtn:     sendBtn,
		CloseBtn:    closeBtn,
		manager:     manager,
	}

	sendAction := func() {
		text, _ := entry.GetText()
		if text != "" {
			manager.BroadcastInput(text+"\n", nil)
			entry.SetText("")
		}
	}

	entry.Connect("activate", func() {
		sendAction()
	})

	sendBtn.Connect("clicked", func() {
		sendAction()
	})

	closeBtn.Connect("clicked", func() {
		box.Hide()
	})

	// Keyboard escape to close
	entry.Connect("key-press-event", func(_ *gtk.Entry, event *gdk.Event) bool {
		keyEvent := gdk.EventKeyNewFromEvent(event)
		if keyEvent.KeyVal() == gdk.KEY_Escape {
			box.Hide()
			return true
		}
		return false
	})

	return bar, nil
}
