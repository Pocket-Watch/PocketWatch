@echo off

REM Set the scripts root directory
set "scripts_root=%~dp0"
cd "%scripts_root%.."

REM Check if PostgreSQL is ready
pg_isready >nul 2>&1
if "%errorlevel%" equ 0 (
    echo Stopping the database.
    pg_ctl -D database/ -l database/logfile.txt stop
) else (
    echo Starting the database.
    pg_ctl -D database/ -l database/logfile.txt start
)

endlocal
