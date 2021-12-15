# Setup defaults
GO ?= go
ZIP ?= zip
ARCH ?= amd64


# TODO(mredolatti): we can eventually have 2 sources variables (for sync & proxy), and filter out
# the go files that don't affect each binary but ain't nobody got time for that.
# when capturing the sources, we leave out `commitversion.go` since it will now be always updated by this script.
# if we considered it a dependency, we would be constantly rebuilding binaries unnecessarily
sources			:= $(shell find . -name *.go -not -name "commitversion.go")
version			:= $(shell cat splitio/version.go | grep 'const Version' | sed 's/const Version = //' | tr -d '"')
commit_version		:= $(shell git rev-parse --short HEAD)
installer_tpl		:= ./release/install_script_template
installer_tpl_lines	:= $(shell echo $$(( $$(wc -l $(installer_tpl) | awk '{print $$1}') +1 )))


# build paths:
build_prefix	:= build
exec_dir	:= $(build_prefix)/executable
zip_dir		:= $(build_prefix)/compressed
installer_dir	:= $(build_prefix)/installer


# Always update commit version
$(shell cat release/commitversion.go.template | sed -e "s/commit_version/${commit_version}/" > ./splitio/commitversion.go)


.PHONY: help clean build test build_release images_dev images_release
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
	rm -f build/{executable,compressed,installer}/*

## Build split-sync and split-proxy
build: split-sync split-proxy

## Build the split-sync executable
split-sync: $(sources) go.sum
	$(GO) build -o $@ cmd/synchronizer/main.go

## Build the split-proxy executable
split-proxy: $(sources) go.sum
	$(GO) build -o $@ cmd/proxy/main.go

## Run the unit tests
test: $(sources) go.sum
	$(GO) test ./... -count=1 -race

## Generate binaires for all architectures, ready to upload for distribution
build_release: \
    $(installer_dir)/split_sync_linux_installer.bin \
    $(installer_dir)/split_sync_darwin_installer.bin \
    $(zip_dir)/split_sync_windows.zip \
    $(installer_dir)/split_proxy_linux_installer.bin \
    $(installer_dir)/split_proxy_darwin_installer.bin \
    $(zip_dir)/split_proxy_windows.zip
	$(info )
	$(info Binaries generated:)
	$(foreach f,$^,$(info - $(f)))
	$(info )

entrypoints: \
    entrypoint.sync.sh \
    entrypoint.proxy.sh
	$(info )
	$(info Entrypoints generated:)
	$(foreach f,$^,$(info - $(f)))
	$(info )


# --------------------------------------------------------------------------
#
# Internal targets:
#
# Nothing wrong with invoking these ones from the CLI, but shouldn't be necessary
# in most cases.

go.sum: go.mod
	$(GO) mod tidy

# windows-specific declarations.
# we need to put them first to work with make <= 3.81
# (in make >=3.82 they match for being more specific, in <=3.81 they match because they appear first)
# @{
$(exec_dir)/split_sync_windows.exe: $(sources) go.sum
	GOARCH=$(ARCH) GOOS=windows $(GO) build -o $@ cmd/synchronizer/main.go

$(zip_dir)/split_sync_windows.zip: $(exec_dir)/split_sync_windows.exe
	$(ZIP) -9 --junk-paths $@ $<

$(exec_dir)/split_proxy_windows.exe: $(sources) go.sum
	GOARCH=$(ARCH) GOOS=windows $(GO) build -o $@ cmd/proxy/main.go

$(zip_dir)/split_proxy_windows.zip: $(exec_dir)/split_proxy_windows.exe
	$(ZIP) -9 --junk-paths $@ $<
# @} 

# Posix-specific declarations
# @{
$(zip_dir)/split_%.zip: $(exec_dir)/split_%
	$(ZIP) -9 --junk-paths $@ $<

$(exec_dir)/split_%: $(sources) go.sum
	GOARCH=$(ARCH) GOOS=$(call os_from_bin,$@) $(GO) build -o $@ cmd/$(call mainfolder_from_bin,$@)/main.go

$(installer_dir)/split_%_installer.bin: $(zip_dir)/split_%.zip
	cat  $(installer_tpl) \
	    | sed -e "s/AUTO_REPLACE_APP_NAME/$(call apptitle_from_zip,$<)/" \
	    | sed -e "s/AUTO_REPLACE_INSTALL_NAME/$(call progname_from_zip,$<)/" \
	    | sed -e "s/AUTO_REPLACE_BUILD_VERSION/$(version)/" \
	    | sed -e "s/AUTO_REPLACE_COMMIT_VERSION/$(commit_version)/" \
	    | sed -e "s/AUTO_REPLACE_SCRIPT_LINES/$(installer_tpl_lines)/" \
	    | sed -e "s/AUTO_REPLACE_BIN_FILENAME/$(call bin_from_zip,$<)/" \
	    | sed -e "s/AUTO_REPLACE_SUM1/$(shell sum $< | awk '{ print $$1 }')/" \
	    | sed -e "s/AUTO_REPLACE_SUM2/$(shell sum $< | awk '{ print $$2 }')/" \
	    > $@.tmp
	cat $@.tmp $< > $@
	chmod 755 $@
	rm $@.tmp
# @}


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
# Custom macros
#
# These macros are used to manipulate filenames (extract infromation from them, substitute portions, etc)
# TODO(mredolatti): we might be able to make this quite simplet with a better folder structure in `build/`
remove_ext		= $(1:%.zip=%)
remove_exec_path	= $(1:$(exec_dir)/%=%)
remove_zip_path		= $(1:$(zip_dir)/%=%)
extract_os		= $(word 3,$(subst _, ,$1))
remove_os		= $(subst _$(call extract_os,$(1)),,$(1))
progname_from_zip 	= $(subst _,-,$(call remove_os,$(call remove_ext,$(call remove_zip_path,$(1)))))
apptitle_from_zip	= $(if $(subst split-sync,,$(call progname_from_zip,$1)),Proxy,Synchronizer)
mainfolder_from_bin	= $(if $(findstring split_sync,$1),synchronizer,proxy)
os_from_bin		= $(call extract_os,$(call remove_exec_path, $(1)))
bin_from_zip		= $(call remove_zip_path,$(call remove_ext,$1))
