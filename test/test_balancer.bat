@echo off
setlocal enabledelayedexpansion

:: Define test cases
set strategies=round_robin least_connections weighted_round_robin random

for %%S in (%strategies%) do (
    echo Testing %%S...

    :: Kill any existing process on port 8080
    for /f "tokens=5" %%a in ('netstat -aon ^| findstr /R "\<8080\>"') do (
        taskkill /F /PID %%a >nul 2>&1
        timeout /T 1 >nul
    )

    :: Set Load Balancer Strategy and Run Server
    set "LB_STRATEGY=%%S" && start "" /B go run ../backend/cmd/api/main.go
    timeout /T 5 >nul

    :: Send a test request
    curl -s -X POST http://localhost:8080/request -H "Content-Type: application/json" -d "{\"client_id\": \"test\"}" -w "\n"
    echo.

    :: Kill the server process before switching to the next test
    for /f "tokens=5" %%a in ('netstat -aon ^| findstr /R "\<8080\>"') do (
        taskkill /F /PID %%a >nul 2>&1
        timeout /T 1 >nul
    )
    timeout /T 2 >nul
)

echo All tests completed.
pause
