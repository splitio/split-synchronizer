CURRENT_PATH := $(shell pwd)
PARENT_PATH := $(shell dirname $(CURRENT_PATH))
DOWNLOAD_FOLDER := $(CURRENT_PATH)/downloads
UNPACK_FOLDER := $(CURRENT_PATH)/unpacked
BIN_FOLDER := $(CURRENT_PATH)/unpacked/go/bin
BUILD_FOLDER := $(CURRENT_PATH)/build


GO := $(BIN_FOLDER)/go
ASSET ?= go1.24.4.linux-amd64.tar.gz
SOURCES := $(shell find $(PARENT_PATH) -path $(dirname $(pwd))/windows -prune -o -name "*.go" -print) \
		   $(PARENT_PATH)/go.mod \
		   $(PARENT_PATH)/go.sum

SYNC_BIN := split_sync_windows.exe
PROXY_BIN := split_proxy_windows.exe

.PHONY: clean setup_ms_go

default: help

## remove all downloaded/unpacked/generated files
clean:
	rm -Rf downloads unpacked build

## download and setup a ms-patched version of go which is fips-compliant for windows
setup_ms_go: $(UNPACK_FOLDER)/go

## build fips-compliant split-proxy && split-sync
binaries: $(BUILD_FOLDER)/$(SYNC_BIN) $(BUILD_FOLDER)/$(PROXY_BIN)


# --------


$(DOWNLOAD_FOLDER)/$(ASSET):
	mkdir -p $(DOWNLOAD_FOLDER)
	wget https://aka.ms/golang/release/latest/$(ASSET) --directory-prefix $(DOWNLOAD_FOLDER)
	# wget https://aka.ms/golang/release/latest/$(ASSET).sha256 --directory-prefix $(DOWNLOAD_FOLDER)
	# TODO(mredolatti): validate sha256

$(UNPACK_FOLDER)/go: $(DOWNLOAD_FOLDER)/$(ASSET)
	mkdir -p $(UNPACK_FOLDER)
	tar xvzf $(DOWNLOAD_FOLDER)/$(ASSET) --directory $(UNPACK_FOLDER)

$(BUILD_FOLDER)/$(PROXY_BIN): $(GO) $(SOURCES)
	mkdir -p $(BUILD_FOLDER)
	GOOS=windows GOEXPERIMENT=cngcrypto $(GO) build -tags=enforce_fips -o $@ $(PARENT_PATH)/cmd/proxy/main.go

$(BUILD_FOLDER)/$(SYNC_BIN): $(GO) $(SOURCES)
	mkdir -p $(BUILD_FOLDER)
	GOOS=windows GOEXPERIMENT=cngcrypto $(GO) build -tags=enforce_fips -o $@ $(PARENT_PATH)/cmd/synchronizer/main.go

# Help target borrowed from: https://docs.cloudposse.com/reference/best-practices/make-best-practices/
## This help screen
help:
	@printf "Available targets:\n\n"
	@awk '/^[a-zA-Z\-\_0-9%:\\]+/ { \
	    helpMessage = match(lastLine, /^## (.*)/); \
		if (helpMessage) { \
		    helpCommand = $$1; \
		    helpMessage = substr(lastLine, RSTART + 3, RLENGTH); \
		    gsub("\\\\", "", helpCommand); \
		    gsub(":+$$", "", helpCommand); \
		    printf "  \x1b[32;01m%-35s\x1b[0m %s\n", helpCommand, helpMessage; \
		} \
	    } \
	    { lastLine = $$0 }' $(MAKEFILE_LIST) | sort -u
	@printf "\n"
