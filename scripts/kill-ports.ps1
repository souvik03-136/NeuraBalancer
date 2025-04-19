$ports = @(8080, 8081, 5000, 5001, 5002)
$killed = $false

foreach ($port in $ports) {
    $connections = Get-NetTCPConnection -LocalPort $port -ErrorAction SilentlyContinue
    if ($connections) {
        Stop-Process -Id $connections.OwningProcess -Force
        $killed = $true
        Write-Output "Killed process using port $port"
    }
}

if (-not $killed) {
    Write-Output "No processes found using ports 8080, 8081, 5000, 5001, 5002"
}