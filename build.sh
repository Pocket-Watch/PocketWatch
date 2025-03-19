#!/bin/sh

set -xe

project_root=$(dirname "$0")
cd "$project_root"

ip="0.0.0.0"
port="1234"
browser="firefox"

mkdir -p build/
cd src/ 
go build -race -o ../build/pocketwatch
cd ..

# $browser "$ip:$port/watch/" &
./build/pocketwatch --config-path "secret/config.json"
