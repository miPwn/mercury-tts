param(
    [int]$PollSeconds = 5
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
            Name = 'instant'
            Service = 'coqui-xtts-instant-local'
            ListenPort = 5003
            TargetPort = 5003
        },
        @{
            Name = 'story'
            Service = 'coqui-xtts-story-local'
            ListenPort = 5004
            TargetPort = 5003
        }
    )

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
                continue
            }

            $owner = Get-ListeningOwner -Port $forward.ListenPort
            if ($owner) {
                Write-Log ("Port {0} already owned by pid {1}; watchdog will not replace it" -f $forward.ListenPort, $owner)
                continue
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