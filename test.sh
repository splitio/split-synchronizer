#!/bin/bash

go test -cover $(go list ./... | grep -v /vendor/)
