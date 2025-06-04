build:
	mkdir build
	go build -o build/incremental-md5 main.go

.PHONY: build
