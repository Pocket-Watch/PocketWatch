#!/bin/sh

set -xe

project_root=$(dirname "$0")
cd "$project_root"

mkdir -p build/
cd src/ 
go build -race -o ../build/pocketwatch
cd ..

# Generate dummy config when one is missing.
if test ! -f "secret/config.json"; then
    mkdir -p "secret/"
    ./build/pocketwatch --generate-config --config-path "secret/config.json" "$@"
else 
    ./build/pocketwatch --config-path "secret/config.json" "$@"
fi


