@echo off

cd src
go build -o ../pocketwatch.exe
if %errorlevel% neq 0 exit /b %errorlevel%
cd ..
echo Script is starting server
.\pocketwatch.exe --port 1234 -es

@echo on
