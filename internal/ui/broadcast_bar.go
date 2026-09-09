package ui

import (
	"github.com/darakcheeff/pac/internal/i18n"
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

	label, _ := gtk.LabelNew(i18n.T("Кластерный ввод:", "Cluster Input:"))
	box.PackStart(label, false, false, 0)

	combo, _ := gtk.ComboBoxTextNew()
	combo.Append("all", i18n.T("Во все открытые сессии", "To all open sessions"))
	combo.SetActiveID("all")
	box.PackStart(combo, false, false, 0)

	entry, _ := gtk.EntryNew()
	entry.SetPlaceholderText(i18n.T("Введите команду и нажмите Enter для отправки...", "Enter command and press Enter to broadcast..."))
	entry.SetHExpand(true)
	box.PackStart(entry, true, true, 0)

	sendBtn, _ := gtk.ButtonNewFromIconName("mail-send-symbolic", gtk.ICON_SIZE_BUTTON)
	sendBtn.SetTooltipText(i18n.T("Отправить команду (Enter)", "Send command (Enter)"))
	box.PackStart(sendBtn, false, false, 0)

	closeBtn, _ := gtk.ButtonNewFromIconName("window-close-symbolic", gtk.ICON_SIZE_BUTTON)
	closeBtn.SetTooltipText(i18n.T("Скрыть панель", "Close panel"))
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
