package dialogs

import (
	"fmt"
	"strconv"
	"time"

	"github.com/darakcheeff/pac/internal/i18n"
	"github.com/darakcheeff/pac/internal/storage"
	"github.com/gotk3/gotk3/gtk"
)

// ShowQuickConnectDialog opens quick connection dialog
func ShowQuickConnectDialog(parent *gtk.Window, onConnect func(host *storage.Host)) {
	dlg, _ := gtk.DialogNew()
	dlg.SetTitle(i18n.T("Быстрое подключение", "Quick Connect"))
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

	// Host / Device entry
	entryHost, _ := gtk.EntryNew()
	entryHost.SetPlaceholderText(i18n.T("192.168.1.1 или server.com", "192.168.1.1 or server.com"))

	// User entry
	entryUser, _ := gtk.EntryNew()
	entryUser.SetPlaceholderText(i18n.T("root / admin", "root / admin"))

	// Password entry
	entryPass, _ := gtk.EntryNew()
	entryPass.SetVisibility(false)

	// Protocol
	lblProto, _ := gtk.LabelNew(i18n.T("Протокол:", "Protocol:"))
	lblProto.SetHAlign(gtk.ALIGN_END)
	comboProto, _ := gtk.ComboBoxTextNew()
	comboProto.Append("ssh", "SSH (Secure Shell)")
	comboProto.Append("telnet", "Telnet")
	comboProto.Append("serial", i18n.T("Serial / COM-порт", "Serial / COM Port"))
	comboProto.Append("local", i18n.T("Локальный терминал", "Local Terminal"))
	comboProto.SetActiveID("ssh")

	lblHost, _ := gtk.LabelNew(i18n.T("Хост / IP:", "Host / IP:"))
	lblHost.SetHAlign(gtk.ALIGN_END)

	lblPort, _ := gtk.LabelNew(i18n.T("Порт:", "Port:"))
	lblPort.SetHAlign(gtk.ALIGN_END)

	lblUser, _ := gtk.LabelNew(i18n.T("Пользователь:", "User:"))
	lblUser.SetHAlign(gtk.ALIGN_END)

	lblPass, _ := gtk.LabelNew(i18n.T("Пароль:", "Password:"))
	lblPass.SetHAlign(gtk.ALIGN_END)

	updateProto := func() {
		p := comboProto.GetActiveID()
		if p == "telnet" {
			lblHost.SetText(i18n.T("Хост / IP:", "Host / IP:"))
			entryHost.SetPlaceholderText("192.168.1.1")
			lblPort.SetVisible(true)
			entryPort.SetVisible(true)
			entryPort.SetText("23")
			lblUser.SetVisible(true)
			entryUser.SetVisible(true)
			lblPass.SetVisible(true)
			entryPass.SetVisible(true)
		} else if p == "ssh" {
			lblHost.SetText(i18n.T("Хост / IP:", "Host / IP:"))
			entryHost.SetPlaceholderText(i18n.T("192.168.1.1 или server.com", "192.168.1.1 or server.com"))
			lblPort.SetVisible(true)
			entryPort.SetVisible(true)
			entryPort.SetText("22")
			lblUser.SetVisible(true)
			entryUser.SetVisible(true)
			lblPass.SetVisible(true)
			entryPass.SetVisible(true)
		} else if p == "serial" {
			lblHost.SetText(i18n.T("COM-устройство:", "COM Device:"))
			entryHost.SetPlaceholderText(i18n.T("/dev/ttyUSB0 или /tmp/ttySerial0", "/dev/ttyUSB0 or /tmp/ttySerial0"))
			if entryHost.GetTextLength() == 0 {
				entryHost.SetText("/dev/ttyUSB0")
			}
			lblPort.SetVisible(false)
			entryPort.SetVisible(false)
			lblUser.SetVisible(false)
			entryUser.SetVisible(false)
			lblPass.SetVisible(false)
			entryPass.SetVisible(false)
		} else if p == "local" {
			lblHost.SetText(i18n.T("Команда / Shell:", "Command / Shell:"))
			entryHost.SetPlaceholderText("/bin/bash")
			entryHost.SetText("/bin/bash")
			lblPort.SetVisible(false)
			entryPort.SetVisible(false)
			lblUser.SetVisible(false)
			entryUser.SetVisible(false)
			lblPass.SetVisible(false)
			entryPass.SetVisible(false)
		}
	}

	comboProto.Connect("changed", func() {
		updateProto()
	})

	grid.Attach(lblProto, 0, 0, 1, 1)
	grid.Attach(comboProto, 1, 0, 1, 1)

	grid.Attach(lblHost, 0, 1, 1, 1)
	grid.Attach(entryHost, 1, 1, 1, 1)

	grid.Attach(lblPort, 0, 2, 1, 1)
	grid.Attach(entryPort, 1, 2, 1, 1)

	grid.Attach(lblUser, 0, 3, 1, 1)
	grid.Attach(entryUser, 1, 3, 1, 1)

	grid.Attach(lblPass, 0, 4, 1, 1)
	grid.Attach(entryPass, 1, 4, 1, 1)

	contentArea.Add(grid)

	_, _ = dlg.AddButton(i18n.T("Отмена", "Cancel"), gtk.RESPONSE_CANCEL)
	btnConnect, _ := dlg.AddButton(i18n.T("Подключиться", "Connect"), gtk.RESPONSE_OK)
	btnConnect.SetCanDefault(true)
	dlg.SetDefault(btnConnect)

	dlg.ShowAll()
	updateProto()

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
		name := fmt.Sprintf("%s@%s", userStr, hostStr)
		if proto == storage.ProtoSerial {
			name = fmt.Sprintf("Serial (%s)", hostStr)
		} else if proto == storage.ProtoLocal {
			name = i18n.T("Локальный терминал", "Local Terminal")
		}

		autoSFTP := true
		if proto == storage.ProtoLocal || proto == storage.ProtoSerial {
			autoSFTP = false
		}

		host := &storage.Host{
			ID:             fmt.Sprintf("quick-%d", time.Now().Unix()),
			Name:           name,
			Protocol:       proto,
			Host:           hostStr,
			Port:           portInt,
			Username:       userStr,
			AuthMethod:     storage.AuthPassword,
			Password:       passStr,
			SerialPort:     hostStr,
			SerialBaudRate: 115200,
			SerialDataBits: 8,
			SerialStopBits: 1,
			SerialParity:   "none",
			AutoSFTP:       autoSFTP,
			TerminalType:   "xterm-256color",
			CreatedAt:      time.Now(),
		}
		if onConnect != nil {
			onConnect(host)
		}
	}
	dlg.Destroy()
}
