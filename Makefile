SHELL := /bin/bash
PATH := bin:$(PATH)

PKG := `go list ./... | grep -v /vendor/`
MAIN := ws2http.go

ifeq ($(RACE),1)
	GOFLAGS=-race
endif

VERSION?=$(shell git version > /dev/null 2>&1 && git describe --dirty=-dirty --always 2>/dev/null || echo NO_VERSION)
LDFLAGS=-ldflags "-X=main.Version=$(VERSION)"

all: fix build

fix:
	@go get $(PKG)

fmt:
	@go fmt $(PKG)

vet:
	@go vet $(PKG)

build:
	@go build $(LDFLAGS) $(GOFLAGS) $(MAIN)

clean:
	@rm -rf ws2http

run:
	@echo "Compiling"
	@go run $(LDFLAGS) $(GOFLAGS) $(MAIN) -trace -route /rpc:http://localhost/rpc/

test:
	@go test $(GOFLAGS) $(PKG)

test-short:
	@go test -v -test.short -test.run="Test[^D][^B]" $(GOFLAGS) $(PKG)