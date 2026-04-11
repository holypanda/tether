@echo off
REM 双击启动: 窗口保持打开, 看得到所有输出和错误
cd /d "%~dp0"
powershell.exe -NoProfile -ExecutionPolicy Bypass -NoExit -File "%~dp0tunnel.ps1"
