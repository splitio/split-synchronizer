#!/bin/bash

SYNC_OSX_ZIP_FILENAME=split-sync-osx-amd64.zip
SYNC_OSX_INSTALL_SCRIPT=sync_osx_install_script
SYNC_OSX_BINARY_PATH=split-sync-osx-amd64

SYNC_LINUX_ZIP_FILENAME=split-sync-linux-amd64.zip
SYNC_LINUX_INSTALL_SCRIPT=sync_linux_install_script
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

# #Compile sync
GOOS=darwin GOARCH=amd64 go build  -o ${SYNC_OSX_BINARY_PATH} ../cmd/synchronizer/main.go
GOOS=linux GOARCH=amd64 go build -o ${SYNC_LINUX_BINARY_PATH} ../cmd/synchronizer/main.go
GOOS=windows GOARCH=amd64 go build -o ${SYNC_WINDOWS_BINARY_PATH} ../cmd/synchronizer/main.go
 
#Compile proxy
GOOS=darwin GOARCH=amd64 go build -o ${PROXY_OSX_BINARY_PATH} ../cmd/proxy/main.go
GOOS=linux GOARCH=amd64 go build -o ${PROXY_LINUX_BINARY_PATH} ../cmd/proxy/main.go
GOOS=windows GOARCH=amd64 go build -o ${PROXY_WINDOWS_BINARY_PATH} ../cmd/proxy/main.go

#Compress sync binaries
zip -9 ${SYNC_OSX_ZIP_FILENAME}  ${SYNC_OSX_BINARY_PATH}
zip -9 ${SYNC_LINUX_ZIP_FILENAME} ${SYNC_LINUX_BINARY_PATH}
zip -9 ${SYNC_WINDOWS_ZIP_FILENAME}${BUILD_VERSION}.zip ${SYNC_WINDOWS_BINARY_PATH}

 #Compress proxy binaries
zip -9 ${PROXY_OSX_ZIP_FILENAME}  ${PROXY_OSX_BINARY_PATH}
zip -9 ${PROXY_LINUX_ZIP_FILENAME} ${PROXY_LINUX_BINARY_PATH}
zip -9 ${PROXY_WINDOWS_ZIP_FILENAME}${BUILD_VERSION}.zip ${PROXY_WINDOWS_BINARY_PATH}
 
TEMPLATELINES=`wc -l install_script_template | awk '{print $1}'`
TOTALLINES=$(($TEMPLATELINES + 1))
 
# Build split-sync installer for OSX
SYNC_OSX_SUM=`sum ${SYNC_OSX_ZIP_FILENAME}`
SYNC_OSX_ASUM1=`echo "${SYNC_OSX_SUM}" | awk '{print $1}'`
SYNC_OSX_ASUM2=`echo "${SYNC_OSX_SUM}" | awk '{print $2}'`
cat install_script_template \
    | sed -e "s/AUTO_REPLACE_BUILD_VERSION/${BUILD_VERSION}/" \
    | sed -e "s/AUTO_REPLACE_COMMIT_VERSION/${COMMIT_VERSION}/" \
    | sed -e "s/AUTO_REPLACE_SCRIPT_LINES/${TOTALLINES}/" \
    | sed -e "s/AUTO_REPLACE_BIN_FILENAME/${SYNC_OSX_BINARY_PATH}/" \
    | sed -e "s/AUTO_REPLACE_SUM1/${SYNC_OSX_ASUM1}/" \
    | sed -e "s/AUTO_REPLACE_SUM2/${SYNC_OSX_ASUM2}/" \
    > ${SYNC_OSX_INSTALL_SCRIPT}

