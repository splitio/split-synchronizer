#!/bin/bash

SYNC_OSX_ZIP_FILENAME=splitio-agent-osx-amd64.zip
SYNC_OSX_INSTALL_SCRIPT=osx_install_script
SYNC_OSX_BINARY_PATH=splitio-agent-osx-amd64

SYNC_LINUX_ZIP_FILENAME=splitio-agent-linux-amd64.zip
SYNC_LINUX_INSTALL_SCRIPT=linux_install_script
SYNC_LINUX_BINARY_PATH=splitio-agent-linux-amd64

SYNC_WINDOWS_ZIP_FILENAME=split-sync-win_
SYNC_WINDOWS_BINARY_PATH=split-sync.exe

PROXY_OSX_ZIP_FILENAME=split-proxy-osx-amd64.zip
PROXY_OSX_INSTALL_SCRIPT=proxy_osx_install_script
PROXY_OSX_BINARY_PATH=split-proxy-osx-amd64

PROXY_LINUX_ZIP_FILENAME=split-proxy-linux-amd64.zip
PROXY_LINUX_INSTALL_SCRIPT=proxy_linux_install_script
PROXY_LINUX_BINARY_PATH=split-proxy-linux-amd64

PROXY_WINDOWS_ZIP_FILENAME=split-proxy-win_
PROXY_WINDOWS_BINARY_PATH=split-proxy.exe

#Versionning
COMMIT_VERSION=`git rev-parse --short HEAD`
BUILD_VERSION=`tail -n 1 ../splitio/version.go | awk '{print $4}' | tr -d '"'`

cat commitversion.go.template | sed -e "s/COMMIT_VERSION/${COMMIT_VERSION}/" > ../splitio/commitversion.go

#--- Creating versions.html
TAG_VERSIONS=`git tag -l | sort -r`

ROWS=""
for version in ${TAG_VERSIONS};
do
  if [ ! -z "$version" -a "$version" != " " -a "$version" != "1.0.0" -a "$version" != "1.0.1" -a "$version" != "${BUILD_VERSION}" ]; then
    ROW=$(cat versions.download-row.html | sed -e "s/{{VERSION}}/$version/g")
    ROWS=$ROWS$ROW
  fi
done

VERSIONS_PRE_HTML=$(<versions.pre.html)
VERSIONS_POS_HTML=$(<versions.pos.html)

echo "$VERSIONS_PRE_HTML""$ROWS""$VERSIONS_POS_HTML" > versions.html
#--- End

#Compile sync
GOOS=darwin GOARCH=amd64 go build  -o ${OSX_BINARY_PATH} ../cmd/synchronizer/main.go
GOOS=linux GOARCH=amd64 go build -o ${LINUX_BINARY_PATH} ../cmd/synchronizer/main.go
GOOS=windows GOARCH=amd64 go build -o ${WINDOWS_BINARY_PATH} ../cmd/synchronizer/main.go

#Compile proxy
GOOS=darwin GOARCH=amd64 go build -o ${PROXY_OSX_BINARY_PATH} ../cmd/proxy/main.go
GOOS=linux GOARCH=amd64 go build -o ${PROXY_LINUX_BINARY_PATH} ../cmd/proxy/main.go
GOOS=windows GOARCH=amd64 go build -o ${PROXY_WINDOWS_BINARY_PATH} ../cmd/proxy/main.go

#Compress binaries
zip -9 ${OSX_ZIP_FILENAME}  ${OSX_BINARY_PATH}
zip -9 ${LINUX_ZIP_FILENAME} ${LINUX_BINARY_PATH}
zip -9 ${WINDOWS_ZIP_FILENAME}${BUILD_VERSION}.zip ${WINDOWS_BINARY_PATH}

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
rm ${OSX_BINARY_PATH} ${LINUX_BINARY_PATH} ${WINDOWS_BINARY_PATH}
