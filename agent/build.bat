@echo off
rem Build the flow-statistics agent: vet + test + cross-compile (OpenWrt arm64) + sha256.
rem Run from any directory; outputs land next to this script.

setlocal
cd /d "%~dp0"

where go >nul 2>nul
if errorlevel 1 (
  echo ERROR: go not found in PATH
  exit /b 1
)

go vet ./...
if errorlevel 1 goto :fail

go test ./...
if errorlevel 1 goto :fail

set GOOS=linux
set GOARCH=arm64
set CGO_ENABLED=0
go build -ldflags="-s -w" -o traffic-agent-linux-arm64 .
if errorlevel 1 goto :fail

rem sha256 via certutil (built into Windows); installer lowercases on compare.
set "HASH="
for /f "tokens=1" %%h in ('certutil -hashfile traffic-agent-linux-arm64 SHA256 ^| findstr /i /r "^[0-9a-f][0-9a-f]*$"') do set "HASH=%%h"
if not defined HASH (
  echo ERROR: failed to compute sha256
  exit /b 1
)
>"traffic-agent-linux-arm64.sha256" echo %HASH%

echo OK: traffic-agent-linux-arm64 + traffic-agent-linux-arm64.sha256
exit /b 0

:fail
echo BUILD FAILED
exit /b 1
