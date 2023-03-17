#!/bin/bash

function build_binary {
    name="$1"

    for arch in amd64 arm64; do
        GOOS=darwin GOARCH=$arch go build -o "$name"osx-$arch
    done

    for arch in arm64 amd64 386; do
        GOOS=linux GOARCH=$arch go build -o "$name"linux-$arch
    done

    for arch in amd64 386; do
        GOOS=windows GOARCH=$arch go build -o "$name"windows-$arch.exe
    done
}

version="$1"
mkdir -p "binaries/""$version"

build_binary "binaries/""$version""/linx-server-v""$version""_"

cd linx-genkey
build_binary "../binaries/""$version""/linx-genkey-v""$version""_"
cd ..
