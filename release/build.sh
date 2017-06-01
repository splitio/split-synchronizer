#!/bin/bash
#
# curl -L -o install_osx.bin http://downloads.split.io/synchronizer/install_osx.bin && chmod 755 install_osx.bin && ./install_osx.bin
#
# curl -L -o install_osx.bin https://s3.amazonaws.com/go-producer-stage.split.io/install_osx.bin && chmod 755 install_osx.bin && ./install_osx.bin
# curl -L -o install_linux.bin https://s3.amazonaws.com/go-producer-stage.split.io/install_linux.bin && chmod 755 install_linux.bin && ./install_linux.bin

OSX_ZIP_FILENAME=splitio-agent-osx-amd64.zip
OSX_INSTALL_SCRIPT=osx_install_script
OSX_BINARY_PATH=splitio-agent-osx-amd64

LINUX_ZIP_FILENAME=splitio-agent-linux-amd64.zip
LINUX_INSTALL_SCRIPT=linux_install_script
LINUX_BINARY_PATH=splitio-agent-linux-amd64

WINDOWS_ZIP_FILENAME=splitio-sync-win.zip
WINDOWS_BINARY_PATH=splitio-sync.exe

#Compile agent
GOOS=darwin GOARCH=amd64 go build -o ${OSX_BINARY_PATH} ..
GOOS=linux GOARCH=amd64 go build -o ${LINUX_BINARY_PATH} ..
GOOS=windows GOARCH=amd64 go build -o ${WINDOWS_BINARY_PATH} ..

#Compress binaries
zip -9 ${OSX_ZIP_FILENAME}  ${OSX_BINARY_PATH}
zip -9 ${LINUX_ZIP_FILENAME} ${LINUX_BINARY_PATH}
zip -9 ${WINDOWS_ZIP_FILENAME} ${WINDOWS_BINARY_PATH}

OSX_SUM=`sum ${OSX_ZIP_FILENAME}`
OSX_ASUM1=`echo "${OSX_SUM}" | awk '{print $1}'`
OSX_ASUM2=`echo "${OSX_SUM}" | awk '{print $2}'`
cat install_script_template | sed -e "s/AUTO_REPLACE_BIN_FILENAME/${OSX_BINARY_PATH}/" | sed -e "s/AUTO_REPLACE_SUM1/${OSX_ASUM1}/" | sed -e "s/AUTO_REPLACE_SUM2/${OSX_ASUM2}/" > ${OSX_INSTALL_SCRIPT}

LINUX_SUM=`sum ${LINUX_ZIP_FILENAME}`
LINUX_ASUM1=`echo "${LINUX_SUM}" | awk '{print $1}'`
LINUX_ASUM2=`echo "${LINUX_SUM}" | awk '{print $2}'`
cat install_script_template | sed -e "s/AUTO_REPLACE_BIN_FILENAME/${LINUX_BINARY_PATH}/" | sed -e "s/AUTO_REPLACE_SUM1/${LINUX_ASUM1}/" | sed -e "s/AUTO_REPLACE_SUM2/${LINUX_ASUM2}/" > ${LINUX_INSTALL_SCRIPT}

#Create installers
make

#Delete aux files
rm ${OSX_INSTALL_SCRIPT} ${OSX_ZIP_FILENAME} ${LINUX_INSTALL_SCRIPT} ${LINUX_ZIP_FILENAME}
