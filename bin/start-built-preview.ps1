param(
  [ValidateRange(1, 65535)]
  [int]$Port = 3000,
  [switch]$QA,
  [ValidatePattern('^[a-zA-Z0-9_.-]+\.exe$')]
  [string]$BinaryName,
  [switch]$Foreground
)

$ErrorActionPreference = 'Stop'
$relayProject = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$relayRuntime = Join-Path $relayProject '.agent-work\local-preview'
$relayRuntimePrefix = $relayRuntime.TrimEnd('\') + '\'

if ($BinaryName) {
  $relayBinary = Join-Path $relayRuntime $BinaryName
  if (-not (Test-Path -LiteralPath $relayBinary -PathType Leaf)) {
    throw "Gateway binary does not exist: $relayBinary. Run bin/start-local-preview.ps1 to build it."
  }
} else {
  # Select the latest completed build instead of a filename tied to a past date.
  $relayBuild = Get-ChildItem -LiteralPath $relayRuntime -Filter 'further-api*.exe' -File -ErrorAction SilentlyContinue |
    Where-Object { $_.Length -gt 0 -and $_.Name -notlike 'further-api-building-*' -and $_.Name -ne 'further-api-next.exe' } |
    Sort-Object LastWriteTimeUtc, Name -Descending | Select-Object -First 1
  if (-not $relayBuild) { throw 'No built gateway is available. Run bin/start-local-preview.ps1 first.' }
  $relayBinary = $relayBuild.FullName
}

$relayData = if ($QA) { Join-Path $relayRuntime 'qa-preview' } else { $relayRuntime }
$relayLogs = Join-Path $relayData 'logs'
$relayOutLog = Join-Path $relayData 'server.out.log'
$relayErrLog = Join-Path $relayData 'server.err.log'
New-Item -ItemType Directory -Force -Path $relayData, $relayLogs | Out-Null
$relaySecretFile = Join-Path $relayData 'session-secret.txt'
if (-not (Test-Path -LiteralPath $relaySecretFile -PathType Leaf)) {
  # These APIs also work with Windows PowerShell 5.1.
  $relaySecretBytes = New-Object byte[] 32
  $relayRandom = [Security.Cryptography.RandomNumberGenerator]::Create()
  try { $relayRandom.GetBytes($relaySecretBytes) } finally { $relayRandom.Dispose() }
  Set-Content -LiteralPath $relaySecretFile -Value ([BitConverter]::ToString($relaySecretBytes).Replace('-', '').ToLowerInvariant()) -NoNewline
}
$relaySecret = (Get-Content -Raw -LiteralPath $relaySecretFile).Trim()
if ([string]::IsNullOrWhiteSpace($relaySecret) -or $relaySecret -eq 'random_string') {
  throw "The existing session secret is invalid: $relaySecretFile. Preserve or repair it before starting the gateway."
}

# Restart only this project's gateway on the requested port.
foreach ($relayOwner in (Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess -Unique)) {
  $relayProcess = Get-CimInstance Win32_Process -Filter "ProcessId=$relayOwner"
  $relayExe = if ($relayProcess.ExecutablePath) { [IO.Path]::GetFullPath($relayProcess.ExecutablePath) } else { '' }
  if ($relayProcess.Name -notlike 'further-api*.exe' -or -not $relayExe.StartsWith($relayRuntimePrefix, [StringComparison]::OrdinalIgnoreCase)) {
    throw "Port $Port belongs to another application (PID $relayOwner)."
  }
  Stop-Process -Id $relayOwner -Force
}
for ($relayAttempt = 0; $relayAttempt -lt 50; $relayAttempt++) {
  if (-not (Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue)) { break }
  Start-Sleep -Milliseconds 100
}
if (Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue) { throw "Port $Port did not become free." }

$relayEnvironment = @{
  BIND_HOST = '127.0.0.1'
  PORT = [string]$Port
  SQL_DSN = ''
  LOG_SQL_DSN = ''
  REDIS_CONN_STRING = ''
  SQLITE_PATH = (Join-Path $relayData 'further-api.db').Replace('\', '/') + '?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate'
  SESSION_SECRET = $relaySecret
  CRYPTO_SECRET = $relaySecret
  TRUSTED_PROXIES = 'none'
  GLOBAL_API_RATE_LIMIT = $(if ($QA) { '10000' } else { $null })
  SEARCH_RATE_LIMIT = $(if ($QA) { '1000' } else { $null })
}
$relayPreviousEnvironment = @{}
try {
  foreach ($relayKey in $relayEnvironment.Keys) {
    $relayPreviousEnvironment[$relayKey] = [Environment]::GetEnvironmentVariable($relayKey, 'Process')
    [Environment]::SetEnvironmentVariable($relayKey, $relayEnvironment[$relayKey], 'Process')
  }
  if ($Foreground) {
    $relayServer = Start-Process -FilePath $relayBinary -ArgumentList @('--log-dir', ('"' + $relayLogs + '"')) -WorkingDirectory $relayProject -WindowStyle Hidden -RedirectStandardOutput $relayOutLog -RedirectStandardError $relayErrLog -PassThru
    $relayLauncherId = $relayServer.Id
  } else {
    # WMI owns the detached launcher, so closing an agent/terminal session does
    # not terminate the gateway. Secrets are passed in its environment only.
    $relayStartup = New-CimInstance -ClassName Win32_ProcessStartup -ClientOnly -Property @{
      ShowWindow = [uint16]0
      EnvironmentVariables = [string[]]@(Get-ChildItem Env: | ForEach-Object { $_.Name + '=' + $_.Value })
    }
    $relayCommand = '"{0}" /d /s /c ""{1}" --log-dir "{2}" > "{3}" 2> "{4}""' -f $env:ComSpec, $relayBinary, $relayLogs, $relayOutLog, $relayErrLog
    $relayCreated = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{
      CommandLine = $relayCommand
      CurrentDirectory = $relayProject
      ProcessStartupInformation = $relayStartup
    }
    if ($relayCreated.ReturnValue -ne 0) {
      throw "Could not start the detached gateway (Windows error $($relayCreated.ReturnValue)). Use -Foreground to run it in this terminal."
    }
    $relayLauncherId = [int]$relayCreated.ProcessId
  }
} finally {
  foreach ($relayKey in $relayPreviousEnvironment.Keys) {
    [Environment]::SetEnvironmentVariable($relayKey, $relayPreviousEnvironment[$relayKey], 'Process')
  }
}

$relayReady = $false
for ($relayAttempt = 0; $relayAttempt -lt 100; $relayAttempt++) {
  if (-not (Get-Process -Id $relayLauncherId -ErrorAction SilentlyContinue)) { break }
  try {
    $relayResponse = Invoke-WebRequest -UseBasicParsing -Uri "http://127.0.0.1:$Port/api/status" -TimeoutSec 2
    if ([int]$relayResponse.StatusCode -eq 200) { $relayReady = $true; break }
  } catch { }
  Start-Sleep -Milliseconds 300
}
if (-not $relayReady) {
  throw "The gateway did not become ready on port $Port. Check $relayOutLog and $relayErrLog."
}
$relayServerId = Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction Stop | Select-Object -ExpandProperty OwningProcess -First 1
Write-Output "further-api preview ready: http://127.0.0.1:$Port/ (PID $relayServerId, binary $([IO.Path]::GetFileName($relayBinary)))"
if ($Foreground) {
  $relayServer.WaitForExit()
  if ($relayServer.ExitCode -ne 0) { throw "The gateway exited with code $($relayServer.ExitCode). Check $relayErrLog." }
}
