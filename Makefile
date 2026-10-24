APP_NAME := pac
VNC_HELPER := pac-vnc
PREFIX ?= /usr/local

.PHONY: all build test clean install

all: build

build:
	go build -v -ldflags="-s -w" -o $(APP_NAME) ./cmd/pac
	go build -v -ldflags="-s -w" -o $(VNC_HELPER) ./cmd/pac-vnc

test:
	go test -v ./...

clean:
	rm -f $(APP_NAME) $(VNC_HELPER)

install: build
	install -d $(DESTDIR)$(PREFIX)/bin
	install -m 755 $(APP_NAME) $(DESTDIR)$(PREFIX)/bin/$(APP_NAME)
	install -m 755 $(VNC_HELPER) $(DESTDIR)$(PREFIX)/bin/$(VNC_HELPER)
	install -d $(DESTDIR)$(PREFIX)/share/applications
	install -m 644 pac.desktop $(DESTDIR)$(PREFIX)/share/applications/pac.desktop
	install -d $(DESTDIR)$(PREFIX)/share/pixmaps
	install -m 644 assets/icon.png $(DESTDIR)$(PREFIX)/share/pixmaps/pac.png
