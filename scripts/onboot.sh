#!/bin/sh

# Linux script to run PocketWatch on system boot

set -xe

project_root=$(dirname "$0")
session="pocketwatch"

cd "$project_root/.."

main_server="./build/pocketwatch --config-path secret/config.json"

# Prefer compiled internal server if one exists.
if test -e "build/internal_server"; then
    internal_server="./build/internal_server"
else 
    internal_server="python ./scripts/internal_server.py"
fi

tmux new -s $session -d
tmux rename-window  -t $session internal_server
tmux send-keys      -t $session "$internal_server $(printf \\r)"
sleep 1
tmux new-window     -t $session -n main_server
tmux send-keys      -t $session "$main_server $(printf \\r)"

