#!/bin/sh

# Linux script to run PocketWatch on system boot

set -xe

project_root=$(dirname "$0")
session="pocketwatch"

cd "$project_root/.."

main_server="./build/pocketwatch --config-path secret/config.json"

# Prefer compiled internal ytdlp server if one exists.
if test -e "build/ytdlp_server"; then
    ytdlp_server="./build/ytdlp_server"
else 
    ytdlp_server="python ./scripts/ytdlp_server.py"
fi

tmux new -s $session -d
tmux rename-window -t $session ytdlp_server
tmux send-keys     -t $session "$ytdlp_server $(printf \\r)"
sleep 1
tmux new-window    -t $session -n main_server
tmux send-keys     -t $session "$main_server $(printf \\r)"

