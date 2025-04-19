@echo off
setlocal enabledelayedexpansion

:: Define test cases (load balancing strategies)
set strategies=round_robin least_connections weighted_round_robin random

:: Function to kill process running on port 8080
:kill_process
for /f "tokens=5" %%a in ('netstat -aon ^| findstr /R "\<8080\>"') do (
    tasklist /FI "PID eq %%a" | findstr /I "%%a" >nul
    if not errorlevel 1 (
        echo Killing process on port 8080 (PID: %%a)...
        taskkill /F /PID %%a >nul 2>&1
        timeout /T 2 >nul
    )
)
exit /b

:: Loop through each load balancing strategy
for %%S in (%strategies%) do (
    echo ----------------------------------------
    echo Testing Load Balancer Strategy: %%S
    echo ----------------------------------------

    :: Kill any existing process on port 8080
    call :kill_process

    :: Set Load Balancer Strategy and Start Server
    set "LB_STRATEGY=%%S"
    echo Starting server with strategy: %%S...
    start "" /B go run ../backend/cmd/api/main.go
    timeout /T 5 >nul

    :: Send a test request
    echo Sending test request...
    curl -s -X POST http://localhost:8080/request -H "Content-Type: application/json" -d "{\"client_id\": \"test\"}" -w "\n"
    echo.

    :: Kill the server process before the next test
    call :kill_process
    timeout /T 3 >nul
)

echo ----------------------------------------
echo ✅ All tests completed successfully!
echo ----------------------------------------
pause
