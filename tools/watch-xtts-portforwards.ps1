param(
    [int]$PollSeconds = 5,
    [int]$HealthTimeoutSeconds = 4,
    [int]$UnhealthyThreshold = 2
)

$ErrorActionPreference = 'Stop'

$mutex = [System.Threading.Mutex]::new($false, 'Global\HALO-XTTS-PortForward-Watchdog')
if (-not $mutex.WaitOne(0, $false)) {
    exit 0
}

try {
    $repoRoot = Split-Path -Parent $PSScriptRoot
    $stateDir = Join-Path $env:LOCALAPPDATA 'HALO\xtts-portforward'
    $logDir = Join-Path $stateDir 'logs'
    $pidDir = Join-Path $stateDir 'pids'
    New-Item -ItemType Directory -Force -Path $logDir | Out-Null
    New-Item -ItemType Directory -Force -Path $pidDir | Out-Null

    $kubectl = (Get-Command kubectl -ErrorAction Stop).Source

    $forwards = @(
        @{
            Name       = 'instant'
            Service    = 'coqui-xtts-instant-local'
            ListenPort = 5003
            TargetPort = 5003
            ProbeUrl   = 'http://127.0.0.1:5003/'
        },
        @{
            Name       = 'story'
            Service    = 'coqui-xtts-story-local'
            ListenPort = 5004
            TargetPort = 5003
            ProbeUrl   = 'http://127.0.0.1:5004/'
        }
    )

    $unhealthyCounts = @{}

    function Write-Log {
        param(
            [string]$Message
        )

        $timestamp = Get-Date -Format 'yyyy-MM-dd HH:mm:ss'
        Add-Content -Path (Join-Path $logDir 'watchdog.log') -Value "[$timestamp] $Message"
    }

    function Get-PidFile {
        param(
            [hashtable]$Forward
        )

        Join-Path $pidDir ("{0}.pid" -f $Forward.Name)
    }

    function Test-ServicePresent {
        param(
            [hashtable]$Forward
        )

        $args = @('get', 'svc', '-n', 'tts', $Forward.Service, '-o', 'name')
        $result = & $kubectl @args 2>$null
        return $LASTEXITCODE -eq 0 -and -not [string]::IsNullOrWhiteSpace($result)
    }

    function Get-RunningProcess {
        param(
            [hashtable]$Forward
        )

        $pidFile = Get-PidFile $Forward
        if (-not (Test-Path $pidFile)) {
            return $null
        }

        $rawPid = (Get-Content -Path $pidFile -ErrorAction SilentlyContinue | Select-Object -First 1)
        if (-not $rawPid) {
            Remove-Item -Force -ErrorAction SilentlyContinue $pidFile
            return $null
        }

        $process = Get-Process -Id ([int]$rawPid) -ErrorAction SilentlyContinue
        if ($null -eq $process) {
            Remove-Item -Force -ErrorAction SilentlyContinue $pidFile
            return $null
        }

        return $process
    }

    function Get-ListeningOwner {
        param(
            [int]$Port
        )

        Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue |
        Select-Object -First 1 -ExpandProperty OwningProcess
    }

    function Get-ListeningOwners {
        param(
            [int]$Port
        )

        @(Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue |
            Select-Object -ExpandProperty OwningProcess -Unique)
    }

    function Test-ForwardHealthy {
        param(
            [hashtable]$Forward
        )

        $curlExe = (Get-Command curl.exe -ErrorAction SilentlyContinue).Source
        if (-not $curlExe) {
            return $false
        }

        $response = & $curlExe -sS --connect-timeout $HealthTimeoutSeconds --max-time $HealthTimeoutSeconds -o NUL -w '%{http_code}' $Forward.ProbeUrl 2>$null
        if ($LASTEXITCODE -ne 0) {
            return $false
        }

        return -not [string]::IsNullOrWhiteSpace($response) -and $response -ne '000'
    }

    function Stop-Forward {
        param(
            [hashtable]$Forward,
            [System.Diagnostics.Process]$Process = $null,
            [string]$Reason = 'restarting'
        )

        if ($null -eq $Process) {
            $Process = Get-RunningProcess $Forward
        }

        if ($null -ne $Process -and -not $Process.HasExited) {
            try {
                Stop-Process -Id $Process.Id -Force -ErrorAction Stop
                Write-Log ("Stopped {0} port-forward pid {1} ({2})" -f $Forward.Name, $Process.Id, $Reason)
            }
            catch {
                Write-Log ("Failed stopping {0} pid {1}: {2}" -f $Forward.Name, $Process.Id, $_.Exception.Message)
            }
        }

        Remove-Item -Force -ErrorAction SilentlyContinue (Get-PidFile $Forward)
    }

    function Start-Forward {
        param(
            [hashtable]$Forward
        )

        $stdoutLog = Join-Path $logDir ("{0}.stdout.log" -f $Forward.Name)
        $stderrLog = Join-Path $logDir ("{0}.stderr.log" -f $Forward.Name)
        $argumentList = @(
            'port-forward',
            '-n', 'tts',
            '--address', '0.0.0.0',
            ("svc/{0}" -f $Forward.Service),
            ("{0}:{1}" -f $Forward.ListenPort, $Forward.TargetPort)
        )

        $process = Start-Process -FilePath $kubectl -ArgumentList $argumentList -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdoutLog -RedirectStandardError $stderrLog
        Set-Content -Path (Get-PidFile $Forward) -Value $process.Id
        $unhealthyCounts[$Forward.Name] = 0
        Write-Log ("Started {0} port-forward on {1} -> svc/{2}:{3} (pid {4})" -f $Forward.Name, $Forward.ListenPort, $Forward.Service, $Forward.TargetPort, $process.Id)
    }

    Write-Log ("Watchdog online from {0}" -f $repoRoot)

    while ($true) {
        foreach ($forward in $forwards) {
            if (-not (Test-ServicePresent $forward)) {
                Write-Log ("Service {0} not ready; deferring {1} forward" -f $forward.Service, $forward.Name)
                continue
            }

            $process = Get-RunningProcess $forward
            if ($null -ne $process -and -not $process.HasExited) {
                if (Test-ForwardHealthy $forward) {
                    $unhealthyCounts[$forward.Name] = 0
                    continue
                }

                $unhealthyCounts[$forward.Name] = 1 + [int]($unhealthyCounts[$forward.Name])
                Write-Log ("Health check failed for {0} on port {1} (attempt {2}/{3})" -f $forward.Name, $forward.ListenPort, $unhealthyCounts[$forward.Name], $UnhealthyThreshold)
                if ($unhealthyCounts[$forward.Name] -lt $UnhealthyThreshold) {
                    continue
                }

                Stop-Forward -Forward $forward -Process $process -Reason 'unhealthy health probe'
            }

            $owners = Get-ListeningOwners -Port $forward.ListenPort
            if ($owners.Count -gt 0) {
                $blockingOwners = @()
                foreach ($owner in $owners) {
                    $ownerProcess = Get-Process -Id $owner -ErrorAction SilentlyContinue
                    if ($null -eq $ownerProcess) {
                        continue
                    }

                    if ($ownerProcess.Name -in @('com.docker.backend', 'wslrelay')) {
                        continue
                    }

                    if ($ownerProcess.Name -eq 'kubectl') {
                        continue
                    }

                    $blockingOwners += $owner
                }

                if ($blockingOwners.Count -gt 0) {
                    Write-Log ("Port {0} blocked by pid(s) {1}; watchdog will not replace them" -f $forward.ListenPort, ($blockingOwners -join ', '))
                    continue
                }
            }

            Start-Forward $forward
            Start-Sleep -Seconds 2
        }

        Start-Sleep -Seconds $PollSeconds
    }
}
finally {
    $mutex.ReleaseMutex() | Out-Null
    $mutex.Dispose()
}