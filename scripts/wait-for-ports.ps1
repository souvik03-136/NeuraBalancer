param(
    [Parameter(Mandatory=$true)]
    [int[]]$Ports,
    
    [int]$MaxAttempts = 10,  # Increased from 5
    [int]$RetryInterval = 2  # Increased from 1
)

$ErrorActionPreference = 'Stop'

foreach ($port in $Ports) {
    $count = 0
    $success = $false
    
    Write-Host "⌛ Checking port $port..."
    
    while ($count -lt $MaxAttempts) {
        try {
            $result = Test-NetConnection localhost -Port $port -WarningAction SilentlyContinue
            if ($result.TcpTestSucceeded) {
                $success = $true
                Write-Host "Port $port ready"
                break
            }
        } catch {
            # Ignore connection errors
        }
        
        $count++
        Write-Host "⏳ Attempt $count/$MaxAttempts - Port $port not responding"
        Start-Sleep $RetryInterval
    }

    if (-not $success) {
        Write-Host "CRITICAL: Port $port failed after $MaxAttempts attempts"
        Write-Host "Check these possible issues:"
        Write-Host "   1. Service not starting (check logs/balancer-run.log)"
        Write-Host "   2. Port conflict (run 'task kill-ports')"
        Write-Host "   3. Firewall blocking port $port"
        exit 1
    }
}

exit 0