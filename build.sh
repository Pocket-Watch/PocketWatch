#!/bin/sh

set -xe

mkdir -p build/
cd src/ 
go build -o ../build/
cd ..
./build/watchlocally -port 1234
