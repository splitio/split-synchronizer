#!/usr/bin/env bash

.PHONY: all clean

default: all
all: clean root proxy admin
root: ca.key ca.crt
proxy: proxy.key proxy.crt
admin: admin.key admin.crt

clean:
	rm -Rf *.crt *.pem *.csr *.key *.old *.attr *.index *.serial

ca.db.index:
	touch $@

# ROOT CA
ca.key ca.crt: ca.db.index
	openssl rand -hex 16 > ca.serial
	openssl genrsa \
		-aes256 \
		-passout pass:some_passphrase \
		-out ca.key 4096
	openssl req \
		-key ca.key \
		-new \
		-x509 \
		-days 5000 \
		-sha256 \
		-passin pass:some_passphrase \
		-subj '/C=AR/ST=Buenos Aires/L=Tandil/O=Split/OU=IT/CN=RootCA/emailAddress=martin.redolatti@split.io' \
		-out ca.crt

# PROXY
proxy.key proxy.crt:
	openssl genrsa \
		-out proxy.key 2048
	openssl req \
		-key proxy.key \
		-new \
		-sha256 \
		-addext 'subjectAltName=DNS:split-proxy,email:admin@file-server' \
		-subj '/C=AR/ST=Buenos Aires/L=Tandil/O=UNICEN/OU=IT/CN=split-proxy/emailAddress=martin.redolatti@split.io' \
		-out proxy.csr
	openssl ca -config openssl.conf \
		-batch \
		-days 365 \
		-notext \
		-passin pass:some_passphrase \
		-in proxy.csr \
		-out proxy.crt
