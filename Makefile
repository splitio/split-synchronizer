# Setup defaults
GO ?= go
ZIP ?= zip
ARCH ?= amd64
PYTHON ?= python3
DOCKER ?= docker
BUILD ?= build
SHELL = /usr/bin/env bash -o pipefail

# Extra arguments
EXTRA_BUILD_ARGS ?=

# don't depend on commit version, to avoid rebuilding unnecessarily
sources			:= $(shell find . -name *.go -not -name "commitversion.go")
version			:= $(shell cat splitio/version.go | grep 'const Version' | sed 's/const Version = //' | tr -d '"')
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

## Run the unit tests
test: $(sources) go.sum
	$(GO) test ./... -count=1 -race $(ARGS)

### Run unit tests and generate coverage output
test_coverage: $(sources) go.sum
	$(GO) test -v -cover -coverprofile=coverage.out $(ARGS) ./...

## Generate binaires for all architectures, ready to upload for distribution (with and without version)
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
	$(DOCKER) build -t splitsoftware/split-synchronizer:latest -t splitsoftware/split-synchronizer:$(version) -f docker/Dockerfile.synchronizer .
	$(DOCKER) build -t splitsoftware/split-proxy:latest -t splitsoftware/split-proxy:$(version) -f docker/Dockerfile.proxy .
	@echo "Images created. Make sure everything works ok, and then run the following commands to push them."
	@echo "$(DOCKER) push splitsoftware/split-synchronizer:$(version)"
	@echo "$(DOCKER) push splitsoftware/split-synchronizer:latest"
	@echo "$(DOCKER) push splitsoftware/split-proxy:$(version)"
	@echo "$(DOCKER) push splitsoftware/split-proxy:latest"

# --------------------------------------------------------------------------
#
# Internal targets:
#
# Nothing wrong with invoking these ones from the CLI, but shouldn't be necessary
# in most cases.

go.sum: go.mod
	$(GO) mod tidy

# because of windows .exe suffix, we need a macro on the right side, which needs to be executed
# after the `%` evaluation, therefore, in a second expansion
.SECONDEXPANSION:
$(BUILD)/split_%.zip: $(BUILD)/split_$$(call make_exec,%)
	$(ZIP) -9 --junk-paths $@ $<

$(BUILD)/install_split_%.bin: $(BUILD)/split_%.zip
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

execs := split_sync_linux split_sync_osx split_sync_windows.exe split_proxy_linux split_proxy_osx split_proxy_windows.exe
.INTERMEDIATE: $(addprefix $(BUILD)/,$(execs))
$(addprefix $(BUILD)/,$(execs)): $(BUILD)/split_%: $(sources) go.sum
	GOARCH=$(ARCH) GOOS=$(call parse_os,$@) $(GO) build -o $@ cmd/$(call cmdfolder_from_bin,$@)/main.go

entrypoint.%.sh: clilist
	cat docker/entrypoint.sh.tpl \
	    | sed 's/{{ARGS}}/$(shell ./clilist -target=$*)/' \
	    | sed 's/{{PREFIX}}/SPLIT_$(call to_uppercase,$(if $(findstring synchronizer,$*),sync,proxy))/' \
	    | sed 's/{{EXECUTABLE}}/split-$(if $(findstring synchronizer,$*),sync,proxy)/' \
	    > $@
	chmod +x $@

$(BUILD)/synchronizer: \
    $(BUILD)/downloads.sync.html \
    $(BUILD)/install_split_sync_linux.bin \
    $(BUILD)/install_split_sync_osx.bin \
    $(BUILD)/split_sync_windows.zip
	mkdir -p $(BUILD)/synchronizer/$(version)
	for f in $^; do cp $$f $(BUILD)/synchronizer/$(version)/$$(basename "$${f%.*}")_$(version).$${f##*.}; done
	cp {$(subst $(space),$(comma),$^)} $(BUILD)/synchronizer

$(BUILD)/proxy: \
    $(BUILD)/downloads.proxy.html \
    $(BUILD)/install_split_proxy_linux.bin \
    $(BUILD)/install_split_proxy_osx.bin \
    $(BUILD)/split_proxy_windows.zip
	mkdir -p $(BUILD)/proxy/$(version)
	for f in $^; do cp $$f $(BUILD)/proxy/$(version)/$$(basename "$${f%.*}")_$(version).$${f##*.}; done
	cp {$(subst $(space),$(comma),$^)} $(BUILD)/proxy

$(BUILD)/downloads.%.html:
	$(PYTHON) release/dp_gen.py --app $* > $@

table_header:
	@echo "| **Command line option** | **JSON option** | **Environment variable** (container-only) | **Description** |"
	@echo "| --- | --- | --- | --- |"

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
parse_os		= $(call normalize_os,$(word 3,$(subst _, ,$(call remove_ext_path,$1))))
make_exec		= $(if $(findstring windows,$1),$1.exe,$1)
installed_from_zip 	= $(if $(findstring split_sync,$1),split-sync,split-proxy)
apptitle_from_zip	= $(if $(findstring split_sync,$1),Synchronizer,Proxy)
cmdfolder_from_bin	= $(if $(findstring split_sync,$1),synchronizer,proxy)

# "constants"
null  :=
space := $(null) #
comma := ,
