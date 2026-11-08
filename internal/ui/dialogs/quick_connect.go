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
	dlg.SetModal(false)
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
	entryPort.SetActivatesDefault(true)

	// 1. Network Host / IP entry (SSH / Telnet)
	entryHost, _ := gtk.EntryNew()
	entryHost.SetPlaceholderText(i18n.T("192.168.1.1 или server.com", "192.168.1.1 or server.com"))
	entryHost.SetActivatesDefault(true)

	// 2. Serial Port Combo / Entry (with detected devices)
	comboSerialPort, _ := gtk.ComboBoxTextNewWithEntry()
	availPorts := scanSerialPorts()
	for _, p := range availPorts {
		comboSerialPort.AppendText(p)
	}
	if len(availPorts) > 0 {
		comboSerialPort.SetActive(0)
	} else {
		comboSerialPort.AppendText("/dev/ttyUSB0")
		comboSerialPort.SetActive(0)
	}

	// 3. Local Shell Command entry
	entryShell, _ := gtk.EntryNew()
	entryShell.SetPlaceholderText("/bin/bash")
	entryShell.SetText("/bin/bash")
	entryShell.SetActivatesDefault(true)

	// User entry
	entryUser, _ := gtk.EntryNew()
	entryUser.SetPlaceholderText(i18n.T("root / admin", "root / admin"))
	entryUser.SetActivatesDefault(true)

	// Password entry
	entryPass, _ := gtk.EntryNew()
	entryPass.SetVisibility(false)
	entryPass.SetActivatesDefault(true)

	// Protocol
	lblProto, _ := gtk.LabelNew(i18n.T("Протокол:", "Protocol:"))
	lblProto.SetHAlign(gtk.ALIGN_END)
	comboProto, _ := gtk.ComboBoxTextNew()
	comboProto.Append("ssh", "SSH (Secure Shell)")
	comboProto.Append("telnet", "Telnet")
	comboProto.Append("serial", i18n.T("Serial / COM-порт", "Serial / COM Port"))
	comboProto.Append("local", i18n.T("Локальный терминал", "Local Terminal"))
	comboProto.SetActiveID("ssh")

	lblTarget, _ := gtk.LabelNew(i18n.T("Хост / IP:", "Host / IP:"))
	lblTarget.SetHAlign(gtk.ALIGN_END)

	lblPort, _ := gtk.LabelNew(i18n.T("Порт:", "Port:"))
	lblPort.SetHAlign(gtk.ALIGN_END)

	lblUser, _ := gtk.LabelNew(i18n.T("Пользователь:", "User:"))
	lblUser.SetHAlign(gtk.ALIGN_END)

	lblPass, _ := gtk.LabelNew(i18n.T("Пароль:", "Password:"))
	lblPass.SetHAlign(gtk.ALIGN_END)

	updateProto := func() {
		p := comboProto.GetActiveID()

		isNet := p == "ssh" || p == "telnet"
		isSerial := p == "serial"
		isLocal := p == "local"

		// Dynamic Target Label
		if isNet {
			lblTarget.SetText(i18n.T("Хост / IP:", "Host / IP:"))
		} else if isSerial {
			lblTarget.SetText(i18n.T("COM-устройство:", "COM Device:"))
		} else if isLocal {
			lblTarget.SetText(i18n.T("Команда / Shell:", "Command / Shell:"))
		}

		// Independent Target Widgets Visibility (Best Practice: 1 widget per distinct entity)
		entryHost.SetVisible(isNet)
		comboSerialPort.SetVisible(isSerial)
		entryShell.SetVisible(isLocal)

		// Port field
		lblPort.SetVisible(isNet)
		entryPort.SetVisible(isNet)
		if p == "telnet" {
			curP, _ := entryPort.GetText()
			if curP == "" || curP == "22" {
				entryPort.SetText("23")
			}
		} else if p == "ssh" {
			curP, _ := entryPort.GetText()
			if curP == "" || curP == "23" {
				entryPort.SetText("22")
			}
		}

		// User & Password fields
		lblUser.SetVisible(isNet)
		entryUser.SetVisible(isNet)
		lblPass.SetVisible(isNet)
		entryPass.SetVisible(isNet)
	}

	comboProto.Connect("changed", func() {
		updateProto()
	})

	grid.Attach(lblProto, 0, 0, 1, 1)
	grid.Attach(comboProto, 1, 0, 1, 1)

	grid.Attach(lblTarget, 0, 1, 1, 1)
	grid.Attach(entryHost, 1, 1, 1, 1)
	grid.Attach(comboSerialPort, 1, 1, 1, 1)
	grid.Attach(entryShell, 1, 1, 1, 1)

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
		proto := storage.Protocol(comboProto.GetActiveID())

		hostStr := ""
		serialPort := ""
		if proto == storage.ProtoLocal {
			hostStr, _ = entryShell.GetText()
		} else if proto == storage.ProtoSerial {
			if devEntry, err := comboSerialPort.GetEntry(); err == nil && devEntry != nil {
				serialPort, _ = devEntry.GetText()
			} else {
				serialPort = comboSerialPort.GetActiveText()
			}
			hostStr = serialPort
		} else {
			hostStr, _ = entryHost.GetText()
		}

		userStr, _ := entryUser.GetText()
		passStr, _ := entryPass.GetText()
		portStr, _ := entryPort.GetText()
		portInt, _ := strconv.Atoi(portStr)
		if portInt == 0 {
			portInt = 22
		}

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
			SerialPort:     serialPort,
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
