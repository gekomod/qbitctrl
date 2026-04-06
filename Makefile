BINARY=qbitctrl
VERSION=1.0.0
LDFLAGS=-ldflags="-s -w -X main.Version=$(VERSION)"

.PHONY: build run clean dist install

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/server/

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY) dist/*

dist:
	mkdir -p dist
	GOOS=linux GOARCH=amd64        go build $(LDFLAGS) -o dist/$(BINARY)-linux-amd64   ./cmd/server/
	GOOS=linux GOARCH=arm64        go build $(LDFLAGS) -o dist/$(BINARY)-linux-arm64   ./cmd/server/
	GOOS=linux GOARCH=arm GOARM=7  go build $(LDFLAGS) -o dist/$(BINARY)-linux-armv7   ./cmd/server/
	GOOS=windows GOARCH=amd64      go build $(LDFLAGS) -o dist/$(BINARY)-windows.exe   ./cmd/server/
	ls -lh dist/

install: build
	cp $(BINARY) /usr/local/bin/
	cp qbitctrl.service /etc/systemd/system/
	systemctl daemon-reload
	systemctl enable --now qbitctrl
	@echo "Zainstalowano i uruchomiono qBitCtrl"
