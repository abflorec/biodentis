@echo off
setlocal
set "app=%~dp0bin\BIODENTIS-Escritorio.exe"
if not exist "%app%" (
  echo Falta bin\BIODENTIS-Escritorio.exe. Ejecuta scripts\compilar-escritorio.ps1.
  pause
  exit /b 1
)
start "" "%app%"
