@echo off

set "project_root=%~dp0"
cd "%project_root%"

cd src
go build -o ../pocketwatch.exe
if %errorlevel% neq 0 exit /b %errorlevel%
cd ..
echo Script is starting server
.\pocketwatch.exe

@echo on
