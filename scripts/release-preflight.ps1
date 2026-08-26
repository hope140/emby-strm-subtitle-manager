[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidateNotNullOrEmpty()]
    [string] $ImageRef,

    [Parameter()]
    [string] $ComposePath = (Join-Path (Get-Location) 'deploy/compose.release.example.yaml'),

    [Parameter()]
    [string] $ConfigPath = (Join-Path (Get-Location) 'deploy/config.example.yaml'),

    # Validates the checked-in examples without requiring their deployment
    # placeholders to be replaced. Omit this switch for a private release copy.
    [switch] $Template
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Require-Match {
    param(
        [Parameter(Mandatory)] [string] $Text,
        [Parameter(Mandatory)] [string] $Pattern,
        [Parameter(Mandatory)] [string] $Message
    )

    if ($Text -notmatch $Pattern) {
        throw $Message
    }
}

function Reject-Match {
    param(
        [Parameter(Mandatory)] [string] $Text,
        [Parameter(Mandatory)] [string] $Pattern,
        [Parameter(Mandatory)] [string] $Message
    )

    if ($Text -match $Pattern) {
        throw $Message
    }
}

if ($ImageRef -notmatch '^[^\s@]+@sha256:[a-f0-9]{64}$') {
    throw 'ImageRef must be a full immutable registry/repository@sha256 digest reference.'
}

$resolvedCompose = (Resolve-Path -LiteralPath $ComposePath -ErrorAction Stop).Path
$resolvedConfig = (Resolve-Path -LiteralPath $ConfigPath -ErrorAction Stop).Path
$compose = Get-Content -LiteralPath $resolvedCompose -Raw -Encoding UTF8
$config = Get-Content -LiteralPath $resolvedConfig -Raw -Encoding UTF8

Reject-Match $compose '(?m)^\s*build\s*:' 'Release Compose must not contain a build section.'
Require-Match $compose '(?m)^\s*image:\s*"?\$\{IMAGE_REF:\?' 'Release Compose must resolve its image from IMAGE_REF.'
Require-Match $compose '(?m)^\s*pull_policy:\s*always\s*$' 'Release Compose must pull the immutable image reference.'
Require-Match $compose '(?m)^\s*user:\s*"?10001:10001"?\s*$' 'Release Compose must run as UID/GID 10001.'
Require-Match $compose '(?m)^\s*read_only:\s*true\s*$' 'Release Compose must keep the root filesystem read-only.'
Require-Match $compose '(?m)^\s*init:\s*true\s*$' 'Release Compose must enable init.'
Require-Match $compose '(?m)^\s*-\s*"?127\.0\.0\.1:18080:8080"?\s*$' 'Release Compose must bind the application port to host loopback only.'
Require-Match $compose '(?m)^\s*-\s*no-new-privileges:true\s*$' 'Release Compose must keep no-new-privileges enabled.'
Require-Match $compose '(?m)^\s*-\s*ALL\s*$' 'Release Compose must drop all Linux capabilities.'
Require-Match $compose '(?s)target:\s*/media\s*\r?\n\s*read_only:\s*true' 'Release Compose must mount /media read-only.'
Require-Match $compose '(?m)^\s*APP_ADMIN_USERNAME:\s*' 'Release Compose must declare APP_ADMIN_USERNAME directly.'
Require-Match $compose '(?m)^\s*APP_ADMIN_PASSWORD:\s*' 'Release Compose must declare APP_ADMIN_PASSWORD directly.'
foreach ($secret in @('emby_api_key', 'app_identity_key', 'app_api_auth_token')) {
    Require-Match $compose ("(?m)^\s*{0}:\s*$" -f [regex]::Escape($secret)) ("Release Compose is missing the {0} secret declaration." -f $secret)
}

Require-Match $config '(?m)^\s*write_enabled:\s*false\s*$' 'Release config must keep write_enabled=false.'
Require-Match $config '(?m)^\s*remote_search_enabled:\s*false\s*$' 'Release config must keep remote_search_enabled=false.'
if ([regex]::Matches($config, '(?m)^\s*enabled:\s*false\s*$').Count -lt 2) {
    throw 'Release config must keep both D2 and D3 Canary modes disabled.'
}

if (-not $Template) {
    Reject-Match $compose '/replace/with/' 'Private release Compose still has a media-path placeholder.'
    Reject-Match $config '/replace/with/|example\.invalid' 'Private release config still has a deployment placeholder.'
    Require-Match $compose '(?m)^\s*APP_ADMIN_USERNAME:\s*"?[^"\s][^\r\n]*' 'Private release Compose has no administrator username.'
    Require-Match $compose '(?m)^\s*APP_ADMIN_PASSWORD:\s*"?[^"\s][^\r\n]*' 'Private release Compose has no administrator password.'
}

Write-Host 'Release preflight passed.'
