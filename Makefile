BINARY := mip
BIN    := bin
PKG    := ./cmd/mip

.PHONY: all build test vet clean linux windows release

all: test build

build:
	go build -o $(BIN)/$(BINARY) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

linux:
	GOOS=linux GOARCH=amd64 go build -o $(BIN)/$(BINARY)-linux-amd64 $(PKG)

windows:
	GOOS=windows GOARCH=amd64 go build -o $(BIN)/$(BINARY)-windows-amd64.exe $(PKG)

# Cross-compiled release binaries for the supported platforms.
release: linux windows

clean:
	rm -rf $(BIN)
