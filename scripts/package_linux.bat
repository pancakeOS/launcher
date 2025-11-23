@echo off
REM Wrapper to run the Linux packaging script via WSL from Windows.
REM Usage: package_linux.bat [VERSION]

setlocal enabledelayedexpansion
if "%1"=="" (
  set VERSION=0.0.0
) else (
  set VERSION=%1
)












exit /b %ERRORLEVEL%
nendlocalwsl bash -lc "cd '%WSLPATH%' && VERSION=%VERSION% ./scripts/package_linux.sh"
necho Running packaging script inside WSL at %WSLPATH% with VERSION=%VERSION%for /f "usebackq delims=" %%p in (`wsl wslpath "%CD%"`) do set WSLPATH=%%p
nREM Convert current directory to WSL path)  exit /b 1  echo You can also open WSL and run: VERSION=%VERSION% ./scripts/package_linux.sh  echo WSL not found. Install WSL or run the packaging script inside a Linux environment.nwhere wsl >nul 2>&1
nif %ERRORLEVEL% NEQ 0 (