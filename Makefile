# Setup defaults
GO ?= go
MAKE ?= make
ZIP ?= zip
ARCH ?= amd64
PYTHON ?= python3
DOCKER ?= docker
BUILD ?= build
BUILD_FIPS ?= $(BUILD)/fips
BUILD_FIPS_WIN_TMP ?= windows/build
SHELL = /usr/bin/env bash -o pipefail
ENFORCE_FIPS := -tags enforce_fips
CURRENT_OS = $(shell uname -a | awk '{print $$1}')
PLATFORM ?=

# Extra arguments
EXTRA_BUILD_ARGS ?=

# don't depend on commit version, to avoid rebuilding unnecessarily
sources				:= $(shell find . -name *.go -not -name "commitversion.go")
version				:= $(shell cat splitio/version.go | grep 'const Version' | sed 's/const Version = //' | tr -d '"')
commit_version		:= $(shell git rev-parse --short HEAD)
installer_tpl		:= ./release/install_script_template
installer_tpl_lines	:= $(shell echo $$(( $$(wc -l $(installer_tpl) | awk '{print $$1}') +1 )))

# Always update commit version
$(shell cat release/commitversion.go.template | sed -e "s/COMMIT_VERSION/${commit_version}/" > ./splitio/commitversion.go)

.PHONY: help clean build test test_coverage release_assets images_release \
    sync_options_table proxy_options_table download_pages table_header

default: help

# --------------------------------------------------------------------------
#
# Main targets:
#
# These targets express common actions and are meant to be invoked directly by devs

