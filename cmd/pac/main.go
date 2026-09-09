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

	"github.com/darakcheeff/pac/internal/storage"
	"github.com/darakcheeff/pac/internal/ui"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

func main() {
	// Pin main goroutine to OS thread for GTK/X11 message pump stability
	runtime.LockOSThread()

	dbPath := flag.String("db", "", "Path to SQLite database file")
	connectTarget := flag.String("connect", "", "Host name or ID to connect to on startup")
	flag.Parse()

	gtk.Init(nil)

	store, err := storage.NewStore(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	app, err := ui.NewAppWindow(store)
	if err != nil {
		log.Fatalf("Failed to create application window: %v", err)
	}

	// Trap termination signals (Ctrl+C, SIGTERM, SIGHUP) to save session state on exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigChan
		glib.IdleAdd(func() {
			app.Quit()
		})
	}()

	app.Window.ShowAll()

	if *connectTarget != "" {
		targets := strings.Split(*connectTarget, ",")
		hosts, _ := store.GetAllHosts()
		for _, target := range targets {
			t := strings.TrimSpace(target)
			for _, h := range hosts {
				if h.ID == t || h.Name == t {
					hostCopy := h
					glib.IdleAdd(func() {
						app.ConnectToHost(&hostCopy)
					})
					break
				}
			}
		}
	}

	fmt.Println("PAC Connection Manager NextGen started successfully.")
	gtk.Main()
}
