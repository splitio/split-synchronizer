#!/bin/bash

GOOS=windows GOARCH=amd64 go build -o bin/splitio-agent-windows-amd64.exe
GOOS=linux GOARCH=amd64 go build -o bin/splitio-agent-linux-amd64
GOOS=darwin GOARCH=amd64 go build -o bin/splitio-agent-osx-amd64
