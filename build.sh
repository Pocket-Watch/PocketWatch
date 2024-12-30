#!/bin/sh

set -xe

project_root=$(dirname "$0")
cd "$project_root"

# ip="localhost"
ip="192.168.1.2"
port="1234"
browser="firefox"

mkdir -p build/
cd src/ 
go build -o ../build/watchlocally
cd ..
# $browser "$ip:$port/watch/" &
./build/watchlocally -ip $ip -port $port
