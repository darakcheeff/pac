package dialogs

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/darakcheeff/pac/internal/i18n"
	"github.com/darakcheeff/pac/internal/storage"
	"github.com/gotk3/gotk3/glib"
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
		dlg.SetTitle(i18n.T("Новое подключение", "New Connection"))
	} else {
		dlg.SetTitle(i18n.Tf("Свойства: %s", "Properties: %s", host.Name))
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
	lblName, _ := gtk.LabelNew(i18n.T("Название:", "Name:"))
	lblName.SetHAlign(gtk.ALIGN_END)
	entryName, _ := gtk.EntryNew()
	entryName.SetText(host.Name)
	entryName.SetHExpand(true)
	topGrid.Attach(lblName, 0, 0, 1, 1)
	topGrid.Attach(entryName, 1, 0, 1, 1)

	// 2. Group
	lblGroup, _ := gtk.LabelNew(i18n.T("Папка / Группа:", "Folder / Group:"))
	lblGroup.SetHAlign(gtk.ALIGN_END)
	comboGroup, _ := gtk.ComboBoxTextNew()
	for _, g := range allGroups {
		gName := g.Name
		if g.ID == "root" {
			gName = i18n.T("Все подключения", "All Connections")
		}
		comboGroup.Append(g.ID, gName)
	}
	if host.GroupID != "" {
		comboGroup.SetActiveID(host.GroupID)
	} else {
		comboGroup.SetActiveID("root")
	}
	topGrid.Attach(lblGroup, 0, 1, 1, 1)
	topGrid.Attach(comboGroup, 1, 1, 1, 1)

	// 3. Protocol
	lblProto, _ := gtk.LabelNew(i18n.T("Протокол:", "Protocol:"))
	lblProto.SetHAlign(gtk.ALIGN_END)
	comboProto, _ := gtk.ComboBoxTextNew()
	comboProto.Append("ssh", "SSH (Secure Shell)")
	comboProto.Append("telnet", "Telnet")
	comboProto.Append("serial", i18n.T("Serial / COM-порт", "Serial / COM Port"))
	comboProto.Append("local", i18n.T("Локальный терминал", "Local Terminal"))
	comboProto.SetActiveID(string(host.Protocol))
	topGrid.Attach(lblProto, 0, 2, 1, 1)
	topGrid.Attach(comboProto, 1, 2, 1, 1)

	// 4. Description
	lblDesc, _ := gtk.LabelNew(i18n.T("Описание:", "Description:"))
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
	lblHost, _ := gtk.LabelNew(i18n.T("Хост / IP-адрес:", "Host / IP Address:"))
	lblHost.SetHAlign(gtk.ALIGN_END)
	entryHost, _ := gtk.EntryNew()
	entryHost.SetText(host.Host)
	entryHost.SetHExpand(true)
	gridNet.Attach(lblHost, 0, 0, 1, 1)
	gridNet.Attach(entryHost, 1, 0, 2, 1)

	// Network: Port
	lblPort, _ := gtk.LabelNew(i18n.T("Порт:", "Port:"))
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
	lblSerialPort, _ := gtk.LabelNew(i18n.T("COM-устройство:", "COM Device:"))
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
	lblBaud, _ := gtk.LabelNew(i18n.T("Скорость (Baud Rate):", "Baud Rate:"))
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
	lblDataBits, _ := gtk.LabelNew(i18n.T("Биты данных:", "Data Bits:"))
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
	lblStopBits, _ := gtk.LabelNew(i18n.T("Стоп-биты:", "Stop Bits:"))
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
	lblParity, _ := gtk.LabelNew(i18n.T("Чётность (Parity):", "Parity:"))
	lblParity.SetHAlign(gtk.ALIGN_END)
	comboParity, _ := gtk.ComboBoxTextNew()
	comboParity.Append("none", i18n.T("None (Нет)", "None"))
	comboParity.Append("even", i18n.T("Even (Чёт)", "Even"))
	comboParity.Append("odd", i18n.T("Odd (Нечёт)", "Odd"))
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
	lblShell, _ := gtk.LabelNew(i18n.T("Командная оболочка:", "Command Shell:"))
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
	lblWorkDir, _ := gtk.LabelNew(i18n.T("Рабочая папка:", "Working Directory:"))
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
			i18n.T("Выберите рабочую папку", "Select Working Directory"),
			parent,
			gtk.FILE_CHOOSER_ACTION_SELECT_FOLDER,
			i18n.T("Отмена", "Cancel"), gtk.RESPONSE_CANCEL,
			i18n.T("Выбрать", "Select"), gtk.RESPONSE_ACCEPT,
		)
		if fc.Run() == gtk.RESPONSE_ACCEPT {
			entryWorkDir.SetText(fc.GetFilename())
		}
		fc.Destroy()
	})
	gridNet.Attach(lblWorkDir, 0, 8, 1, 1)
	gridNet.Attach(entryWorkDir, 1, 8, 1, 1)
	gridNet.Attach(btnBrowseWorkDir, 2, 8, 1, 1)

	// Authentication fields on "Основное" tab
	// Username
	lblUser, _ := gtk.LabelNew(i18n.T("Имя пользователя:", "Username:"))
	lblUser.SetHAlign(gtk.ALIGN_END)
	entryUser, _ := gtk.EntryNew()
	entryUser.SetText(host.Username)
	entryUser.SetHExpand(true)
	gridNet.Attach(lblUser, 0, 9, 1, 1)
	gridNet.Attach(entryUser, 1, 9, 2, 1)

	// Auth Method
	lblAuthM, _ := gtk.LabelNew(i18n.T("Метод авторизации:", "Auth Method:"))
	lblAuthM.SetHAlign(gtk.ALIGN_END)
	comboAuthM, _ := gtk.ComboBoxTextNew()
	comboAuthM.Append("password", i18n.T("Пароль (Password)", "Password"))
	comboAuthM.Append("key", i18n.T("Приватный SSH-ключ (Private Key)", "Private Key"))
	comboAuthM.Append("agent", "SSH Agent (SSH_AUTH_SOCK)")
	comboAuthM.Append("keyboard-interactive", i18n.T("Интерактивный ввод в терминале", "Keyboard Interactive"))
	curAuth := string(host.AuthMethod)
	if curAuth == "" {
		if host.KeyPath != "" {
			curAuth = "key"
		} else {
			curAuth = "password"
		}
	} else if curAuth == "password" && host.KeyPath != "" && host.Password == "" {
		curAuth = "key"
	}
	comboAuthM.SetActiveID(curAuth)
	gridNet.Attach(lblAuthM, 0, 10, 1, 1)
	gridNet.Attach(comboAuthM, 1, 10, 2, 1)

	// Password
	lblPass, _ := gtk.LabelNew(i18n.T("Пароль:", "Password:"))
	lblPass.SetHAlign(gtk.ALIGN_END)
	entryPass, _ := gtk.EntryNew()
	entryPass.SetVisibility(false)
	entryPass.SetText(host.Password)
	btnTogglePass, _ := gtk.ButtonNewFromIconName("eye-open-negative-filled-symbolic", gtk.ICON_SIZE_BUTTON)
	btnTogglePass.SetTooltipText(i18n.T("Показать / Скрыть пароль", "Show / Hide Password"))
	btnTogglePass.Connect("clicked", func() {
		entryPass.SetVisibility(!entryPass.GetVisibility())
	})
	gridNet.Attach(lblPass, 0, 11, 1, 1)
	gridNet.Attach(entryPass, 1, 11, 1, 1)
	gridNet.Attach(btnTogglePass, 2, 11, 1, 1)

	// Key Path
	lblKey, _ := gtk.LabelNew(i18n.T("Файл SSH-ключа:", "SSH Key File:"))
	lblKey.SetHAlign(gtk.ALIGN_END)
	entryKey, _ := gtk.EntryNew()
	entryKey.SetText(host.KeyPath)
	btnBrowseKey, _ := gtk.ButtonNewFromIconName("document-open-symbolic", gtk.ICON_SIZE_BUTTON)
	btnBrowseKey.SetTooltipText(i18n.T("Выбрать файл закрытого ключа...", "Select Private Key File..."))
	btnBrowseKey.Connect("clicked", func() {
		fc, _ := gtk.FileChooserDialogNewWith2Buttons(
			i18n.T("Выберите файл закрытого ключа SSH", "Select SSH Private Key File"),
			parent,
			gtk.FILE_CHOOSER_ACTION_OPEN,
			i18n.T("Отмена", "Cancel"), gtk.RESPONSE_CANCEL,
			i18n.T("Выбрать", "Select"), gtk.RESPONSE_ACCEPT,
		)
		if fc.Run() == gtk.RESPONSE_ACCEPT {
			entryKey.SetText(fc.GetFilename())
		}
		fc.Destroy()
	})
	gridNet.Attach(lblKey, 0, 12, 1, 1)
	gridNet.Attach(entryKey, 1, 12, 1, 1)
	gridNet.Attach(btnBrowseKey, 2, 12, 1, 1)

	// Key Passphrase
	lblKeyPass, _ := gtk.LabelNew(i18n.T("Пароль к ключу:", "Key Passphrase:"))
	lblKeyPass.SetHAlign(gtk.ALIGN_END)
	entryKeyPass, _ := gtk.EntryNew()
	entryKeyPass.SetVisibility(false)
	entryKeyPass.SetText(host.KeyPass)
	gridNet.Attach(lblKeyPass, 0, 13, 1, 1)
	gridNet.Attach(entryKeyPass, 1, 13, 2, 1)

	tabNetLabel := createTabLabel(i18n.T("Основное", "General"))
	notebook.AppendPage(gridNet, tabNetLabel)

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
	comboJumpHosts.Append("none", i18n.T("-- Без промежуточного хоста (Прямое подключение) --", "-- Direct Connection (No Jump) --"))
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

	chkX11, _ := gtk.CheckButtonNewWithLabel(i18n.T("Проброс графики X11 Forwarding (-X)", "X11 Forwarding (-X)"))
	chkX11.SetActive(host.X11Forwarding)
	gridJump.Attach(chkX11, 1, 1, 1, 1)

	chkSFTP, _ := gtk.CheckButtonNewWithLabel(i18n.T("Автоматический SFTP файловый браузер (MobaXterm Style)", "Automatic SFTP File Browser"))
	chkSFTP.SetActive(host.AutoSFTP)
	gridJump.Attach(chkSFTP, 1, 2, 1, 1)

	lblKeepAlive, _ := gtk.LabelNew(i18n.T("KeepAlive интервал (сек):", "KeepAlive interval (sec):"))
	lblKeepAlive.SetHAlign(gtk.ALIGN_END)
	spinKeepAlive, _ := gtk.SpinButtonNewWithRange(0, 3600, 5)
	kaVal := host.SSHKeepAliveInterval
	if kaVal == 0 {
		kaVal = 15
	}
	spinKeepAlive.SetValue(float64(kaVal))
	spinKeepAlive.SetTooltipText(i18n.T("Интервал отправки SSH/TCP keepalive пакетов (в секундах, 0 - выключено, по умолчанию 15)", "SSH/TCP keepalive packet interval in seconds (0 = disabled, default = 15)"))
	gridJump.Attach(lblKeepAlive, 0, 3, 1, 1)
	gridJump.Attach(spinKeepAlive, 1, 3, 1, 1)

	// Separator before Port Forwarding
	sepFwd, _ := gtk.SeparatorNew(gtk.ORIENTATION_HORIZONTAL)
	gridJump.Attach(sepFwd, 0, 4, 3, 1)

	// Port Forwarding Header
	lblFwdHeader, _ := gtk.LabelNew(i18n.T("Проброс портов (SSH Tunnels):", "Port Forwarding (SSH Tunnels):"))
	lblFwdHeader.SetHAlign(gtk.ALIGN_START)
	gridJump.Attach(lblFwdHeader, 0, 5, 3, 1)

	// Port Forwarding List & Buttons
	fwdStore, _ := gtk.ListStoreNew(glib.TYPE_STRING, glib.TYPE_INT, glib.TYPE_STRING)
	fwdView, _ := gtk.TreeViewNewWithModel(fwdStore)
	fwdView.SetHeadersVisible(true)

	rFwdType, _ := gtk.CellRendererTextNew()
	colFwdType, _ := gtk.TreeViewColumnNewWithAttribute(i18n.T("Тип", "Type"), rFwdType, "text", 0)
	colFwdType.SetMinWidth(130)
	fwdView.AppendColumn(colFwdType)

	rFwdPort, _ := gtk.CellRendererTextNew()
	colFwdPort, _ := gtk.TreeViewColumnNewWithAttribute(i18n.T("Локальный порт", "Local Port"), rFwdPort, "text", 1)
	colFwdPort.SetMinWidth(90)
	fwdView.AppendColumn(colFwdPort)

	rFwdTarget, _ := gtk.CellRendererTextNew()
	colFwdTarget, _ := gtk.TreeViewColumnNewWithAttribute(i18n.T("Удаленный адрес", "Remote Target"), rFwdTarget, "text", 2)
	colFwdTarget.SetMinWidth(140)
	fwdView.AppendColumn(colFwdTarget)

	currentForwards := make([]storage.PortForward, len(host.PortForwards))
	copy(currentForwards, host.PortForwards)

	refreshFwdStore := func() {
		fwdStore.Clear()
		for _, f := range currentForwards {
			iter := fwdStore.Append()
			typeStr := "Local (-L)"
			targetStr := fmt.Sprintf("%s:%d", f.RemoteHost, f.RemotePort)
			if f.Type == "remote" {
				typeStr = "Remote (-R)"
			} else if f.Type == "dynamic" {
				typeStr = "Dynamic SOCKS5 (-D)"
				targetStr = "-"
			}
			_ = fwdStore.SetValue(iter, 0, typeStr)
			_ = fwdStore.SetValue(iter, 1, f.LocalPort)
			_ = fwdStore.SetValue(iter, 2, targetStr)
		}
	}
	refreshFwdStore()

	fwdScroll, _ := gtk.ScrolledWindowNew(nil, nil)
	fwdScroll.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	fwdScroll.SetShadowType(gtk.SHADOW_IN)
	fwdScroll.SetSizeRequest(360, 110)
	fwdScroll.SetHExpand(true)
	fwdScroll.SetVExpand(true)
	fwdScroll.Add(fwdView)

	fwdBtnBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 4)
	btnAddFwd, _ := gtk.ButtonNewWithLabel(i18n.T("➕ Добавить...", "➕ Add..."))
	btnEditFwd, _ := gtk.ButtonNewWithLabel(i18n.T("✏ Изменить...", "✏ Edit..."))
	btnDelFwd, _ := gtk.ButtonNewWithLabel(i18n.T("🗑 Удалить", "🗑 Delete"))

	fwdBtnBox.PackStart(btnAddFwd, false, false, 0)
	fwdBtnBox.PackStart(btnEditFwd, false, false, 0)
	fwdBtnBox.PackStart(btnDelFwd, false, false, 0)

	btnAddFwd.Connect("clicked", func() {
		showPortForwardWizardDialog(parent, nil, func(f storage.PortForward) {
			currentForwards = append(currentForwards, f)
			refreshFwdStore()
		})
	})

	editSelectedFwd := func() {
		sel, _ := fwdView.GetSelection()
		_, iter, ok := sel.GetSelected()
		if ok && iter != nil {
			path, _ := fwdStore.GetPath(iter)
			idx := path.GetIndices()[0]
			if idx >= 0 && idx < len(currentForwards) {
				showPortForwardWizardDialog(parent, &currentForwards[idx], func(f storage.PortForward) {
					currentForwards[idx] = f
					refreshFwdStore()
				})
			}
		}
	}

	btnEditFwd.Connect("clicked", func() {
		editSelectedFwd()
	})

	fwdView.Connect("row-activated", func() {
		editSelectedFwd()
	})

	btnDelFwd.Connect("clicked", func() {
		sel, _ := fwdView.GetSelection()
		_, iter, ok := sel.GetSelected()
		if ok && iter != nil {
			path, _ := fwdStore.GetPath(iter)
			idx := path.GetIndices()[0]
			if idx >= 0 && idx < len(currentForwards) {
				currentForwards = append(currentForwards[:idx], currentForwards[idx+1:]...)
				refreshFwdStore()
			}
		}
	})

	fwdContainer, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 6)
	fwdContainer.PackStart(fwdScroll, true, true, 0)
	fwdContainer.PackStart(fwdBtnBox, false, false, 0)
	gridJump.Attach(fwdContainer, 0, 6, 3, 1)

	tabJumpLabel := createTabLabel(i18n.T("Туннелирование и сеть", "Tunnels & Network"))
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
	lblTermType, _ := gtk.LabelNew(i18n.T("Тип эмуляции:", "Terminal Emulation:"))
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
	lblFont, _ := gtk.LabelNew(i18n.T("Шрифт терминала:", "Terminal Font:"))
	lblFont.SetHAlign(gtk.ALIGN_END)
	curFont := host.FontName
	if curFont == "" {
		curFont = "Monospace 11"
	}
	fontBtn, _ := gtk.FontButtonNewWithFont(curFont)
	fontBtn.SetTitle(i18n.T("Выберите шрифт терминала", "Select Terminal Font"))
	gridTerm.Attach(lblFont, 0, 1, 1, 1)
	gridTerm.Attach(fontBtn, 1, 1, 1, 1)

	// Color Scheme
	lblScheme, _ := gtk.LabelNew(i18n.T("Цветовая схема:", "Color Scheme:"))
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
	lblScrollback, _ := gtk.LabelNew(i18n.T("Буфер прокрутки (строк):", "Scrollback lines:"))
	lblScrollback.SetHAlign(gtk.ALIGN_END)
	curScroll := host.ScrollbackLines
	if curScroll <= 0 {
		curScroll = 10000
	}
	spinScroll, _ := gtk.SpinButtonNewWithRange(500, 100000, 500)
	spinScroll.SetValue(float64(curScroll))
	gridTerm.Attach(lblScrollback, 0, 3, 1, 1)
	gridTerm.Attach(spinScroll, 1, 3, 1, 1)

	tabTermLabel := createTabLabel(i18n.T("Терминал и вид", "Terminal & Appearance"))
	notebook.AppendPage(gridTerm, tabTermLabel)

	// --- Tab 5: Logging & History ---
	gridLog, _ := gtk.GridNew()
	gridLog.SetRowSpacing(8)
	gridLog.SetColumnSpacing(8)
	gridLog.SetMarginStart(12)
	gridLog.SetMarginEnd(12)
	gridLog.SetMarginTop(12)
	gridLog.SetMarginBottom(12)

	chkLog, _ := gtk.CheckButtonNewWithLabel(i18n.T("Включить автоматическую запись сессии в файл лога", "Enable automatic session logging to file"))
	chkLog.SetActive(host.EnableLogging)
	gridLog.Attach(chkLog, 0, 0, 2, 1)

	lblLogPath, _ := gtk.LabelNew(i18n.T("Шаблон пути к логу:", "Log path template:"))
	lblLogPath.SetHAlign(gtk.ALIGN_END)
	entryLogPath, _ := gtk.EntryNew()
	entryLogPath.SetPlaceholderText("~/.config/pac/logs/%H_%Y%m%d_%T.log")
	entryLogPath.SetText(host.LogPathFormat)
	gridLog.Attach(lblLogPath, 0, 1, 1, 1)
	gridLog.Attach(entryLogPath, 1, 1, 1, 1)

	chkCleanANSI, _ := gtk.CheckButtonNewWithLabel(i18n.T("Очищать ANSI escape-последовательности из файла лога", "Strip ANSI escape sequences from log"))
	chkCleanANSI.SetActive(host.LogCleanANSI)
	gridLog.Attach(chkCleanANSI, 0, 2, 2, 1)

	chkRestore, _ := gtk.CheckButtonNewWithLabel(i18n.T("Восстанавливать историю экрана при перезапуске приложения", "Restore terminal scrollback on startup"))
	chkRestore.SetActive(host.RestoreHistory)
	gridLog.Attach(chkRestore, 0, 3, 2, 1)

	tabLogLabel := createTabLabel(i18n.T("Логирование", "Logging"))
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

	tabNotesLabel := createTabLabel(i18n.T("Заметки", "Notes"))
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

		// Tunnels and Jump tab visibility (SSH & Telnet)
		hasJump := proto == "ssh" || proto == "telnet"
		gridJump.SetVisible(hasJump)
		isSSH := proto == "ssh"
		lblSelectJump.SetVisible(hasJump)
		comboJumpHosts.SetVisible(hasJump)
		chkX11.SetVisible(isSSH)
		chkSFTP.SetVisible(isSSH)
		lblKeepAlive.SetVisible(isSSH)
		spinKeepAlive.SetVisible(isSSH)
		sepFwd.SetVisible(isSSH)
		lblFwdHeader.SetVisible(isSSH)
		fwdContainer.SetVisible(isSSH)

		// Dynamic authentication fields on "Основное"
		if proto != "ssh" && proto != "telnet" {
			lblUser.SetVisible(false)
			entryUser.SetVisible(false)
			lblAuthM.SetVisible(false)
			comboAuthM.SetVisible(false)
			lblPass.SetVisible(false)
			entryPass.SetVisible(false)
			btnTogglePass.SetVisible(false)
			lblKey.SetVisible(false)
			entryKey.SetVisible(false)
			btnBrowseKey.SetVisible(false)
			lblKeyPass.SetVisible(false)
			entryKeyPass.SetVisible(false)
		} else if proto == "telnet" {
			lblUser.SetVisible(true)
			entryUser.SetVisible(true)
			lblAuthM.SetVisible(false)
			comboAuthM.SetVisible(false)
			lblPass.SetVisible(true)
			entryPass.SetVisible(true)
			btnTogglePass.SetVisible(true)
			lblKey.SetVisible(false)
			entryKey.SetVisible(false)
			btnBrowseKey.SetVisible(false)
			lblKeyPass.SetVisible(false)
			entryKeyPass.SetVisible(false)
		} else { // ssh
			lblUser.SetVisible(true)
			entryUser.SetVisible(true)
			lblAuthM.SetVisible(true)
			comboAuthM.SetVisible(true)
			authMethod := comboAuthM.GetActiveID()
			showPass := authMethod == "password"
			showKey := authMethod == "key"

			lblPass.SetVisible(showPass)
			entryPass.SetVisible(showPass)
			btnTogglePass.SetVisible(showPass)

			lblKey.SetVisible(showKey)
			entryKey.SetVisible(showKey)
			btnBrowseKey.SetVisible(showKey)
			lblKeyPass.SetVisible(showKey)
			entryKeyPass.SetVisible(showKey)
		}
	}

	comboProto.Connect("changed", func() {
		updateProtocolVisibility()
	})

	comboAuthM.Connect("changed", func() {
		updateProtocolVisibility()
	})

	updateProtocolVisibility()

	_, _ = dlg.AddButton(i18n.T("Отмена", "Cancel"), gtk.RESPONSE_CANCEL)
	btnSave, _ := dlg.AddButton(i18n.T("Сохранить", "Save"), gtk.RESPONSE_OK)
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
		host.SSHKeepAliveInterval = int(spinKeepAlive.GetValue())
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

		host.PortForwards = currentForwards

		if err := store.SaveHost(host); err != nil {
			errMsg := i18n.Tf("Не удалось сохранить хост \"%s\":\n\n%s", "Failed to save host \"%s\":\n\n%s", host.Name, err.Error())
			errDlg := gtk.MessageDialogNew(parent, gtk.DIALOG_MODAL, gtk.MESSAGE_ERROR, gtk.BUTTONS_OK, "%s", errMsg)
			errDlg.SetTitle(i18n.T("Ошибка сохранения", "Save Error"))
			errDlg.Run()
			errDlg.Destroy()
			dlg.Destroy()
			return
		}
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

