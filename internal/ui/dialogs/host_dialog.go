package dialogs

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/darakcheeff/pac/internal/storage"
	"github.com/gotk3/gotk3/gtk"
)

// scanSerialPorts detects available serial COM devices on Linux
func scanSerialPorts() []string {
	var ports []string
	matchesUSB, _ := filepath.Glob("/dev/ttyUSB*")
	ports = append(ports, matchesUSB...)
	matchesACM, _ := filepath.Glob("/dev/ttyACM*")
	ports = append(ports, matchesACM...)
	matchesS, _ := filepath.Glob("/dev/ttyS*")
	for _, m := range matchesS {
		if m == "/dev/ttyS0" || m == "/dev/ttyS1" || m == "/dev/ttyS2" || m == "/dev/ttyS3" {
			ports = append(ports, m)
		}
	}
	if len(ports) == 0 {
		ports = []string{"/dev/ttyUSB0", "/dev/ttyS0", "/dev/ttyACM0"}
	}
	return ports
}

// ShowHostEditorDialog opens connection properties editor dialog
func ShowHostEditorDialog(parent *gtk.Window, store *storage.Store, host *storage.Host, parentGroupID string, onSaved func(h *storage.Host)) {
	isNew := false
	if host == nil {
		isNew = true
		if parentGroupID == "" {
			parentGroupID = "root"
		}
		host = &storage.Host{
			ID:              fmt.Sprintf("host-%d", time.Now().UnixNano()),
			GroupID:         parentGroupID,
			Name:            "Новое подключение",
			Protocol:        storage.ProtoSSH,
			Port:            22,
			AuthMethod:      storage.AuthPassword,
			AutoSFTP:        true,
			SerialPort:      "/dev/ttyUSB0",
			SerialBaudRate:  115200,
			SerialDataBits:  8,
			SerialStopBits:  1,
			SerialParity:    "none",
			TerminalType:    "xterm-256color",
			FontName:        "Monospace 11",
			ColorScheme:     "mate",
			ScrollbackLines: 10000,
			LogCleanANSI:    true,
			RestoreHistory:  true,
			CreatedAt:       time.Now(),
		}
	}

	allHosts, _ := store.GetAllHosts()
	allGroups, _ := store.GetAllGroups()

	dlg, _ := gtk.DialogNew()
	if isNew {
		dlg.SetTitle("Новое подключение")
	} else {
		dlg.SetTitle("Свойства: " + host.Name)
	}
	dlg.SetTransientFor(parent)
	dlg.SetModal(true)
	dlg.SetDefaultSize(640, 560)

	contentArea, _ := dlg.GetContentArea()
	mainVBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 8)
	mainVBox.SetMarginStart(10)
	mainVBox.SetMarginEnd(10)
	mainVBox.SetMarginTop(10)
	mainVBox.SetMarginBottom(10)
	contentArea.Add(mainVBox)

	// Top Header Grid (Name, Group, Protocol, Description)
	topGrid, _ := gtk.GridNew()
	topGrid.SetRowSpacing(6)
	topGrid.SetColumnSpacing(8)

	// 1. Name
	lblName, _ := gtk.LabelNew("Название:")
	lblName.SetHAlign(gtk.ALIGN_END)
	entryName, _ := gtk.EntryNew()
	entryName.SetText(host.Name)
	entryName.SetHExpand(true)
	topGrid.Attach(lblName, 0, 0, 1, 1)
	topGrid.Attach(entryName, 1, 0, 1, 1)

	// 2. Group
	lblGroup, _ := gtk.LabelNew("Папка / Группа:")
	lblGroup.SetHAlign(gtk.ALIGN_END)
	comboGroup, _ := gtk.ComboBoxTextNew()
	for _, g := range allGroups {
		comboGroup.Append(g.ID, g.Name)
	}
	if host.GroupID != "" {
		comboGroup.SetActiveID(host.GroupID)
	} else {
		comboGroup.SetActiveID("root")
	}
	topGrid.Attach(lblGroup, 0, 1, 1, 1)
	topGrid.Attach(comboGroup, 1, 1, 1, 1)

	// 3. Protocol
	lblProto, _ := gtk.LabelNew("Протокол:")
	lblProto.SetHAlign(gtk.ALIGN_END)
	comboProto, _ := gtk.ComboBoxTextNew()
	comboProto.Append("ssh", "SSH (Secure Shell)")
	comboProto.Append("telnet", "Telnet")
	comboProto.Append("serial", "Serial / COM-порт")
	comboProto.Append("local", "Локальный терминал")
	comboProto.SetActiveID(string(host.Protocol))
	topGrid.Attach(lblProto, 0, 2, 1, 1)
	topGrid.Attach(comboProto, 1, 2, 1, 1)

	// 4. Description
	lblDesc, _ := gtk.LabelNew("Описание:")
	lblDesc.SetHAlign(gtk.ALIGN_END)
	entryDesc, _ := gtk.EntryNew()
	entryDesc.SetText(host.Description)
	topGrid.Attach(lblDesc, 0, 3, 1, 1)
	topGrid.Attach(entryDesc, 1, 3, 1, 1)

	mainVBox.PackStart(topGrid, false, false, 0)

	// Notebook for Tabbed Parameters
	notebook, _ := gtk.NotebookNew()
	notebook.SetScrollable(true)
	mainVBox.PackStart(notebook, true, true, 0)

	// --- Tab 1: Connection Target (Network / Serial / Local) ---
	gridNet, _ := gtk.GridNew()
	gridNet.SetRowSpacing(8)
	gridNet.SetColumnSpacing(8)
	gridNet.SetMarginStart(12)
	gridNet.SetMarginEnd(12)
	gridNet.SetMarginTop(12)
	gridNet.SetMarginBottom(12)

	// Network: Host / IP
	lblHost, _ := gtk.LabelNew("Хост / IP-адрес:")
	lblHost.SetHAlign(gtk.ALIGN_END)
	entryHost, _ := gtk.EntryNew()
	entryHost.SetText(host.Host)
	entryHost.SetHExpand(true)
	gridNet.Attach(lblHost, 0, 0, 1, 1)
	gridNet.Attach(entryHost, 1, 0, 2, 1)

	// Network: Port
	lblPort, _ := gtk.LabelNew("Порт:")
	lblPort.SetHAlign(gtk.ALIGN_END)
	entryPort, _ := gtk.EntryNew()
	if host.Port > 0 {
		entryPort.SetText(strconv.Itoa(host.Port))
	} else {
		entryPort.SetText("22")
	}
	gridNet.Attach(lblPort, 0, 1, 1, 1)
	gridNet.Attach(entryPort, 1, 1, 1, 1)

	// Serial: Port Device
	lblSerialPort, _ := gtk.LabelNew("COM-устройство:")
	lblSerialPort.SetHAlign(gtk.ALIGN_END)
	comboSerialPort, _ := gtk.ComboBoxTextNewWithEntry()
	availPorts := scanSerialPorts()
	for _, p := range availPorts {
		comboSerialPort.AppendText(p)
	}
	curDev := host.SerialPort
	if curDev == "" && len(availPorts) > 0 {
		curDev = availPorts[0]
	}
	if devEntry, err := comboSerialPort.GetEntry(); err == nil && devEntry != nil {
		devEntry.SetText(curDev)
	}
	gridNet.Attach(lblSerialPort, 0, 2, 1, 1)
	gridNet.Attach(comboSerialPort, 1, 2, 2, 1)

	// Serial: Baud Rate
	lblBaud, _ := gtk.LabelNew("Скорость (Baud Rate):")
	lblBaud.SetHAlign(gtk.ALIGN_END)
	comboBaud, _ := gtk.ComboBoxTextNew()
	for _, b := range []int{9600, 19200, 38400, 57600, 115200, 230400, 460800, 921600} {
		comboBaud.Append(strconv.Itoa(b), strconv.Itoa(b))
	}
	curBaud := host.SerialBaudRate
	if curBaud == 0 {
		curBaud = 115200
	}
	comboBaud.SetActiveID(strconv.Itoa(curBaud))
	gridNet.Attach(lblBaud, 0, 3, 1, 1)
	gridNet.Attach(comboBaud, 1, 3, 1, 1)

	// Serial: Data Bits
	lblDataBits, _ := gtk.LabelNew("Биты данных:")
	lblDataBits.SetHAlign(gtk.ALIGN_END)
	comboDataBits, _ := gtk.ComboBoxTextNew()
	for _, d := range []int{5, 6, 7, 8} {
		comboDataBits.Append(strconv.Itoa(d), strconv.Itoa(d))
	}
	curDataBits := host.SerialDataBits
	if curDataBits == 0 {
		curDataBits = 8
	}
	comboDataBits.SetActiveID(strconv.Itoa(curDataBits))
	gridNet.Attach(lblDataBits, 0, 4, 1, 1)
	gridNet.Attach(comboDataBits, 1, 4, 1, 1)

	// Serial: Stop Bits
	lblStopBits, _ := gtk.LabelNew("Стоп-биты:")
	lblStopBits.SetHAlign(gtk.ALIGN_END)
	comboStopBits, _ := gtk.ComboBoxTextNew()
	comboStopBits.Append("1", "1")
	comboStopBits.Append("1.5", "1.5")
	comboStopBits.Append("2", "2")
	curStopBits := host.SerialStopBits
	if curStopBits == 0 {
		curStopBits = 1
	}
	comboStopBits.SetActiveID(strconv.Itoa(curStopBits))
	gridNet.Attach(lblStopBits, 0, 5, 1, 1)
	gridNet.Attach(comboStopBits, 1, 5, 1, 1)

	// Serial: Parity
	lblParity, _ := gtk.LabelNew("Чётность (Parity):")
	lblParity.SetHAlign(gtk.ALIGN_END)
	comboParity, _ := gtk.ComboBoxTextNew()
	comboParity.Append("none", "None (Нет)")
	comboParity.Append("even", "Even (Чёт)")
	comboParity.Append("odd", "Odd (Нечёт)")
	comboParity.Append("mark", "Mark")
	comboParity.Append("space", "Space")
	curParity := host.SerialParity
	if curParity == "" || curParity == "N" {
		curParity = "none"
	}
	comboParity.SetActiveID(curParity)
	gridNet.Attach(lblParity, 0, 6, 1, 1)
	gridNet.Attach(comboParity, 1, 6, 1, 1)

	// Local Shell: Command
	lblShell, _ := gtk.LabelNew("Командная оболочка:")
	lblShell.SetHAlign(gtk.ALIGN_END)
	entryShell, _ := gtk.EntryNew()
	defShell := os.Getenv("SHELL")
	if defShell == "" {
		defShell = "/bin/bash"
	}
	if host.Protocol == storage.ProtoLocal && host.Host != "" {
		entryShell.SetText(host.Host)
	} else {
		entryShell.SetText(defShell)
	}
	gridNet.Attach(lblShell, 0, 7, 1, 1)
	gridNet.Attach(entryShell, 1, 7, 2, 1)

	// Local Shell: Working Directory
	lblWorkDir, _ := gtk.LabelNew("Рабочая папка:")
	lblWorkDir.SetHAlign(gtk.ALIGN_END)
	entryWorkDir, _ := gtk.EntryNew()
	entryWorkDir.SetText(host.Notes)
	if host.Protocol == storage.ProtoLocal && entryWorkDir.GetTextLength() == 0 {
		home, _ := os.UserHomeDir()
		entryWorkDir.SetText(home)
	}
	btnBrowseWorkDir, _ := gtk.ButtonNewFromIconName("folder-open-symbolic", gtk.ICON_SIZE_BUTTON)
	btnBrowseWorkDir.SetTooltipText("Выбрать рабочую папку...")
	btnBrowseWorkDir.Connect("clicked", func() {
		fc, _ := gtk.FileChooserDialogNewWith2Buttons(
			"Выберите рабочую папку",
			parent,
			gtk.FILE_CHOOSER_ACTION_SELECT_FOLDER,
			"Отмена", gtk.RESPONSE_CANCEL,
			"Выбрать", gtk.RESPONSE_ACCEPT,
		)
		if fc.Run() == gtk.RESPONSE_ACCEPT {
			entryWorkDir.SetText(fc.GetFilename())
		}
		fc.Destroy()
	})
	gridNet.Attach(lblWorkDir, 0, 8, 1, 1)
	gridNet.Attach(entryWorkDir, 1, 8, 1, 1)
	gridNet.Attach(btnBrowseWorkDir, 2, 8, 1, 1)

	tabNetLabel := createTabLabel("Параметры соединения")
	notebook.AppendPage(gridNet, tabNetLabel)

	// --- Tab 2: Authentication (SSH & Telnet) ---
	gridAuth, _ := gtk.GridNew()
	gridAuth.SetRowSpacing(8)
	gridAuth.SetColumnSpacing(8)
	gridAuth.SetMarginStart(12)
	gridAuth.SetMarginEnd(12)
	gridAuth.SetMarginTop(12)
	gridAuth.SetMarginBottom(12)

	// Username
	lblUser, _ := gtk.LabelNew("Имя пользователя:")
	lblUser.SetHAlign(gtk.ALIGN_END)
	entryUser, _ := gtk.EntryNew()
	entryUser.SetText(host.Username)
	entryUser.SetHExpand(true)
	gridAuth.Attach(lblUser, 0, 0, 1, 1)
	gridAuth.Attach(entryUser, 1, 0, 2, 1)

	// Auth Method
	lblAuthM, _ := gtk.LabelNew("Метод авторизации:")
	lblAuthM.SetHAlign(gtk.ALIGN_END)
	comboAuthM, _ := gtk.ComboBoxTextNew()
	comboAuthM.Append("password", "Пароль (Password)")
	comboAuthM.Append("key", "Приватный SSH-ключ (Private Key)")
	comboAuthM.Append("agent", "SSH Agent (SSH_AUTH_SOCK)")
	comboAuthM.Append("keyboard-interactive", "Интерактивный ввод в терминале")
	curAuth := string(host.AuthMethod)
	if curAuth == "" {
		curAuth = "password"
	}
	comboAuthM.SetActiveID(curAuth)
	gridAuth.Attach(lblAuthM, 0, 1, 1, 1)
	gridAuth.Attach(comboAuthM, 1, 1, 2, 1)

	// Password
	lblPass, _ := gtk.LabelNew("Пароль:")
	lblPass.SetHAlign(gtk.ALIGN_END)
	entryPass, _ := gtk.EntryNew()
	entryPass.SetVisibility(false)
	entryPass.SetText(host.Password)
	btnTogglePass, _ := gtk.ButtonNewFromIconName("eye-open-negative-filled-symbolic", gtk.ICON_SIZE_BUTTON)
	btnTogglePass.SetTooltipText("Показать / Скрыть пароль")
	btnTogglePass.Connect("clicked", func() {
		entryPass.SetVisibility(!entryPass.GetVisibility())
	})
	gridAuth.Attach(lblPass, 0, 2, 1, 1)
	gridAuth.Attach(entryPass, 1, 2, 1, 1)
	gridAuth.Attach(btnTogglePass, 2, 2, 1, 1)

	// Key Path
	lblKey, _ := gtk.LabelNew("Файл SSH-ключа:")
	lblKey.SetHAlign(gtk.ALIGN_END)
	entryKey, _ := gtk.EntryNew()
	entryKey.SetText(host.KeyPath)
	btnBrowseKey, _ := gtk.ButtonNewFromIconName("document-open-symbolic", gtk.ICON_SIZE_BUTTON)
	btnBrowseKey.SetTooltipText("Выбрать файл закрытого ключа...")
	btnBrowseKey.Connect("clicked", func() {
		fc, _ := gtk.FileChooserDialogNewWith2Buttons(
			"Выберите файл закрытого ключа SSH",
			parent,
			gtk.FILE_CHOOSER_ACTION_OPEN,
			"Отмена", gtk.RESPONSE_CANCEL,
			"Выбрать", gtk.RESPONSE_ACCEPT,
		)
		if fc.Run() == gtk.RESPONSE_ACCEPT {
			entryKey.SetText(fc.GetFilename())
		}
		fc.Destroy()
	})
	gridAuth.Attach(lblKey, 0, 3, 1, 1)
	gridAuth.Attach(entryKey, 1, 3, 1, 1)
	gridAuth.Attach(btnBrowseKey, 2, 3, 1, 1)

	// Key Passphrase
	lblKeyPass, _ := gtk.LabelNew("Пароль к ключу:")
	lblKeyPass.SetHAlign(gtk.ALIGN_END)
	entryKeyPass, _ := gtk.EntryNew()
	entryKeyPass.SetVisibility(false)
	entryKeyPass.SetText(host.KeyPass)
	gridAuth.Attach(lblKeyPass, 0, 4, 1, 1)
	gridAuth.Attach(entryKeyPass, 1, 4, 2, 1)

	tabAuthLabel := createTabLabel("Авторизация")
	notebook.AppendPage(gridAuth, tabAuthLabel)

	// --- Tab 3: ProxyJump & Network Advanced (SSH) ---
	gridJump, _ := gtk.GridNew()
	gridJump.SetRowSpacing(8)
	gridJump.SetColumnSpacing(8)
	gridJump.SetMarginStart(12)
	gridJump.SetMarginEnd(12)
	gridJump.SetMarginTop(12)
	gridJump.SetMarginBottom(12)

	lblSelectJump, _ := gtk.LabelNew("Bastion / ProxyJump:")
	lblSelectJump.SetHAlign(gtk.ALIGN_END)
	comboJumpHosts, _ := gtk.ComboBoxTextNew()
	comboJumpHosts.Append("none", "-- Без промежуточного хоста (Прямое подключение) --")
	comboJumpHosts.SetActiveID("none")

	for _, h := range allHosts {
		if h.ID != host.ID && h.Protocol == storage.ProtoSSH {
			comboJumpHosts.Append(h.ID, fmt.Sprintf("%s (%s@%s:%d)", h.Name, h.Username, h.Host, h.Port))
			if host.ProxyJumpHost == h.ID {
				comboJumpHosts.SetActiveID(h.ID)
			}
		}
	}
	gridJump.Attach(lblSelectJump, 0, 0, 1, 1)
	gridJump.Attach(comboJumpHosts, 1, 0, 1, 1)

	chkX11, _ := gtk.CheckButtonNewWithLabel("Проброс графики X11 Forwarding (-X)")
	chkX11.SetActive(host.X11Forwarding)
	gridJump.Attach(chkX11, 1, 1, 1, 1)

	chkSFTP, _ := gtk.CheckButtonNewWithLabel("Автоматический SFTP файловый браузер (MobaXterm Style)")
	chkSFTP.SetActive(host.AutoSFTP)
	gridJump.Attach(chkSFTP, 1, 2, 1, 1)

	tabJumpLabel := createTabLabel("Туннелирование и сеть")
	notebook.AppendPage(gridJump, tabJumpLabel)

	// --- Tab 4: Terminal & Appearance ---
	gridTerm, _ := gtk.GridNew()
	gridTerm.SetRowSpacing(8)
	gridTerm.SetColumnSpacing(8)
	gridTerm.SetMarginStart(12)
	gridTerm.SetMarginEnd(12)
	gridTerm.SetMarginTop(12)
	gridTerm.SetMarginBottom(12)

	// Terminal Type
	lblTermType, _ := gtk.LabelNew("Тип эмуляции:")
	lblTermType.SetHAlign(gtk.ALIGN_END)
	comboTermType, _ := gtk.ComboBoxTextNew()
	for _, tt := range []string{"xterm-256color", "vt100", "linux", "screen-256color", "xterm"} {
		comboTermType.Append(tt, tt)
	}
	curTT := host.TerminalType
	if curTT == "" {
		curTT = "xterm-256color"
	}
	comboTermType.SetActiveID(curTT)
	gridTerm.Attach(lblTermType, 0, 0, 1, 1)
	gridTerm.Attach(comboTermType, 1, 0, 1, 1)

	// Font Button
	lblFont, _ := gtk.LabelNew("Шрифт терминала:")
	lblFont.SetHAlign(gtk.ALIGN_END)
	curFont := host.FontName
	if curFont == "" {
		curFont = "Monospace 11"
	}
	fontBtn, _ := gtk.FontButtonNewWithFont(curFont)
	fontBtn.SetTitle("Выберите шрифт терминала")
	gridTerm.Attach(lblFont, 0, 1, 1, 1)
	gridTerm.Attach(fontBtn, 1, 1, 1, 1)

	// Color Scheme
	lblScheme, _ := gtk.LabelNew("Цветовая схема:")
	lblScheme.SetHAlign(gtk.ALIGN_END)
	comboScheme, _ := gtk.ComboBoxTextNew()
	comboScheme.Append("mate", "MATE Terminal (Default Dark)")
	comboScheme.Append("solarized-dark", "Solarized Dark")
	comboScheme.Append("dracula", "Dracula")
	comboScheme.Append("monokai", "Monokai")
	comboScheme.Append("white-on-black", "White on Black")
	comboScheme.Append("black-on-white", "Black on White")
	curScheme := host.ColorScheme
	if curScheme == "" {
		curScheme = "mate"
	}
	comboScheme.SetActiveID(curScheme)
	gridTerm.Attach(lblScheme, 0, 2, 1, 1)
	gridTerm.Attach(comboScheme, 1, 2, 1, 1)

	// Scrollback Lines
	lblScrollback, _ := gtk.LabelNew("Буфер прокрутки (строк):")
	lblScrollback.SetHAlign(gtk.ALIGN_END)
	curScroll := host.ScrollbackLines
	if curScroll <= 0 {
		curScroll = 10000
	}
	spinScroll, _ := gtk.SpinButtonNewWithRange(500, 100000, 500)
	spinScroll.SetValue(float64(curScroll))
	gridTerm.Attach(lblScrollback, 0, 3, 1, 1)
	gridTerm.Attach(spinScroll, 1, 3, 1, 1)

	tabTermLabel := createTabLabel("Терминал и вид")
	notebook.AppendPage(gridTerm, tabTermLabel)

	// --- Tab 5: Logging & History ---
	gridLog, _ := gtk.GridNew()
	gridLog.SetRowSpacing(8)
	gridLog.SetColumnSpacing(8)
	gridLog.SetMarginStart(12)
	gridLog.SetMarginEnd(12)
	gridLog.SetMarginTop(12)
	gridLog.SetMarginBottom(12)

	chkLog, _ := gtk.CheckButtonNewWithLabel("Включить автоматическую запись сессии в файл лога")
	chkLog.SetActive(host.EnableLogging)
	gridLog.Attach(chkLog, 0, 0, 2, 1)

	lblLogPath, _ := gtk.LabelNew("Шаблон пути к логу:")
	lblLogPath.SetHAlign(gtk.ALIGN_END)
	entryLogPath, _ := gtk.EntryNew()
	entryLogPath.SetPlaceholderText("~/.config/pac/logs/%H_%Y%m%d_%T.log")
	entryLogPath.SetText(host.LogPathFormat)
	gridLog.Attach(lblLogPath, 0, 1, 1, 1)
	gridLog.Attach(entryLogPath, 1, 1, 1, 1)

	chkCleanANSI, _ := gtk.CheckButtonNewWithLabel("Очищать ANSI escape-последовательности из файла лога")
	chkCleanANSI.SetActive(host.LogCleanANSI)
	gridLog.Attach(chkCleanANSI, 0, 2, 2, 1)

	chkRestore, _ := gtk.CheckButtonNewWithLabel("Восстанавливать историю экрана при перезапуске приложения")
	chkRestore.SetActive(host.RestoreHistory)
	gridLog.Attach(chkRestore, 0, 3, 2, 1)

	tabLogLabel := createTabLabel("Логирование")
	notebook.AppendPage(gridLog, tabLogLabel)

	// --- Tab 6: Notes ---
	notesBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 4)
	notesBox.SetMarginStart(10)
	notesBox.SetMarginEnd(10)
	notesBox.SetMarginTop(10)
	notesBox.SetMarginBottom(10)

	scrolledNotes, _ := gtk.ScrolledWindowNew(nil, nil)
	scrolledNotes.SetShadowType(gtk.SHADOW_IN)
	textNotes, _ := gtk.TextViewNew()
	textNotes.SetWrapMode(gtk.WRAP_WORD_CHAR)
	bufNotes, _ := textNotes.GetBuffer()
	bufNotes.SetText(host.Notes)
	scrolledNotes.Add(textNotes)
	notesBox.PackStart(scrolledNotes, true, true, 0)

	tabNotesLabel := createTabLabel("Заметки")
	notebook.AppendPage(notesBox, tabNotesLabel)

	// --- Dynamic Protocol Visibility Switcher ---
	updateProtocolVisibility := func() {
		proto := comboProto.GetActiveID()

		// Network rows (Host / Port)
		isNet := proto == "ssh" || proto == "telnet"
		lblHost.SetVisible(isNet)
		entryHost.SetVisible(isNet)
		lblPort.SetVisible(isNet)
		entryPort.SetVisible(isNet)

		if isNew {
			if proto == "telnet" {
				entryPort.SetText("23")
			} else if proto == "ssh" {
				entryPort.SetText("22")
			}
		}

		// Serial rows
		isSerial := proto == "serial"
		lblSerialPort.SetVisible(isSerial)
		comboSerialPort.SetVisible(isSerial)
		lblBaud.SetVisible(isSerial)
		comboBaud.SetVisible(isSerial)
		lblDataBits.SetVisible(isSerial)
		comboDataBits.SetVisible(isSerial)
		lblStopBits.SetVisible(isSerial)
		comboStopBits.SetVisible(isSerial)
		lblParity.SetVisible(isSerial)
		comboParity.SetVisible(isSerial)

		// Local Shell rows
		isLocal := proto == "local"
		lblShell.SetVisible(isLocal)
		entryShell.SetVisible(isLocal)
		lblWorkDir.SetVisible(isLocal)
		entryWorkDir.SetVisible(isLocal)
		btnBrowseWorkDir.SetVisible(isLocal)

		// Tabs visibility
		gridAuth.SetVisible(proto == "ssh" || proto == "telnet")
		gridJump.SetVisible(proto == "ssh")

		// Auth fields for Telnet vs SSH
		if proto == "telnet" {
			lblAuthM.SetVisible(false)
			comboAuthM.SetVisible(false)
			lblKey.SetVisible(false)
			entryKey.SetVisible(false)
			btnBrowseKey.SetVisible(false)
			lblKeyPass.SetVisible(false)
			entryKeyPass.SetVisible(false)
		} else if proto == "ssh" {
			lblAuthM.SetVisible(true)
			comboAuthM.SetVisible(true)
			lblKey.SetVisible(true)
			entryKey.SetVisible(true)
			btnBrowseKey.SetVisible(true)
			lblKeyPass.SetVisible(true)
			entryKeyPass.SetVisible(true)
		}
	}

	comboProto.Connect("changed", func() {
		updateProtocolVisibility()
	})

	updateProtocolVisibility()

	_, _ = dlg.AddButton("Отмена", gtk.RESPONSE_CANCEL)
	btnSave, _ := dlg.AddButton("Сохранить", gtk.RESPONSE_OK)
	btnSave.SetCanDefault(true)
	dlg.SetDefault(btnSave)

	dlg.ShowAll()
	updateProtocolVisibility()

	if dlg.Run() == gtk.RESPONSE_OK {
		host.Name, _ = entryName.GetText()
		host.GroupID = comboGroup.GetActiveID()
		host.Description, _ = entryDesc.GetText()
		proto := comboProto.GetActiveID()
		host.Protocol = storage.Protocol(proto)

		if proto == "ssh" || proto == "telnet" {
			host.Host, _ = entryHost.GetText()
			portStr, _ := entryPort.GetText()
			host.Port, _ = strconv.Atoi(portStr)
		} else if proto == "local" {
			host.Host, _ = entryShell.GetText()
			host.Port = 0
		} else if proto == "serial" {
			host.Host = ""
			host.Port = 0
		}

		host.Username, _ = entryUser.GetText()
		host.AuthMethod = storage.AuthMethod(comboAuthM.GetActiveID())
		host.Password, _ = entryPass.GetText()
		host.KeyPath, _ = entryKey.GetText()
		host.KeyPass, _ = entryKeyPass.GetText()

		// Serial Settings
		if devEntry, err := comboSerialPort.GetEntry(); err == nil && devEntry != nil {
			host.SerialPort, _ = devEntry.GetText()
		}
		bStr := comboBaud.GetActiveID()
		host.SerialBaudRate, _ = strconv.Atoi(bStr)
		dStr := comboDataBits.GetActiveID()
		host.SerialDataBits, _ = strconv.Atoi(dStr)
		sStr := comboStopBits.GetActiveID()
		host.SerialStopBits, _ = strconv.Atoi(sStr)
		host.SerialParity = comboParity.GetActiveID()

		// ProxyJump
		selectedJump := comboJumpHosts.GetActiveID()
		if selectedJump != "" && selectedJump != "none" {
			host.ProxyJumpHost = selectedJump
		} else {
			host.ProxyJumpHost = ""
		}

		host.X11Forwarding = chkX11.GetActive()
		host.AutoSFTP = chkSFTP.GetActive()
		host.TerminalType = comboTermType.GetActiveID()
		host.FontName = fontBtn.GetFont()
		host.ColorScheme = comboScheme.GetActiveID()
		host.ScrollbackLines = int(spinScroll.GetValue())
		host.EnableLogging = chkLog.GetActive()
		host.LogPathFormat, _ = entryLogPath.GetText()
		host.LogCleanANSI = chkCleanANSI.GetActive()
		host.RestoreHistory = chkRestore.GetActive()

		startIter := bufNotes.GetStartIter()
		endIter := bufNotes.GetEndIter()
		notesText, _ := bufNotes.GetText(startIter, endIter, false)
		host.Notes = notesText

		_ = store.SaveHost(host)
		if onSaved != nil {
			onSaved(host)
		}
	}
	dlg.Destroy()
}

func createTabLabel(text string) *gtk.Label {
	lbl, _ := gtk.LabelNew(text)
	return lbl
}
