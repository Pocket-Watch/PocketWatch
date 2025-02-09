@echo off

cd src
go build -race -o ../watchlocally.exe
if %errorlevel% neq 0 exit /b %errorlevel%
cd ..
echo Script is starting server
.\watchlocally.exe -port 1234

@echo on
