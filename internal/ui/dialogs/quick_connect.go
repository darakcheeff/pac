package dialogs

import (
	"fmt"
	"strconv"
	"time"

	"github.com/darakcheeff/pac/internal/storage"
	"github.com/gotk3/gotk3/gtk"
)

// ShowQuickConnectDialog opens quick connection dialog
func ShowQuickConnectDialog(parent *gtk.Window, onConnect func(host *storage.Host)) {
	dlg, _ := gtk.DialogNew()
	dlg.SetTitle("Быстрое подключение")
	dlg.SetTransientFor(parent)
	dlg.SetModal(true)
	dlg.SetDefaultSize(420, 280)

	contentArea, _ := dlg.GetContentArea()
	grid, _ := gtk.GridNew()
	grid.SetRowSpacing(8)
	grid.SetColumnSpacing(8)
	grid.SetMarginStart(12)
	grid.SetMarginEnd(12)
	grid.SetMarginTop(12)
	grid.SetMarginBottom(12)

	// Port entry declared early
	entryPort, _ := gtk.EntryNew()
	entryPort.SetText("22")

	// Protocol
	lblProto, _ := gtk.LabelNew("Протокол:")
	lblProto.SetHAlign(gtk.ALIGN_END)
	comboProto, _ := gtk.ComboBoxTextNew()
	comboProto.Append("ssh", "SSH (Secure Shell)")
	comboProto.Append("telnet", "Telnet")
	comboProto.Append("serial", "Serial / COM-порт")
	comboProto.Append("local", "Локальный терминал")
	comboProto.SetActiveID("ssh")
	comboProto.Connect("changed", func() {
		p := comboProto.GetActiveID()
		if p == "telnet" {
			entryPort.SetText("23")
		} else if p == "ssh" {
			entryPort.SetText("22")
		}
	})
	grid.Attach(lblProto, 0, 0, 1, 1)
	grid.Attach(comboProto, 1, 0, 1, 1)

	// Host
	lblHost, _ := gtk.LabelNew("Хост / IP:")
	lblHost.SetHAlign(gtk.ALIGN_END)
	entryHost, _ := gtk.EntryNew()
	entryHost.SetPlaceholderText("192.168.1.1 или server.com")
	grid.Attach(lblHost, 0, 1, 1, 1)
	grid.Attach(entryHost, 1, 1, 1, 1)

	// Port
	lblPort, _ := gtk.LabelNew("Порт:")
	lblPort.SetHAlign(gtk.ALIGN_END)
	grid.Attach(lblPort, 0, 2, 1, 1)
	grid.Attach(entryPort, 1, 2, 1, 1)

	// User
	lblUser, _ := gtk.LabelNew("Пользователь:")
	lblUser.SetHAlign(gtk.ALIGN_END)
	entryUser, _ := gtk.EntryNew()
	entryUser.SetPlaceholderText("root / admin")
	grid.Attach(lblUser, 0, 3, 1, 1)
	grid.Attach(entryUser, 1, 3, 1, 1)

	// Password
	lblPass, _ := gtk.LabelNew("Пароль:")
	lblPass.SetHAlign(gtk.ALIGN_END)
	entryPass, _ := gtk.EntryNew()
	entryPass.SetVisibility(false)
	grid.Attach(lblPass, 0, 4, 1, 1)
	grid.Attach(entryPass, 1, 4, 1, 1)

	contentArea.Add(grid)

	_, _ = dlg.AddButton("Отмена", gtk.RESPONSE_CANCEL)
	btnConnect, _ := dlg.AddButton("Подключиться", gtk.RESPONSE_OK)
	btnConnect.SetCanDefault(true)
	dlg.SetDefault(btnConnect)

	dlg.ShowAll()

	if dlg.Run() == gtk.RESPONSE_OK {
		hostStr, _ := entryHost.GetText()
		userStr, _ := entryUser.GetText()
		passStr, _ := entryPass.GetText()
		portStr, _ := entryPort.GetText()
		portInt, _ := strconv.Atoi(portStr)
		if portInt == 0 {
			portInt = 22
		}

		proto := storage.Protocol(comboProto.GetActiveID())
		host := &storage.Host{
			ID:          fmt.Sprintf("quick-%d", time.Now().Unix()),
			Name:        fmt.Sprintf("%s@%s", userStr, hostStr),
			Protocol:    proto,
			Host:        hostStr,
			Port:        portInt,
			Username:    userStr,
			AuthMethod:  storage.AuthPassword,
			Password:    passStr,
			AutoSFTP:    true,
			TerminalType: "xterm-256color",
			CreatedAt:   time.Now(),
		}
		if onConnect != nil {
			onConnect(host)
		}
	}
	dlg.Destroy()
}
