package dialogs

import (
	"fmt"
	"strconv"
	"time"

	"github.com/darakcheeff/pac/internal/storage"
	"github.com/gotk3/gotk3/gtk"
)

// ShowHostEditorDialog opens connection properties editor dialog
func ShowHostEditorDialog(parent *gtk.Window, store *storage.Store, host *storage.Host, parentGroupID string, onSaved func(h *storage.Host)) {
	isNew := false
	if host == nil {
		isNew = true
		host = &storage.Host{
			ID:              fmt.Sprintf("host-%d", time.Now().UnixNano()),
			GroupID:         parentGroupID,
			Name:            "Новое подключение",
			Protocol:        storage.ProtoSSH,
			Port:            22,
			AuthMethod:      storage.AuthPassword,
			AutoSFTP:        true,
			TerminalType:    "xterm-256color",
			ScrollbackLines: 10000,
			LogCleanANSI:    true,
			RestoreHistory:  true,
			CreatedAt:       time.Now(),
		}
	}

	allHosts, _ := store.GetAllHosts()

	dlg, _ := gtk.DialogNew()
	if isNew {
		dlg.SetTitle("Новое подключение")
	} else {
		dlg.SetTitle("Свойства: " + host.Name)
	}
	dlg.SetTransientFor(parent)
	dlg.SetModal(true)
	dlg.SetDefaultSize(580, 520)

	contentArea, _ := dlg.GetContentArea()
	notebook, _ := gtk.NotebookNew()

	// --- Tab 1: General & Network ---
	gridGen, _ := gtk.GridNew()
	gridGen.SetRowSpacing(8)
	gridGen.SetColumnSpacing(8)
	gridGen.SetMarginStart(12)
	gridGen.SetMarginEnd(12)
	gridGen.SetMarginTop(12)
	gridGen.SetMarginBottom(12)

	// Name
	lblName, _ := gtk.LabelNew("Название:")
	lblName.SetHAlign(gtk.ALIGN_END)
	entryName, _ := gtk.EntryNew()
	entryName.SetText(host.Name)
	entryName.SetHExpand(true)
	gridGen.Attach(lblName, 0, 0, 1, 1)
	gridGen.Attach(entryName, 1, 0, 1, 1)

	// Protocol
	lblProto, _ := gtk.LabelNew("Протокол:")
	lblProto.SetHAlign(gtk.ALIGN_END)
	comboProto, _ := gtk.ComboBoxTextNew()
	comboProto.Append("ssh", "SSH (Secure Shell)")
	comboProto.Append("telnet", "Telnet")
	comboProto.Append("serial", "Serial / COM Port")
	comboProto.Append("local", "Локальный терминал")
	comboProto.SetActiveID(string(host.Protocol))
	gridGen.Attach(lblProto, 0, 1, 1, 1)
	gridGen.Attach(comboProto, 1, 1, 1, 1)

	// Host / IP
	lblHost, _ := gtk.LabelNew("Хост / IP:")
	lblHost.SetHAlign(gtk.ALIGN_END)
	entryHost, _ := gtk.EntryNew()
	entryHost.SetText(host.Host)
	gridGen.Attach(lblHost, 0, 2, 1, 1)
	gridGen.Attach(entryHost, 1, 2, 1, 1)

	// Port
	lblPort, _ := gtk.LabelNew("Порт:")
	lblPort.SetHAlign(gtk.ALIGN_END)
	entryPort, _ := gtk.EntryNew()
	entryPort.SetText(strconv.Itoa(host.Port))
	gridGen.Attach(lblPort, 0, 3, 1, 1)
	gridGen.Attach(entryPort, 1, 3, 1, 1)

	// Description
	lblDesc, _ := gtk.LabelNew("Описание:")
	lblDesc.SetHAlign(gtk.ALIGN_END)
	entryDesc, _ := gtk.EntryNew()
	entryDesc.SetText(host.Description)
	gridGen.Attach(lblDesc, 0, 4, 1, 1)
	gridGen.Attach(entryDesc, 1, 4, 1, 1)

	notebook.AppendPage(gridGen, createTabLabel("Основные"))

	// --- Tab 2: Authentication ---
	gridAuth, _ := gtk.GridNew()
	gridAuth.SetRowSpacing(8)
	gridAuth.SetColumnSpacing(8)
	gridAuth.SetMarginStart(12)
	gridAuth.SetMarginEnd(12)
	gridAuth.SetMarginTop(12)
	gridAuth.SetMarginBottom(12)

	// Username
	lblUser, _ := gtk.LabelNew("Пользователь:")
	lblUser.SetHAlign(gtk.ALIGN_END)
	entryUser, _ := gtk.EntryNew()
	entryUser.SetText(host.Username)
	entryUser.SetHExpand(true)
	gridAuth.Attach(lblUser, 0, 0, 1, 1)
	gridAuth.Attach(entryUser, 1, 0, 1, 1)

	// Auth Method
	lblAuthM, _ := gtk.LabelNew("Метод авторизации:")
	lblAuthM.SetHAlign(gtk.ALIGN_END)
	comboAuthM, _ := gtk.ComboBoxTextNew()
	comboAuthM.Append("password", "Пароль")
	comboAuthM.Append("key", "Приватный ключ (SSH Key)")
	comboAuthM.Append("agent", "SSH Agent (SSH_AUTH_SOCK)")
	comboAuthM.SetActiveID(string(host.AuthMethod))
	gridAuth.Attach(lblAuthM, 0, 1, 1, 1)
	gridAuth.Attach(comboAuthM, 1, 1, 1, 1)

	// Password
	lblPass, _ := gtk.LabelNew("Пароль:")
	lblPass.SetHAlign(gtk.ALIGN_END)
	entryPass, _ := gtk.EntryNew()
	entryPass.SetVisibility(false)
	entryPass.SetText(host.Password)
	gridAuth.Attach(lblPass, 0, 2, 1, 1)
	gridAuth.Attach(entryPass, 1, 2, 1, 1)

	// Key Path
	lblKey, _ := gtk.LabelNew("Путь к SSH-ключу:")
	lblKey.SetHAlign(gtk.ALIGN_END)
	entryKey, _ := gtk.EntryNew()
	entryKey.SetText(host.KeyPath)
	gridAuth.Attach(lblKey, 0, 3, 1, 1)
	gridAuth.Attach(entryKey, 1, 3, 1, 1)

	notebook.AppendPage(gridAuth, createTabLabel("Авторизация"))

	// --- Tab 3: ProxyJump / Bastion ---
	gridJump, _ := gtk.GridNew()
	gridJump.SetRowSpacing(8)
	gridJump.SetColumnSpacing(8)
	gridJump.SetMarginStart(12)
	gridJump.SetMarginEnd(12)
	gridJump.SetMarginTop(12)
	gridJump.SetMarginBottom(12)

	lblJumpInfo, _ := gtk.LabelNew("Подключение через промежуточный Jump / Bastion сервер:")
	lblJumpInfo.SetHAlign(gtk.ALIGN_START)
	gridJump.Attach(lblJumpInfo, 0, 0, 2, 1)

	lblSelectJump, _ := gtk.LabelNew("Сохраненный хост:")
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
	gridJump.Attach(lblSelectJump, 0, 1, 1, 1)
	gridJump.Attach(comboJumpHosts, 1, 1, 1, 1)

	lblCustomJump, _ := gtk.LabelNew("Или вручную (user@bastion:port):")
	lblCustomJump.SetHAlign(gtk.ALIGN_END)
	entryCustomJump, _ := gtk.EntryNew()
	entryCustomJump.SetPlaceholderText("bastion_user@bastion.corp.net:22")
	if host.ProxyJumpHost != "" && comboJumpHosts.GetActiveID() == "none" {
		entryCustomJump.SetText(host.ProxyJumpHost)
	}
	gridJump.Attach(lblCustomJump, 0, 2, 1, 1)
	gridJump.Attach(entryCustomJump, 1, 2, 1, 1)

	notebook.AppendPage(gridJump, createTabLabel("ProxyJump / Bastion"))

	// --- Tab 4: Advanced (SSH, SFTP, X11, Logging) ---
	gridAdv, _ := gtk.GridNew()
	gridAdv.SetRowSpacing(8)
	gridAdv.SetColumnSpacing(8)
	gridAdv.SetMarginStart(12)
	gridAdv.SetMarginEnd(12)
	gridAdv.SetMarginTop(12)
	gridAdv.SetMarginBottom(12)

	chkX11, _ := gtk.CheckButtonNewWithLabel("Проброс графики X11 Forwarding (-X)")
	chkX11.SetActive(host.X11Forwarding)
	gridAdv.Attach(chkX11, 0, 0, 2, 1)

	chkSFTP, _ := gtk.CheckButtonNewWithLabel("Авто-открытие SFTP браузера (MobaXterm Style)")
	chkSFTP.SetActive(host.AutoSFTP)
	gridAdv.Attach(chkSFTP, 0, 1, 2, 1)

	chkLog, _ := gtk.CheckButtonNewWithLabel("Автосохранение сессии в TXT-файл лога")
	chkLog.SetActive(host.EnableLogging)
	gridAdv.Attach(chkLog, 0, 2, 2, 1)

	chkRestore, _ := gtk.CheckButtonNewWithLabel("Восстанавливать историю терминала при рестарте")
	chkRestore.SetActive(host.RestoreHistory)
	gridAdv.Attach(chkRestore, 0, 3, 2, 1)

	notebook.AppendPage(gridAdv, createTabLabel("Дополнительно"))

	contentArea.Add(notebook)

	_, _ = dlg.AddButton("Отмена", gtk.RESPONSE_CANCEL)
	btnSave, _ := dlg.AddButton("Сохранить", gtk.RESPONSE_OK)
	btnSave.SetCanDefault(true)
	dlg.SetDefault(btnSave)

	dlg.ShowAll()

	if dlg.Run() == gtk.RESPONSE_OK {
		host.Name, _ = entryName.GetText()
		host.Description, _ = entryDesc.GetText()
		host.Protocol = storage.Protocol(comboProto.GetActiveID())
		host.Host, _ = entryHost.GetText()
		portStr, _ := entryPort.GetText()
		host.Port, _ = strconv.Atoi(portStr)
		host.Username, _ = entryUser.GetText()
		host.AuthMethod = storage.AuthMethod(comboAuthM.GetActiveID())
		host.Password, _ = entryPass.GetText()
		host.KeyPath, _ = entryKey.GetText()
		host.X11Forwarding = chkX11.GetActive()
		host.AutoSFTP = chkSFTP.GetActive()
		host.EnableLogging = chkLog.GetActive()
		host.RestoreHistory = chkRestore.GetActive()

		// Save ProxyJump
		selectedJump := comboJumpHosts.GetActiveID()
		customJump, _ := entryCustomJump.GetText()
		if selectedJump != "" && selectedJump != "none" {
			host.ProxyJumpHost = selectedJump
		} else if customJump != "" {
			host.ProxyJumpHost = customJump
		} else {
			host.ProxyJumpHost = ""
		}

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
