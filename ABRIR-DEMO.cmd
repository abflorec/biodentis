@echo off
setlocal
powershell.exe -NoProfile -ExecutionPolicy RemoteSigned -File "%~dp0scripts\abrir-demo.ps1"
if errorlevel 1 pause
