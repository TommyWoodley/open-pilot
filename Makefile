.PHONY: build test install

build:
	go build ./cmd/open-pilot

test:
	go test ./... -count=1

install:
	go install ./cmd/open-pilot
