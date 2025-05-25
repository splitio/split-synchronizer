#!/bin/bash

HOSTNAME="localhost"
PORT=3010

rc=1
lineno=0
exec 5<> /dev/tcp/${HOSTNAME}/${PORT}
printf "GET /health/application HTTP/1.1\r\nHost: ${HOSTNAME}\r\nConnection: close\r\n\r\n" >&5
while read LINE <&5; do
    if [[ $lineno -eq 0 && ${LINE} =~ HTTP/1.1[[:space:]]200[[:space:]]OK ]]; then
	rc=0
    fi
    lineno=$((lineno+1))
done

exit $rc
