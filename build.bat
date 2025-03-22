@echo off

cd src
go build -o ../watchlocally.exe
if %errorlevel% neq 0 exit /b %errorlevel%
cd ..
echo Script is starting server
.\watchlocally.exe --port 1234 -es

@echo on
