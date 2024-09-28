#!/bin/sh

set -xe

mkdir -p build/
cd src/ 
go build -o ../build/watchlocally
cd ..
./build/watchlocally -port 1234
