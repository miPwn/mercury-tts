[CmdletBinding()]
param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$CommandArgs
)

$ErrorActionPreference = 'Stop'

$RemoteHost = if ($env:HALO_REMOTE_HOST) { $env:HALO_REMOTE_HOST } else { 'falcon' }
$RemoteCommand = if ($env:HALO_REMOTE_COMMAND) { $env:HALO_REMOTE_COMMAND } else { 'halo' }
$RemoteWorkDir = if ($env:HALO_REMOTE_WORKDIR) { $env:HALO_REMOTE_WORKDIR } else { '' }
$SshExe = if ($env:HALO_SSH_EXE) { $env:HALO_SSH_EXE } else { 'ssh.exe' }

function Show-HaloWrapperHelp {
    @"
HALO command client

Usage:
    halo 'your message here'
    halo [command] [options]

General:
    halo /?
    halo -l [--json]
    halo vq
    halo speak

Playback and review:
    halo read <story-name|filename.txt|/full/path/to/file.txt>
    halo review <name|filename.txt|/full/path/to/file.txt>

Generation:
    halo storygen [-mw max_words | -pc minutes] [topic]
    halo storygen -rc
        aliases: halo story-gen, halo sg
        notes: -mw and -pc are mutually exclusive; requests enqueue immediately

Aware mode:
    halo aware on|off|status
    halo aware trigger [commentary|observation|monologue|story] [topic]
    halo aware tick

Sensory:
    halo sensory status
    halo sensory scan [host|network|photoprism|blink|all]
    halo sensory commentary [host|network|photoprism|blink|all]

Render-only variants:
    halo --render-only 'sentence'
    halo --render-only read <story-name|filename.txt|/full/path/to/file.txt>
    halo --render-only review <name|filename.txt|/full/path/to/file.txt>
    halo --render-only storygen [-mw max_words | -pc minutes] [topic]

Windows wrapper details:
  Remote host:    $RemoteHost
  Remote command: $RemoteCommand
  SSH executable: $SshExe
  Remote workdir: $(if ([string]::IsNullOrWhiteSpace($RemoteWorkDir)) { '<default>' } else { $RemoteWorkDir })
  This wrapper forwards non-help commands to the Falcon-side halo runtime over SSH.
"@
}

function ConvertTo-BashSingleQuoted {
    param([string]$Value)

    $joiner = "'" + '"' + "'" + '"' + "'"
    $parts = $Value -split "'", 0, 'SimpleMatch'
    return "'" + ($parts -join $joiner) + "'"
}

if (-not $CommandArgs -or $CommandArgs.Count -eq 0) {
    Show-HaloWrapperHelp
    exit 1
}

switch -Regex ($CommandArgs[0]) {
    '^(\/\?|--help|-h|help)$' {
        Show-HaloWrapperHelp
        exit 0
    }
}

$null = Get-Command $SshExe -ErrorAction Stop

$quotedArgs = @()
foreach ($arg in $CommandArgs) {
    $quotedArgs += ConvertTo-BashSingleQuoted $arg
}

$remoteInvocation = @($RemoteCommand) + $quotedArgs
$remoteCommandText = ($remoteInvocation -join ' ')
if (-not [string]::IsNullOrWhiteSpace($RemoteWorkDir)) {
    $remoteCommandText = 'cd ' + (ConvertTo-BashSingleQuoted $RemoteWorkDir) + ' && ' + $remoteCommandText
}

& $SshExe $RemoteHost 'bash' '-lc' $remoteCommandText
exit $LASTEXITCODE
