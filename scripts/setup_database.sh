#!/bin/sh

set -e

scripts_root=$(dirname "$0")
cd "$scripts_root"
cd ..

mkdir -p database/

# Create the postgre data.
initdb --username postgres --auth password --pwprompt --encoding utf8 --pgdata database/

# Start postgre database.
pg_ctl --pgdata database/ --log database/logfile.txt start

# Create 'debug' database and user for developement and testing.
sql_create_db="CREATE DATABASE debug_db;"
sql_create_user="CREATE USER debug_user WITH ENCRYPTED PASSWORD 'debugdb123';"
sql_grant_priv="GRANT ALL PRIVILEGES ON DATABASE debug_db TO debug_user;"

psql --username postgres --command "$sql_create_db" --command "$sql_create_user" --command "$sql_grant_priv"

# Grant schema 'public' to the 'debug' user.
sql_grant_schema="GRANT ALL ON SCHEMA public TO debug_user;"
psql --username postgres --command "$sql_grant_schema" "debug_db"

# Stop postgre database.
pg_ctl --pgdata database/ --log database/logfile.txt stop