## Delete all build files (both final & temporary)
clean:
	rm -f ./split-sync
	rm -f ./split-proxy
	rm -f ./entrypoint.*.sh
	rm -f ./clilist
	rm -Rf $(BUILD)/*

## Build split-sync and split-proxy
build: split-sync split-proxy

## Build the split-sync executable
split-sync: $(sources) go.sum
	$(GO) build $(EXTRA_BUILD_ARGS) -o $@ cmd/synchronizer/main.go

## Build the split-proxy executable
split-proxy: $(sources) go.sum
	$(GO) build $(EXTRA_BUILD_ARGS) -o $@ cmd/proxy/main.go

## Build the split-sync executable
split-sync-fips: $(sources) go.sum
	GOEXPERIMENT=boringcrypto $(GO) build $(EXTRA_BUILD_ARGS) -o $@ $(ENFORCE_FIPS) cmd/synchronizer/main.go

## Build the split-proxy executable
split-proxy-fips: $(sources) go.sum
	GOEXPERIMENT=boringcrypto $(GO) build $(EXTRA_BUILD_ARGS) -o $@ $(ENFORCE_FIPS) cmd/proxy/main.go

## Run the unit tests
test: $(sources) go.sum
	$(GO) test ./... -count=1 -race $(ARGS)

### Run unit tests and generate coverage output
test_coverage: $(sources) go.sum
	$(GO) test -v -cover -coverprofile=coverage.out $(ARGS) ./...

## display unit test coverage derived from last test run (use `make test display-coverage` for up-to-date results)
display-coverage: coverage.out
	go tool cover -html=coverage.out

## Generate binaries for all architectures, ready to upload for distribution (with and without version)
release_assets: \
    $(BUILD)/synchronizer \
    $(BUILD)/proxy
	$(info )
	$(info Release files generated:)
	$(foreach f,$^,$(info - $(f)))
	$(info )

# Build internal tool for parsing & extracting info from proxy & syncrhonizer config structs
clilist: $(sources)
	$(GO) build $(EXTRA_BUILD_ARGS) -o $@ docker/util/clilist/main.go

## Generate download pages for split-sync & split-proxy
download_pages: $(BUILD)/downloads.proxy.html $(BUILD)/downloads.sync.html

## Generate proxy config options table
proxy_options_table: clilist $(sources) table_header
	@./clilist -target=proxy -env-prefix=SPLIT_PROXY_ -output="| {cli} | {json} | {env} | {desc} |\n"

## Generate synchronizer config options table
sync_options_table: clilist $(sources) table_header
	@./clilist -target=synchronizer -env-prefix=SPLIT_SYNC_ -output="| {cli} | {json} | {env} | {desc} |\n"

## Generate entrypoints for docker images
entrypoints: entrypoint.synchronizer.sh entrypoint.proxy.sh

## Build release-ready docker images with proper tags and output push commands in stdout
images_release: # entrypoints
	$(DOCKER) build $(platform_str) \
		-t splitsoftware/split-synchronizer:latest -t splitsoftware/split-synchronizer:$(version) \
		-f docker/Dockerfile.synchronizer .
	$(DOCKER) build $(platform_str) \
		-t splitsoftware/split-proxy:latest -t splitsoftware/split-proxy:$(version) \
		-f docker/Dockerfile.proxy .
	$(DOCKER) build $(platform_str) \
		-t splitsoftware/split-synchronizer-fips:latest -t splitsoftware/split-synchronizer-fips:$(version) \
		--build-arg FIPS_MODE=1 \
		-f docker/Dockerfile.synchronizer .
	$(DOCKER) build $(platform_str) \
		-t splitsoftware/split-proxy-fips:latest -t splitsoftware/split-proxy-fips:$(version) \
		--build-arg FIPS_MODE=1 \
		-f docker/Dockerfile.proxy .
	@echo "Images created. Make sure everything works ok, and then run the following commands to push them."
	@echo "$(DOCKER) push splitsoftware/split-synchronizer:$(version)"
	@echo "$(DOCKER) push splitsoftware/split-synchronizer:latest"
	@echo "$(DOCKER) push splitsoftware/split-proxy:$(version)"
	@echo "$(DOCKER) push splitsoftware/split-proxy:latest"
	@echo "$(DOCKER) push splitsoftware/split-synchronizer-fips:$(version)"
	@echo "$(DOCKER) push splitsoftware/split-synchronizer-fips:latest"
	@echo "$(DOCKER) push splitsoftware/split-proxy-fips:$(version)"
	@echo "$(DOCKER) push splitsoftware/split-proxy-fips:latest"

# --------------------------------------------------------------------------
#
# Internal targets:
#
# Nothing wrong with invoking these ones from the CLI, but shouldn't be necessary
# in most cases.

go.sum: go.mod
	$(GO) mod tidy

coverage.out: test_coverage

# because of windows .exe suffix, we need a macro on the right side, which needs to be executed
# after the `%` evaluation, therefore, in a second expansion
.SECONDEXPANSION:
%.zip: $$(call mkexec,%)
	$(ZIP) -9 --junk-paths $@ $<

# factorized installer creation since it cannot be combined into a single
# target for both std & fips-compliant builds
define make-installer
	cat  $(installer_tpl) \
	    | sed -e "s/AUTO_REPLACE_APP_NAME/$(call apptitle_from_zip,$<)/" \
	    | sed -e "s/AUTO_REPLACE_INSTALL_NAME/$(call installed_from_zip,$<)/" \
	    | sed -e "s/AUTO_REPLACE_BUILD_VERSION/$(version)/" \
	    | sed -e "s/AUTO_REPLACE_COMMIT_VERSION/$(commit_version)/" \
	    | sed -e "s/AUTO_REPLACE_SCRIPT_LINES/$(installer_tpl_lines)/" \
	    | sed -e "s/AUTO_REPLACE_BIN_FILENAME/$(call remove_ext_path,$<)/" \
	    | sed -e "s/AUTO_REPLACE_SUM1/$(shell sum $< | awk '{ print $$1 }')/" \
	    | sed -e "s/AUTO_REPLACE_SUM2/$(shell sum $< | awk '{ print $$2 }')/" \
	    > $@.tmp
	cat $@.tmp $< > $@
	chmod 755 $@
	rm $@.tmp
	rm $<
endef

$(BUILD)/install_split_%.bin: $(BUILD)/split_%.zip
	$(make-installer)

$(BUILD_FIPS)/install_split_%.bin: $(BUILD_FIPS)/split_%.zip
	$(make-installer)

# Recipes to build main binaries (both std & fips-compliant)
# @{
posix_execs := split_sync_linux split_sync_osx split_proxy_linux split_proxy_osx
windows_execs := split_sync_windows.exe split_proxy_windows.exe
execs := $(posix_execs) $(windows_execs)
.INTERMEDIATE: $(addprefix $(BUILD)/,$(execs)) $(addprefix $(BUILD_FIPS)/,$(execs))

# regular binaries recipe
$(addprefix $(BUILD)/,$(execs)): $(BUILD)/split_%: $(sources) go.sum
	CGO_ENABLED=0 GOARCH=$(ARCH) GOOS=$(call parse_os,$@) $(GO) build -o $@ cmd/$(call cmdfolder_from_bin,$@)/main.go

# fips-compliant posix binaries recipe
$(addprefix $(BUILD_FIPS)/,$(posix_execs)): $(BUILD_FIPS)/split_%: $(sources) go.sum
	mkdir -p $(BUILD_FIPS)
	GOEXPERIMENT=boringcrypto CGO_ENABLED=0 GOARCH=$(ARCH) GOOS=$(call parse_os,$@) $(GO) build $(ENFORCE_FIPS) -o $@ cmd/$(call cmdfolder_from_bin,$@)/main.go

# fips-compliant windows binaries recipe
ifeq ($(CURRENT_OS),Darwin) # we're on macos, we need to build using a dockerized linux
$(addprefix $(BUILD_FIPS)/,$(windows_execs)): $(BUILD_FIPS)/split_%: $(sources) go.sum
	mkdir -p $(BUILD_FIPS)
	bash -c 'pushd windows && ./build_from_mac.sh'
	cp $(BUILD_FIPS_WIN_TMP)/$(shell basename $@) $(BUILD_FIPS)
else
$(addprefix $(BUILD_FIPS)/,$(windows_execs)): $(BUILD_FIPS)/split_%: $(sources) go.sum
	mkdir -p $(BUILD_FIPS) # we're on linux, we can build natively
	$(MAKE) -f Makefile -C ./windows setup_ms_go binaries
	cp $(BUILD_FIPS_WIN_TMP)/$(shell basename $@) $(BUILD_FIPS)
endif
# @}

entrypoint.%.sh: clilist
	cat docker/entrypoint.sh.tpl \
	    | sed 's/{{ARGS}}/$(shell ./clilist -target=$*)/' \
	    | sed 's/{{PREFIX}}/SPLIT_$(call to_uppercase,$(if $(findstring synchronizer,$*),sync,proxy))/' \
	    | sed 's/{{EXECUTABLE}}/split-$(if $(findstring synchronizer,$*),sync,proxy)/' \
	    > $@
	chmod +x $@

define copy-release-binaries
	for f in $^; do \
		if [[ $$(dirname "$$f") == $(BUILD) ]]; then \
			cp $$f $@/$(version)/$$(basename "$${f%.*}")_$(version).$${f##*.}; \
			mv $$f $@; \
		elif [[ $$(dirname "$$f") == $(BUILD_FIPS) ]]; then \
			cp $$f $@/$(version)/$$(basename "$${f%.*}")_fips_$(version).$${f##*.}; \
			mv $$f $@/$$(basename "$${f%.*}")_fips.$${f##*.}; \
		fi \
	done
endef


$(BUILD)/synchronizer: \
    $(BUILD)/downloads.sync.html \
    $(BUILD)/install_split_sync_linux.bin \
    $(BUILD)/install_split_sync_osx.bin \
    $(BUILD)/split_sync_windows.zip \
    $(BUILD_FIPS)/install_split_sync_linux.bin \
    $(BUILD_FIPS)/install_split_sync_osx.bin \
    $(BUILD_FIPS)/split_sync_windows.zip

	mkdir -p $(BUILD)/synchronizer/$(version)
	cp $(BUILD)/downloads.sync.html $(BUILD)/synchronizer
	$(copy-release-binaries)


$(BUILD)/proxy: \
    $(BUILD)/downloads.proxy.html \
    $(BUILD)/install_split_proxy_linux.bin \
    $(BUILD)/install_split_proxy_osx.bin \
    $(BUILD)/split_proxy_windows.zip \
    $(BUILD_FIPS)/install_split_proxy_linux.bin \
    $(BUILD_FIPS)/install_split_proxy_osx.bin \
    $(BUILD_FIPS)/split_proxy_windows.zip

	mkdir -p $(BUILD)/proxy/$(version)
	cp $(BUILD)/downloads.proxy.html $(BUILD)/proxy
	$(copy-release-binaries)

$(BUILD)/downloads.%.html:
	$(PYTHON) release/dp_gen.py --app $* > $@

table_header:
	@echo "| **Command line option** | **JSON option** | **Environment variable** (container-only) | **Description** |"
	@echo "| --- | --- | --- | --- |"

coverage.out: test_coverage



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

# --------------------------------------------------------------------------
#
# Custom helper macros
#
# These macros are used to manipulate filenames (extract infromation from them, substitute portions, etc)
to_uppercase		= $(shell echo '$1' | tr a-z A-Z)
remove_ext_path		= $(basename $(notdir $1))
normalize_os		= $(if $(subst osx,,$1),$1,darwin)
parse_os			= $(call normalize_os,$(word 3,$(subst _, ,$(call remove_ext_path,$1))))
mkexec				= $(if $(findstring windows,$1),$1.exe,$1)
installed_from_zip	= $(if $(findstring split_sync,$1),split-sync,split-proxy)
apptitle_from_zip	= $(if $(findstring split_sync,$1),Synchronizer,Proxy)
cmdfolder_from_bin	= $(if $(findstring split_sync,$1),synchronizer,proxy)
platform_str		= $(if $(PLATFORM),--platform=$(PLATFORM),)

# "constants"
null  :=
space := $(null) #
comma := ,
