#!/bin/bash

OSX_ZIP_FILENAME=splitio-agent-osx-amd64.zip
OSX_INSTALL_SCRIPT=osx_install_script
OSX_BINARY_PATH=splitio-agent-osx-amd64

LINUX_ZIP_FILENAME=splitio-agent-linux-amd64.zip
LINUX_INSTALL_SCRIPT=linux_install_script
LINUX_BINARY_PATH=splitio-agent-linux-amd64

WINDOWS_ZIP_FILENAME=split-sync-win.zip
WINDOWS_BINARY_PATH=split-sync.exe

#Compile agent
GOOS=darwin GOARCH=amd64 go build -o ${OSX_BINARY_PATH} ..
GOOS=linux GOARCH=amd64 go build -o ${LINUX_BINARY_PATH} ..
GOOS=windows GOARCH=amd64 go build -o ${WINDOWS_BINARY_PATH} ..

#Compress binaries
zip -9 ${OSX_ZIP_FILENAME}  ${OSX_BINARY_PATH}
zip -9 ${LINUX_ZIP_FILENAME} ${LINUX_BINARY_PATH}
zip -9 ${WINDOWS_ZIP_FILENAME} ${WINDOWS_BINARY_PATH}

COMMIT_VERSION=`git rev-parse --short HEAD`
BUILD_VERSION=`tail -n 1 ../splitio/version.go | awk '{print $4}' | tr -d '"'`

TEMPLATELINES=`wc -l install_script_template | awk '{print $1}'`
TOTALLINES=$(($TEMPLATELINES + 1))

OSX_SUM=`sum ${OSX_ZIP_FILENAME}`
OSX_ASUM1=`echo "${OSX_SUM}" | awk '{print $1}'`
OSX_ASUM2=`echo "${OSX_SUM}" | awk '{print $2}'`
cat install_script_template | sed -e "s/AUTO_REPLACE_BUILD_VERSION/${BUILD_VERSION}/" | sed -e "s/AUTO_REPLACE_COMMIT_VERSION/${COMMIT_VERSION}/" | sed -e "s/AUTO_REPLACE_SCRIPT_LINES/${TOTALLINES}/" | sed -e "s/AUTO_REPLACE_BIN_FILENAME/${OSX_BINARY_PATH}/" | sed -e "s/AUTO_REPLACE_SUM1/${OSX_ASUM1}/" | sed -e "s/AUTO_REPLACE_SUM2/${OSX_ASUM2}/" > ${OSX_INSTALL_SCRIPT}

LINUX_SUM=`sum ${LINUX_ZIP_FILENAME}`
LINUX_ASUM1=`echo "${LINUX_SUM}" | awk '{print $1}'`
LINUX_ASUM2=`echo "${LINUX_SUM}" | awk '{print $2}'`
cat install_script_template | sed -e "s/AUTO_REPLACE_BUILD_VERSION/${BUILD_VERSION}/" | sed -e "s/AUTO_REPLACE_COMMIT_VERSION/${COMMIT_VERSION}/" | sed -e "s/AUTO_REPLACE_SCRIPT_LINES/${TOTALLINES}/" | sed -e "s/AUTO_REPLACE_BIN_FILENAME/${LINUX_BINARY_PATH}/" | sed -e "s/AUTO_REPLACE_SUM1/${LINUX_ASUM1}/" | sed -e "s/AUTO_REPLACE_SUM2/${LINUX_ASUM2}/" > ${LINUX_INSTALL_SCRIPT}

#Create installers
make

#Delete aux files
rm ${OSX_INSTALL_SCRIPT} ${OSX_ZIP_FILENAME} ${LINUX_INSTALL_SCRIPT} ${LINUX_ZIP_FILENAME}
