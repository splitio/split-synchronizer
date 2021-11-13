SYNC_OSX_SCRIPT_FILE=sync_osx_install_script
SYNC_OSX_BIN_FILE=split-sync-osx-amd64.zip
SYNC_LINUX_SCRIPT_FILE=sync_linux_install_script
SYNC_LINUX_BIN_FILE=split-sync-linux-amd64.zip

PROXY_OSX_SCRIPT_FILE=proxy_osx_install_script
PROXY_OSX_BIN_FILE=split-proxy-osx-amd64.zip
PROXY_LINUX_SCRIPT_FILE=proxy_linux_install_script
PROXY_LINUX_BIN_FILE=split-proxy-linux-amd64.zip

BUILD_VERSION=$(shell tail -n 1 ../splitio/version.go | awk '{print $$4}' | tr -d '"')

all: install_scripts

install_scripts:
	# Sync
	cat ${SYNC_OSX_SCRIPT_FILE} ${SYNC_OSX_BIN_FILE} > install_sync_osx_${BUILD_VERSION}.bin
	chmod 755 install_sync_osx_${BUILD_VERSION}.bin
	cat ${SYNC_LINUX_SCRIPT_FILE} ${SYNC_LINUX_BIN_FILE} > install_sync_linux_${BUILD_VERSION}.bin
	chmod 755 install_sync_linux_${BUILD_VERSION}.bin
	# Proxy
	cat ${PROXY_OSX_SCRIPT_FILE} ${PROXY_OSX_BIN_FILE} > install_proxy_osx_${BUILD_VERSION}.bin
	chmod 755 install_proxy_osx_${BUILD_VERSION}.bin
	cat ${PROXY_LINUX_SCRIPT_FILE} ${PROXY_LINUX_BIN_FILE} > install_proxy_linux_${BUILD_VERSION}.bin
	chmod 755 install_proxy_linux_${BUILD_VERSION}.bin

clean:
	rm -f install_sync_osx_${BUILD_VERSION}.bin install_sync_linux_${BUILD_VERSION}.bin
	rm -f install_proxy_osx_${BUILD_VERSION}.bin install_proxy_linux_${BUILD_VERSION}.bin
