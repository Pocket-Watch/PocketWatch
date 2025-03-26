#!/bin/sh

set -e

scripts_root=$(dirname "$0")
cd "$scripts_root"
cd ..

pg_ctl -D database/ -l database/logfile start
