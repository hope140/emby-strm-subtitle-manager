[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$go = (Get-Command go -ErrorAction Stop).Source
$npx = (Get-Command npx.cmd -ErrorAction Stop).Source
$browserScript = Join-Path $repoRoot 'scripts/core-ab-ui-e2e-browser.js'
$session = 'core-ab-ui-e2e-' + [guid]::NewGuid().ToString('N')
$testToken = [guid]::NewGuid().ToString('N')
$testUsername = 'fixture-admin'
$testPassword = 'fixture-' + [guid]::NewGuid().ToString('N')
$workRoot = Join-Path ([System.IO.Path]::GetTempPath()) ('subbridge-core-ab-ui-' + [guid]::NewGuid().ToString('N'))
$stdoutPath = Join-Path $workRoot 'fixture.stdout.log'
$stderrPath = Join-Path $workRoot 'fixture.stderr.log'
$fixtureExe = Join-Path $workRoot 'core-ab-ui-fixture.exe'
$fixtureProcess = $null

function Start-Fixture {
    $env:CORE_AB_UI_TEST_TOKEN = $testToken
    $env:CORE_AB_UI_TEST_USERNAME = $testUsername
    $env:CORE_AB_UI_TEST_PASSWORD = $testPassword
    $env:CORE_AB_UI_WORK_ROOT = $workRoot
    $script:fixtureProcess = Start-Process -FilePath $fixtureExe -WorkingDirectory $repoRoot -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath -WindowStyle Hidden -PassThru

    for ($attempt = 0; $attempt -lt 240; $attempt++) {
        if ($script:fixtureProcess.HasExited) {
            $stderr = if (Test-Path -LiteralPath $stderrPath) { [string](Get-Content -Raw -LiteralPath $stderrPath) } else { '' }
            throw "Core A/B fixture exited before becoming ready: $stderr"
        }
        $output = if (Test-Path -LiteralPath $stdoutPath) { [string](Get-Content -Raw -LiteralPath $stdoutPath) } else { '' }
        $match = [regex]::Match($output, 'CORE_AB_UI_FIXTURE_URL=(http://127\.0\.0\.1:\d+)')
        if ($match.Success) {
            return $match.Groups[1].Value
        }
        Start-Sleep -Milliseconds 100
    }
    throw 'Core A/B fixture did not become ready within 24 seconds.'
}

function Stop-Fixture {
    if ($null -ne $script:fixtureProcess -and -not $script:fixtureProcess.HasExited) {
        Stop-Process -Id $script:fixtureProcess.Id -Force
        [void]$script:fixtureProcess.WaitForExit(5000)
    }
    $script:fixtureProcess = $null
}

function Invoke-Playwright {
    param(
        [Parameter(Mandatory)]
        [string[]] $Command
    )

    $arguments = @('--yes', '--package', '@playwright/cli', 'playwright-cli', '--session', $session) + $Command
    $result = & $npx @arguments 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "Playwright CLI failed: $($result -join [Environment]::NewLine)"
    }
    return ($result -join [Environment]::NewLine)
}

function Login-With-TestCredentials {
    [void](Invoke-Playwright @('fill', '#username', $testUsername))
    [void](Invoke-Playwright @('fill', '#password', $testPassword))
    [void](Invoke-Playwright @('click', '#login-form button[type=submit]'))
}

function Invoke-CoreABPhase {
    param(
        [Parameter(Mandatory)]
        [string] $Phase
    )

    [void](Invoke-Playwright @('eval', "window.name = '$Phase'"))
    [void](Invoke-Playwright @('run-code', '--filename', $browserScript))
}

try {
    [void](New-Item -ItemType Directory -Path $workRoot -Force)
    & $go build -o $fixtureExe ./cmd/core-ab-ui-fixture
    if ($LASTEXITCODE -ne 0) {
        throw 'Could not build the local Core A/B UI fixture.'
    }
    $url = Start-Fixture
    [void](Invoke-Playwright @('open', $url))
    Login-With-TestCredentials
    Invoke-CoreABPhase 'core-ab-multisource-unsupported'
    Invoke-CoreABPhase 'core-ab-before-upload-1'
    Invoke-CoreABPhase 'core-ab-upload-1-delete'
    Invoke-CoreABPhase 'core-ab-after-delete-1'
    Invoke-CoreABPhase 'core-ab-after-restore-1-upload-2-replace'
    Invoke-CoreABPhase 'core-ab-after-replace-2'
    Invoke-CoreABPhase 'core-ab-final'
    [void](Invoke-Playwright @('close'))
    Write-Output 'Core A/B UI Playwright E2E passed.'
}
finally {
    Stop-Fixture
    try {
        [void](Invoke-Playwright @('close'))
    }
    catch {
        # The browser session may not have opened if an earlier setup step failed.
    }
    $daemon = Get-CimInstance Win32_Process -ErrorAction SilentlyContinue | Where-Object {
        $_.Name -eq 'node.exe' -and $_.CommandLine -like "*cliDaemon.js $session*"
    }
    if ($daemon) {
        Stop-Process -Id @($daemon | Select-Object -ExpandProperty ProcessId) -Force -ErrorAction SilentlyContinue
    }
    Remove-Item Env:CORE_AB_UI_TEST_TOKEN -ErrorAction SilentlyContinue
    Remove-Item Env:CORE_AB_UI_TEST_USERNAME -ErrorAction SilentlyContinue
    Remove-Item Env:CORE_AB_UI_TEST_PASSWORD -ErrorAction SilentlyContinue
    Remove-Item Env:CORE_AB_UI_WORK_ROOT -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $workRoot) {
        $resolvedWorkRoot = (Resolve-Path -LiteralPath $workRoot).Path
        $tempRoot = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath())
        if (-not $resolvedWorkRoot.StartsWith($tempRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
            throw "refusing to remove non-temporary fixture directory: $resolvedWorkRoot"
        }
        Remove-Item -LiteralPath $resolvedWorkRoot -Recurse -Force -ErrorAction SilentlyContinue
    }
}
