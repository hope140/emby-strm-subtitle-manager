[CmdletBinding()]
param(
    [Parameter()]
    [string] $GoPath
)

$ErrorActionPreference = 'Stop'
$env:GOCACHE = Join-Path (Get-Location) '.gocache'
$env:GOMODCACHE = Join-Path $env:GOCACHE 'mod'

if ([string]::IsNullOrWhiteSpace($GoPath)) {
    $goCommand = Get-Command go -ErrorAction Stop
    $GoPath = $goCommand.Source
} else {
    $GoPath = (Resolve-Path -LiteralPath $GoPath -ErrorAction Stop).Path
}

$goDirectory = Split-Path -Parent $GoPath
$gofmtPath = Join-Path $goDirectory 'gofmt.exe'
if (-not (Test-Path -LiteralPath $gofmtPath -PathType Leaf)) {
    throw "gofmt was not found beside the selected Go executable: $gofmtPath"
}

function Invoke-Go {
    param(
        [Parameter(Mandatory)]
        [string[]] $Arguments
    )

    & $GoPath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "go $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}

$goFiles = Get-ChildItem -Path . -Recurse -File -Filter '*.go' |
    Where-Object { $_.FullName -notmatch '[\\/](\.tools|\.git|\.gocache|\.gomodcache)[\\/]' }
$formatOutput = foreach ($goFile in $goFiles) {
    & $gofmtPath -l $goFile.FullName
}
if (-not [string]::IsNullOrWhiteSpace(($formatOutput -join "`n"))) {
    Write-Error "gofmt reported unformatted files:`n$($formatOutput -join "`n")"
}

Invoke-Go @('vet', './...')
Invoke-Go @('test', '-count=1', './...')
Invoke-Go @('build', '-trimpath', './cmd/server')

Write-Host 'Verification passed.'
