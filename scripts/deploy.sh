#!/bin/sh

# Linux deploy script for PocketWatch

set -e
cd "$(dirname $(realpath "$0"))/.."

session="pocketwatch"
shutdown_wait_time=3

rebuild_latest() {
    echo "Building PocketWatch server."
    git pull

    mkdir -p build/
    cd src/
    go build -o ../build/pocketwatch
    cd ..
}

shutdown_server() {
    if ! tmux has-session -t "$session" 2>/dev/null; then
        return 0
    fi

    tmux send-keys -t "$session:main_server" C-c || true
    tmux send-keys -t "$session:main_server" C-c || true
    tmux send-keys -t "$session:main_server" "shutdown" C-m || true

    waited=0
    while [ $waited -lt $shutdown_wait_time ]; do
        # Check if the pocketwatch process is sitll running.
        if ! pgrep -f -- "./build/pocketwatch" >/dev/null 2>&1; then
            echo "PocketWatch server has been shutdown."
            return 0
        fi

        waited=$((waited + 1))
        echo "Waiting ${waited}s / ${shutdown_wait_time}s for the server shutdown."
        sleep 1
    done

    echo "PocketWatch did not shutdown gracefully. Killing it instead..."
    tmux kill-window -t "$session:main_server" || true
}

run_server() {
    echo "Starting PocketWatch server."
    main_server="./build/pocketwatch --config-path secret/config.json"

    # Prefer compiled internal server if one exists.
    if test -e "build/internal_server"; then
        internal_server="./build/internal_server"
    else
        internal_server="python ./scripts/internal_server.py"
    fi

    # Create a new session if one doesn't already exist
    if ! tmux has-session -t "$session" 2>/dev/null; then
        tmux new -s $session -d
        tmux rename-window -t $session internal_server
        tmux send-keys     -t "$session:internal_server" "$internal_server" C-m
    fi

    # Create a new main_server tab if one doesn't already exist
    if ! tmux has-session -t "$session:main_server" 2>/dev/null; then
        tmux new-window -t $session -n main_server
    fi

    tmux send-keys  -t "$session:main_server" "$main_server" C-m
}

rebuild_latest
shutdown_server
run_server
