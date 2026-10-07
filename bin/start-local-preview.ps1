param(
  [ValidateRange(1, 65535)]
  [int]$Port = 3000,
  [switch]$QA,
  [switch]$Foreground
)

$ErrorActionPreference = 'Stop'
$projectRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$runtimeRoot = Join-Path $projectRoot '.agent-work\local-preview'
$webRoot = Join-Path $projectRoot 'web'
$buildId = [DateTime]::UtcNow.ToString('yyyyMMdd-HHmmss') + '-' + [Guid]::NewGuid().ToString('N').Substring(0, 8)
$nextBinary = Join-Path $runtimeRoot "further-api-building-$buildId.exe"
$binaryName = "further-api-preview-$buildId.exe"
$previewBinary = Join-Path $runtimeRoot $binaryName

$npm = Get-Command npm.cmd -ErrorAction Stop
$goCommand = Get-Command go.exe -ErrorAction SilentlyContinue
if ($goCommand) {
  $goPath = $goCommand.Source
} else {
  $goPath = Join-Path (Split-Path $projectRoot -Parent) '.toolchains\go\bin\go.exe'
  if (-not (Test-Path -LiteralPath $goPath -PathType Leaf)) {
    $previewToolchainRoot = Join-Path $env:USERPROFILE 'go\pkg\mod\golang.org'
    $previewToolchain = Get-ChildItem -LiteralPath $previewToolchainRoot -Directory -Filter 'toolchain@*-go*.windows-amd64' -ErrorAction SilentlyContinue |
      Where-Object { $_.Name -match '-go(\d+\.\d+\.\d+)\.windows-amd64$' -and [version]$Matches[1] -ge [version]'1.27.1' } |
      Sort-Object @{ Expression = { [version]([regex]::Match($_.Name, '-go(\d+\.\d+\.\d+)').Groups[1].Value) }; Descending = $true } |
      Select-Object -First 1
    if ($previewToolchain) { $goPath = Join-Path $previewToolchain.FullName 'bin\go.exe' }
  }
  if (-not (Test-Path -LiteralPath $goPath -PathType Leaf)) { throw 'Go 1.27.1+ is required to build the preview.' }
}
New-Item -ItemType Directory -Force -Path $runtimeRoot | Out-Null

Push-Location $webRoot
try {
  & $npm.Source run build
  if ($LASTEXITCODE -ne 0) { throw 'The web build failed; the running preview has not been stopped.' }
} finally { Pop-Location }

try {
  Push-Location $projectRoot
  try {
    & $goPath build -o $nextBinary .
    if ($LASTEXITCODE -ne 0) { throw 'The Go build failed; the running preview has not been stopped.' }
  } finally { Pop-Location }
  # Publish the completed binary without replacing an executable in use.
  Move-Item -LiteralPath $nextBinary -Destination $previewBinary
} finally {
  if (Test-Path -LiteralPath $nextBinary -PathType Leaf) { Remove-Item -LiteralPath $nextBinary -Force }
}

# Use the same launch, database, session and readiness handling for both paths.
& (Join-Path $PSScriptRoot 'start-built-preview.ps1') -Port $Port -BinaryName $binaryName -QA:$QA -Foreground:$Foreground
