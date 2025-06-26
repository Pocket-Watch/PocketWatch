#!/bin/sh

# Linux deploy script for pocketwatch

set -xe

project_root=$(dirname "$0")
session="pocketwatch"

cd "$project_root"

git pull

mkdir -p build/
cd src/ 
go build -o ../build/pocketwatch
cd ..


tmux send-keys -t $session "C-c" || true
tmux send-keys -t $session "shutdown $(printf \\r)" || true
sleep 1
tmux kill-session -t $session || true

# TODO(kihau): 
#     Wait for the server to exit
#     Always send shutdown to the main_server tab
#     Kill it after 10 seconds of trying

main_server="./build/pocketwatch --config-path secret/config.json"
internal_server="python ./scripts/internal_server.py"

tmux new -s $session -d
tmux rename-window  -t $session internal_server
tmux send-keys      -t $session "$internal_server $(printf \\r)"
sleep 1
tmux new-window     -t $session -n main_server
tmux send-keys      -t $session "$main_server $(printf \\r)"