# Build split-sync installer for Linux
SYNC_LINUX_SUM=`sum ${SYNC_LINUX_ZIP_FILENAME}`
SYNC_LINUX_ASUM1=`echo "${SYNC_LINUX_SUM}" | awk '{print $1}'`
SYNC_LINUX_ASUM2=`echo "${SYNC_LINUX_SUM}" | awk '{print $2}'`
cat install_script_template \
    | sed -e "s/AUTO_REPLACE_BUILD_VERSION/${BUILD_VERSION}/" \
    | sed -e "s/AUTO_REPLACE_COMMIT_VERSION/${COMMIT_VERSION}/" \
    | sed -e "s/AUTO_REPLACE_SCRIPT_LINES/${TOTALLINES}/" \
    | sed -e "s/AUTO_REPLACE_BIN_FILENAME/${SYNC_LINUX_BINARY_PATH}/" \
    | sed -e "s/AUTO_REPLACE_SUM1/${SYNC_LINUX_ASUM1}/" \
    | sed -e "s/AUTO_REPLACE_SUM2/${SYNC_LINUX_ASUM2}/" \
    > ${SYNC_LINUX_INSTALL_SCRIPT}

# Build split-proxy installer for OSX
PROXY_OSX_SUM=`sum ${PROXY_OSX_ZIP_FILENAME}`
PROXY_OSX_ASUM1=`echo "${PROXY_OSX_SUM}" | awk '{print $1}'`
PROXY_OSX_ASUM2=`echo "${PROXY_OSX_SUM}" | awk '{print $2}'`
cat install_script_template \
    | sed -e "s/AUTO_REPLACE_BUILD_VERSION/${BUILD_VERSION}/" \
    | sed -e "s/AUTO_REPLACE_COMMIT_VERSION/${COMMIT_VERSION}/" \
    | sed -e "s/AUTO_REPLACE_SCRIPT_LINES/${TOTALLINES}/" \
    | sed -e "s/AUTO_REPLACE_BIN_FILENAME/${PROXY_OSX_BINARY_PATH}/" \
    | sed -e "s/AUTO_REPLACE_SUM1/${PROXY_OSX_ASUM1}/" \
    | sed -e "s/AUTO_REPLACE_SUM2/${PROXY_OSX_ASUM2}/" \
    > ${PROXY_OSX_INSTALL_SCRIPT}

# Build split-sync installer for Linux
PROXY_LINUX_SUM=`sum ${PROXY_LINUX_ZIP_FILENAME}`
PROXY_LINUX_ASUM1=`echo "${PROXY_LINUX_SUM}" | awk '{print $1}'`
PROXY_LINUX_ASUM2=`echo "${PROXY_LINUX_SUM}" | awk '{print $2}'`
cat install_script_template \
    | sed -e "s/AUTO_REPLACE_BUILD_VERSION/${BUILD_VERSION}/" \
    | sed -e "s/AUTO_REPLACE_COMMIT_VERSION/${COMMIT_VERSION}/" \
    | sed -e "s/AUTO_REPLACE_SCRIPT_LINES/${TOTALLINES}/" \
    | sed -e "s/AUTO_REPLACE_BIN_FILENAME/${PROXY_LINUX_BINARY_PATH}/" \
    | sed -e "s/AUTO_REPLACE_SUM1/${PROXY_LINUX_ASUM1}/" \
    | sed -e "s/AUTO_REPLACE_SUM2/${PROXY_LINUX_ASUM2}/" \
    > ${PROXY_LINUX_INSTALL_SCRIPT}

# #Create installers
make
# 
# #Delete aux files
rm ${SYNC_OSX_INSTALL_SCRIPT} ${SYNC_OSX_ZIP_FILENAME} ${SYNC_LINUX_INSTALL_SCRIPT} ${SYNC_LINUX_ZIP_FILENAME}
rm ${SYNC_OSX_BINARY_PATH} ${SYNC_LINUX_BINARY_PATH} ${SYNC_WINDOWS_BINARY_PATH}
rm ${PROXY_OSX_INSTALL_SCRIPT} ${PROXY_OSX_ZIP_FILENAME} ${PROXY_LINUX_INSTALL_SCRIPT} ${PROXY_LINUX_ZIP_FILENAME}
rm ${PROXY_OSX_BINARY_PATH} ${PROXY_LINUX_BINARY_PATH} ${PROXY_WINDOWS_BINARY_PATH}
