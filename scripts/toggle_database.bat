@echo off

REM Set the scripts root directory
set "scripts_root=%~dp0"
cd "%scripts_root%.."

REM Check if PostgreSQL is ready
pg_isready > NUL 2>&1
if %errorlevel% equ 0 (
    echo Stopping the database.
    pg_ctl -D database/ -l database/logfile.txt stop > NUL 2>&1
) else (
    echo Starting the database.
    REM This process must be detached or else CTRL+C from within another program terminates postgres
    start /b pg_ctl -D database/ -l database/logfile.txt start > NUL 2>&1
)
