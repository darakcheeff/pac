package ui

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/darakcheeff/pac/internal/i18n"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

// RunFilePickerStandalone executes a standalone, isolated file chooser dialog in its own process
func RunFilePickerStandalone(action, title, initialFolder, defaultSaveName string, multiple bool) {
	gtk.Init(nil)

	gtkAction := gtk.FILE_CHOOSER_ACTION_OPEN
	confirmLabel := i18n.T("Выбрать", "Select")
	if action == "save" {
		gtkAction = gtk.FILE_CHOOSER_ACTION_SAVE
		confirmLabel = i18n.T("Сохранить", "Save")
	} else if action == "folder" {
		gtkAction = gtk.FILE_CHOOSER_ACTION_SELECT_FOLDER
		confirmLabel = i18n.T("Выбрать", "Select")
	}

	dlg, err := gtk.FileChooserDialogNewWith2Buttons(
		title,
		nil,
		gtkAction,
		i18n.T("Отмена", "Cancel"), gtk.RESPONSE_CANCEL,
		confirmLabel, gtk.RESPONSE_ACCEPT,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating file chooser dialog: %v\n", err)
		os.Exit(2)
	}

	// Always set explicit directory to prevent GIO/GVFS recent:/// scanning hangs
	targetFolder := initialFolder
	if targetFolder == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			targetFolder = home
		} else {
			targetFolder = "."
		}
	}
	if fi, err := os.Stat(targetFolder); err == nil && !fi.IsDir() {
		targetFolder = filepath.Dir(targetFolder)
	}
	dlg.SetCurrentFolder(targetFolder)

	if action == "save" {
		if defaultSaveName != "" {
			dlg.SetCurrentName(defaultSaveName)
		}
		dlg.SetDoOverwriteConfirmation(true)
	} else if multiple {
		dlg.SetSelectMultiple(true)
	}

	dlg.ShowAll()
	resp := dlg.Run()

	if resp == gtk.RESPONSE_ACCEPT {
		if multiple && action != "save" {
			filenames, err := dlg.GetFilenames()
			if err == nil {
				for _, fn := range filenames {
					fmt.Println(fn)
				}
			}
		} else {
			fn := dlg.GetFilename()
			if fn != "" {
				fmt.Println(fn)
			}
		}
		dlg.Destroy()
		os.Exit(0)
	}

	dlg.Destroy()
	os.Exit(1)
}

// PickFilesOutOfProcess runs the file chooser in an isolated child process to guarantee main process immunity
func PickFilesOutOfProcess(action, title, initialFolder, defaultSaveName string, multiple bool, onSelected func(paths []string)) {
	go func() {
		exe, err := os.Executable()
		if err != nil || exe == "" {
			exe = os.Args[0]
		}

		args := []string{
			"--pick-file",
			"--pick-action=" + action,
			"--pick-title=" + title,
		}
		if initialFolder != "" {
			args = append(args, "--pick-folder="+initialFolder)
		}
		if defaultSaveName != "" {
			args = append(args, "--pick-save-name="+defaultSaveName)
		}
		if multiple {
			args = append(args, "--pick-multiple")
		}

		cmd := exec.Command(exe, args...)
		out, err := cmd.Output()
		if err != nil {
			log.Printf("[Picker] Out-of-process file picker exited/cancelled: %v", err)
			return
		}

		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		var selected []string
		for _, l := range lines {
			if trimmed := strings.TrimSpace(l); trimmed != "" {
				selected = append(selected, trimmed)
			}
		}

		if len(selected) > 0 {
			glib.IdleAdd(func() {
				onSelected(selected)
			})
		}
	}()
}
