@echo off

REM Set the scripts root directory
set "scripts_root=%~dp0"
cd "%scripts_root%.."

REM Create the database directory
mkdir database

REM Create the PostgreSQL data
initdb --username postgres --auth password --pwprompt --encoding utf8 --pgdata database

REM Start PostgreSQL database
pg_ctl --pgdata database --log database\logfile.txt start

REM Create 'debug' database and user for development and testing
set "sql_create_db=CREATE DATABASE debug_db;"
set "sql_create_user=CREATE USER debug_user WITH ENCRYPTED PASSWORD 'debugdb123';"
set "sql_grant_priv=GRANT ALL PRIVILEGES ON DATABASE debug_db TO debug_user;"

psql --username postgres --command "%sql_create_db%" --command "%sql_create_user%" --command "%sql_grant_priv%"

REM Grant schema 'public' to the 'debug' user
set "sql_grant_schema=GRANT ALL ON SCHEMA public TO debug_user;"
psql --username postgres --command "%sql_grant_schema%" debug_db

REM Stop PostgreSQL database
pg_ctl --pgdata database --log database\logfile.txt stop

endlocal
