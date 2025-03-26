#!/bin/sh

set -e

scripts_root=$(dirname "$0")
cd "$scripts_root"
cd ..

alacritty --working-directory $PWD --command nvim &

tmux new -s pocket-dev -d
tmux rename-window  -t pocket-dev pocketwatch
tmux send-keys      -t pocket-dev "./build.sh$(printf \\r)"
tmux new-window     -t pocket-dev -n ollama
tmux send-keys      -t pocket-dev "ollama serve$(printf \\r)"
tmux split-window   -t pocket-dev -h
tmux send-keys      -t pocket-dev "open-webui serve$(printf \\r)"
tmux new-window     -t pocket-dev -n monitors
tmux send-keys      -t pocket-dev "htop$(printf \\r)"
tmux split-window   -t pocket-dev -h
tmux send-keys      -t pocket-dev "nvtop$(printf \\r)"
tmux split-window   -t pocket-dev -v
tmux new-window     -t pocket-dev -n scratch
tmux select-window  -t pocket-dev:0

firefox "http://localhost:1234/watch/" &
firefox "http://localhost:8080/" &
firefox "https://github.com/Pocket-Watch/watch-locally" &

tmux attach-session -t pocket-dev
