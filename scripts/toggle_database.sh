#!/bin/sh

scripts_root=$(dirname "$0")
cd "$scripts_root"
cd ..

pg_isready 2>&1 >/dev/null
if test $? -eq 0; then 
    echo "Stopping the database."
    pg_ctl -D database/ -l database/logfile.txt stop
else 
    echo "Starting the database."
    pg_ctl -D database/ -l database/logfile.txt start
fi
