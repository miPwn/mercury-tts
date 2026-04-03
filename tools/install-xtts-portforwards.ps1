param()

$ErrorActionPreference = 'Stop'

$taskName = 'HALO XTTS PortForward Watchdog'
$runValueName = 'HALO XTTS PortForward Watchdog'
$runKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
$watchdogScript = Join-Path $PSScriptRoot 'watch-xtts-portforwards.ps1'
$powershellExe = (Get-Command powershell.exe -ErrorAction Stop).Source
$currentUser = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
$launchCommand = "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File `"$watchdogScript`""

function Ensure-FirewallRule {
    param(
        [string]$DisplayName,
        [int]$Port
    )

    $existing = Get-NetFirewallRule -DisplayName $DisplayName -ErrorAction SilentlyContinue
    if ($null -eq $existing) {
        try {
            New-NetFirewallRule -DisplayName $DisplayName -Direction Inbound -Action Allow -Protocol TCP -LocalPort $Port -ErrorAction Stop | Out-Null
        }
        catch {
            Write-Warning ("Firewall rule '{0}' was not created: {1}" -f $DisplayName, $_.Exception.Message)
        }
    }
}

function Register-RunEntry {
    New-Item -Path $runKey -Force | Out-Null
    Set-ItemProperty -Path $runKey -Name $runValueName -Value ("`"{0}`" {1}" -f $powershellExe, $launchCommand)
}

function Remove-RunEntry {
    Remove-ItemProperty -Path $runKey -Name $runValueName -ErrorAction SilentlyContinue
}

Ensure-FirewallRule -DisplayName 'HALO XTTS 5003' -Port 5003
Ensure-FirewallRule -DisplayName 'HALO XTTS 5004' -Port 5004

$action = New-ScheduledTaskAction -Execute $powershellExe -Argument $launchCommand
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $currentUser
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -MultipleInstances IgnoreNew
$principal = New-ScheduledTaskPrincipal -UserId $currentUser -LogonType Interactive -RunLevel Highest

$startupMode = 'scheduled-task'
try {
    Register-ScheduledTask -TaskName $taskName -Action $action -Trigger $trigger -Settings $settings -Principal $principal -Force -ErrorAction Stop | Out-Null
    Remove-RunEntry
}
catch {
    $startupMode = 'hkcu-run'
    Register-RunEntry
    Write-Warning ("Scheduled task registration failed, using HKCU Run fallback: {0}" -f $_.Exception.Message)
}

Start-Process -FilePath $powershellExe -ArgumentList @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-WindowStyle', 'Hidden', '-File', $watchdogScript) -WindowStyle Hidden

Write-Output ("Installed startup mode: {0}" -f $startupMode)
Write-Output ("Watchdog script: {0}" -f $watchdogScript)