func showPortForwardWizardDialog(parent *gtk.Window, initial *storage.PortForward, onSave func(f storage.PortForward)) {
	dlg, _ := gtk.DialogNew()
	dlg.SetTitle(i18n.T("Мастер проброса портов", "Port Forwarding Wizard"))
	dlg.SetTransientFor(parent)
	dlg.SetModal(true)
	dlg.SetDefaultSize(400, 220)

	contentArea, _ := dlg.GetContentArea()
	grid, _ := gtk.GridNew()
	grid.SetRowSpacing(8)
	grid.SetColumnSpacing(8)
	grid.SetMarginStart(14)
	grid.SetMarginEnd(14)
	grid.SetMarginTop(14)
	grid.SetMarginBottom(14)

	lblType, _ := gtk.LabelNew(i18n.T("Тип туннеля:", "Tunnel Type:"))
	lblType.SetHAlign(gtk.ALIGN_END)
	comboType, _ := gtk.ComboBoxTextNew()
	comboType.Append("local", i18n.T("Локальный (-L: локальный порт -> сервер)", "Local (-L: local port -> remote)"))
	comboType.Append("remote", i18n.T("Удаленный (-R: порт сервера -> локальный)", "Remote (-R: remote port -> local)"))
	comboType.Append("dynamic", i18n.T("Динамический (-D: SOCKS5 прокси)", "Dynamic (-D: SOCKS5 proxy)"))
	comboType.SetActiveID("local")
	if initial != nil && initial.Type != "" {
		comboType.SetActiveID(initial.Type)
	}
	grid.Attach(lblType, 0, 0, 1, 1)
	grid.Attach(comboType, 1, 0, 1, 1)

	lblLocal, _ := gtk.LabelNew(i18n.T("Локальный порт:", "Local Port:"))
	lblLocal.SetHAlign(gtk.ALIGN_END)
	spinLocal, _ := gtk.SpinButtonNewWithRange(1, 65535, 1)
	locPort := 8080
	if initial != nil && initial.LocalPort > 0 {
		locPort = initial.LocalPort
	}
	spinLocal.SetValue(float64(locPort))
	grid.Attach(lblLocal, 0, 1, 1, 1)
	grid.Attach(spinLocal, 1, 1, 1, 1)

	lblRemoteHost, _ := gtk.LabelNew(i18n.T("Удаленный хост:", "Remote Host:"))
	lblRemoteHost.SetHAlign(gtk.ALIGN_END)
	entryRemoteHost, _ := gtk.EntryNew()
	remHost := "localhost"
	if initial != nil && initial.RemoteHost != "" {
		remHost = initial.RemoteHost
	}
	entryRemoteHost.SetText(remHost)
	grid.Attach(lblRemoteHost, 0, 2, 1, 1)
	grid.Attach(entryRemoteHost, 1, 2, 1, 1)

	lblRemotePort, _ := gtk.LabelNew(i18n.T("Удаленный порт:", "Remote Port:"))
	lblRemotePort.SetHAlign(gtk.ALIGN_END)
	spinRemotePort, _ := gtk.SpinButtonNewWithRange(1, 65535, 1)
	remPort := 80
	if initial != nil && initial.RemotePort > 0 {
		remPort = initial.RemotePort
	}
	spinRemotePort.SetValue(float64(remPort))
	grid.Attach(lblRemotePort, 0, 3, 1, 1)
	grid.Attach(spinRemotePort, 1, 3, 1, 1)

	updateWizardFields := func() {
		t := comboType.GetActiveID()
		isDyn := t == "dynamic"
		lblRemoteHost.SetVisible(!isDyn)
		entryRemoteHost.SetVisible(!isDyn)
		lblRemotePort.SetVisible(!isDyn)
		spinRemotePort.SetVisible(!isDyn)
		if isDyn {
			lblLocal.SetText(i18n.T("Порт SOCKS5:", "SOCKS5 Port:"))
		} else {
			lblLocal.SetText(i18n.T("Локальный порт:", "Local Port:"))
		}
	}
	comboType.Connect("changed", func() {
		updateWizardFields()
	})

	contentArea.Add(grid)
	_, _ = dlg.AddButton(i18n.T("Отмена", "Cancel"), gtk.RESPONSE_CANCEL)
	btnOk, _ := dlg.AddButton(i18n.T("Сохранить", "Save"), gtk.RESPONSE_OK)
	btnOk.SetCanDefault(true)
	dlg.SetDefault(btnOk)

	dlg.ShowAll()
	updateWizardFields()

	if dlg.Run() == gtk.RESPONSE_OK {
		fType := comboType.GetActiveID()
		rH, _ := entryRemoteHost.GetText()
		rP := int(spinRemotePort.GetValue())
		if fType == "dynamic" {
			rH = ""
			rP = 0
		}
		res := storage.PortForward{
			Type:       fType,
			LocalPort:  int(spinLocal.GetValue()),
			RemoteHost: rH,
			RemotePort: rP,
		}
		if onSave != nil {
			onSave(res)
		}
	}
	dlg.Destroy()
}
