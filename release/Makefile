OSX_SCRIPT_FILE=osx_install_script
OSX_BIN_FILE=splitio-agent-osx-amd64.zip

LINUX_SCRIPT_FILE=linux_install_script
LINUX_BIN_FILE=splitio-agent-linux-amd64.zip

BUILD_VERSION=$(shell tail -n 1 ../splitio/version.go | awk '{print $$4}' | tr -d '"')

all: install_script.bin

install_script.bin:
	cat ${OSX_SCRIPT_FILE} ${OSX_BIN_FILE} > install_osx_${BUILD_VERSION}.bin
	chmod 755 install_osx_${BUILD_VERSION}.bin
	cat ${LINUX_SCRIPT_FILE} ${LINUX_BIN_FILE} > install_linux_${BUILD_VERSION}.bin
	chmod 755 install_linux_${BUILD_VERSION}.bin

clean:
	rm -f install_osx.bin install_linux.bin
