@echo off

cd src
go build -o ../watchlocally.exe
cd ..
echo Script is starting server
.\watchlocally.exe -port 1234

@echo on