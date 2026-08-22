APP_NAME := pac
PREFIX ?= /usr/local

.PHONY: all build test clean install

all: build

build:
	go build -v -ldflags="-s -w" -o $(APP_NAME) ./cmd/pac

test:
	go test -v ./...

clean:
	rm -f $(APP_NAME)

install: build
	install -d $(DESTDIR)$(PREFIX)/bin
	install -m 755 $(APP_NAME) $(DESTDIR)$(PREFIX)/bin/$(APP_NAME)
	install -d $(DESTDIR)$(PREFIX)/share/applications
	install -m 644 pac.desktop $(DESTDIR)$(PREFIX)/share/applications/pac.desktop
