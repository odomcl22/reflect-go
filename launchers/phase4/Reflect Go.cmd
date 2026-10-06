@echo off
setlocal DisableDelayedExpansion
set "REFLECT_ARCH=%PROCESSOR_ARCHITECTURE%"
if defined PROCESSOR_ARCHITEW6432 set "REFLECT_ARCH=%PROCESSOR_ARCHITEW6432%"
if /I "%REFLECT_ARCH%"=="AMD64" goto x64
if /I "%REFLECT_ARCH%"=="ARM64" goto arm64
echo Reflect Go requires 64-bit Windows on x86-64 or ARM64.
pause
exit /b 1
:x64
set "REFLECT_BIN=%~dp0bin\windows-x64\reflectgo.exe"
goto run
:arm64
set "REFLECT_BIN=%~dp0bin\windows-arm64\reflectgo.exe"
:run
"%REFLECT_BIN%" %*
if errorlevel 1 pause
