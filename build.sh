#!/bin/bash

#Compile agent
GOOS=windows GOARCH=amd64 go build -o bin/splitio-agent-windows-amd64.exe
GOOS=linux GOARCH=amd64 go build -o bin/splitio-agent-linux-amd64
GOOS=darwin GOARCH=amd64 go build -o bin/splitio-agent-osx-amd64

#Compress binaries
#zip -9 release/splitio-agent-osx-amd64.zip bin/splitio-agent-osx-amd64
#zip -9 release/splitio-agent-linux-amd64.zip bin/splitio-agent-linux-amd64


#Create installers
