@echo off

echo Clearing chunks

REM Set the scripts root directory
set "scripts_root=%~dp0"
cd "%scripts_root%.."

cd web/proxy
del vi-*
del au-*
del live-*
REM cd ../../../

@echo on
