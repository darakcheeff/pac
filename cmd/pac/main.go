package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/darakcheeff/pac/internal/instance"
	"github.com/darakcheeff/pac/internal/storage"
	"github.com/darakcheeff/pac/internal/ui"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

func main() {
	// Pin main goroutine to OS thread for GTK/X11 message pump stability
	runtime.LockOSThread()

	pickFile := flag.Bool("pick-file", false, "Run standalone out-of-process file picker dialog")
	pickAction := flag.String("pick-action", "open", "Action: open or save")
	pickTitle := flag.String("pick-title", "Select Files", "Title for file picker")
	pickFolder := flag.String("pick-folder", "", "Initial directory")
	pickSaveName := flag.String("pick-save-name", "", "Default file name for save action")
	pickMultiple := flag.Bool("pick-multiple", false, "Allow selecting multiple files")

	dbPath := flag.String("db", "", "Path to SQLite database file")
	connectTarget := flag.String("connect", "", "Host name or ID to connect to on startup")
	flag.Parse()

	if *pickFile {
		ui.RunFilePickerStandalone(*pickAction, *pickTitle, *pickFolder, *pickSaveName, *pickMultiple)
		return
	}

	// Single instance enforcement: prevent multiple instances from running concurrently
	alreadyRunning, err := instance.TryNotifyExisting(*connectTarget)
	if err == nil && alreadyRunning {
		if *connectTarget != "" {
			fmt.Printf("PAC is already running. Requested connection to '%s' in active instance.\n", *connectTarget)
		} else {
			fmt.Println("PAC is already running. Focused existing window.")
		}
		return
	}

	gtk.Init(nil)

	store, err := storage.NewStore(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	app, err := ui.NewAppWindow(store)
	if err != nil {
		log.Fatalf("Failed to create application window: %v", err)
	}

	connectByTarget := func(target string) {
		targets := strings.Split(target, ",")
		hosts, _ := store.GetAllHosts()
		for _, t := range targets {
			trimmed := strings.TrimSpace(t)
			for _, h := range hosts {
				if h.ID == trimmed || h.Name == trimmed {
					hostCopy := h
					glib.IdleAdd(func() {
						app.ConnectToHost(&hostCopy)
					})
					break
				}
			}
		}
	}

	// Start single-instance listener to receive notifications from subsequent launches
	instListener, err := instance.StartListener(func() {
		glib.IdleAdd(func() {
			if app != nil && app.Window != nil {
				app.Window.Show()
				app.Window.Deiconify()
				app.Window.Present()
			}
		})
	}, func(target string) {
		glib.IdleAdd(func() {
			if app != nil && app.Window != nil {
				app.Window.Show()
				app.Window.Deiconify()
				app.Window.Present()
			}
			if target != "" {
				connectByTarget(target)
			}
		})
	})
	if err != nil {
		log.Printf("[SingleInstance] Warning: %v", err)
	} else {
		defer instListener.Close()
	}

	// Ignore SIGHUP so closing terminals or PTY disconnects do not kill PAC
	signal.Ignore(syscall.SIGHUP)

	// Trap termination signals (Ctrl+C, SIGTERM) to save session state on exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		if instListener != nil {
			instListener.Close()
		}
		glib.IdleAdd(func() {
			app.Quit()
		})
	}()

	app.Window.ShowAll()

	if *connectTarget != "" {
		connectByTarget(*connectTarget)
	}

	fmt.Println("PAC Connection Manager NextGen started successfully.")
	gtk.Main()
}
