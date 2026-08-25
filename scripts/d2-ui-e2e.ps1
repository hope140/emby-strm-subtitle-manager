[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$go = (Get-Command go -ErrorAction Stop).Source
$npx = (Get-Command npx -ErrorAction Stop).Source
$browserScript = Join-Path $repoRoot 'scripts/d2-ui-e2e-browser.js'
$session = 'd2-ui-e2e-' + [guid]::NewGuid().ToString('N')
$testToken = [guid]::NewGuid().ToString('N')
$testUsername = 'fixture-admin'
$testPassword = 'fixture-admin-password-2026'
$stdoutPath = Join-Path ([System.IO.Path]::GetTempPath()) ('d2-ui-fixture-' + [guid]::NewGuid().ToString('N') + '.stdout.log')
$stderrPath = Join-Path ([System.IO.Path]::GetTempPath()) ('d2-ui-fixture-' + [guid]::NewGuid().ToString('N') + '.stderr.log')
$fixtureExe = Join-Path ([System.IO.Path]::GetTempPath()) ('d2-ui-fixture-' + [guid]::NewGuid().ToString('N') + '.exe')
$fixtureProcess = $null

function Start-Fixture {
    param(
        [Parameter(Mandatory)]
        [bool] $RemoteSearchEnabled,
        [Parameter(Mandatory)]
        [int] $ArtifactTTLSeconds
    )

    $env:D2_UI_TEST_TOKEN = $testToken
    $env:D2_UI_TEST_USERNAME = $testUsername
    $env:D2_UI_TEST_PASSWORD = $testPassword
    $env:D2_UI_REMOTE_SEARCH_ENABLED = if ($RemoteSearchEnabled) { 'true' } else { 'false' }
    $env:D2_UI_ARTIFACT_TTL_SECONDS = [string]$ArtifactTTLSeconds
    $script:fixtureProcess = Start-Process -FilePath $fixtureExe -WorkingDirectory $repoRoot -RedirectStandardOutput $stdoutPath -RedirectStandardError $stderrPath -WindowStyle Hidden -PassThru

    for ($attempt = 0; $attempt -lt 240; $attempt++) {
        if ($script:fixtureProcess.HasExited) {
            $stderr = if (Test-Path -LiteralPath $stderrPath) { [string](Get-Content -Raw -LiteralPath $stderrPath) } else { '' }
            throw "Fake Emby fixture exited before becoming ready: $stderr"
        }
        $output = if (Test-Path -LiteralPath $stdoutPath) { [string](Get-Content -Raw -LiteralPath $stdoutPath) } else { '' }
        $match = [regex]::Match($output, 'D2_UI_FIXTURE_URL=(http://127\.0\.0\.1:\d+)')
        if ($match.Success) {
            return $match.Groups[1].Value
        }
        Start-Sleep -Milliseconds 100
    }
    throw 'Fake Emby fixture did not become ready within 24 seconds.'
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

function Invoke-PageCode {
    param(
        [Parameter(Mandatory)]
        [string] $Phase
    )

    [void](Invoke-Playwright @('eval', "window.name = '$Phase'"))
    [void](Invoke-Playwright @('run-code', '--filename', $browserScript))
}

function Login-With-TestCredentials {
    $snapshot = [string](Invoke-Playwright @('snapshot') | Out-String)
    $usernameMatch = [regex]::Match($snapshot, 'textbox "管理员用户名" \[ref=([^\]]+)\]')
    $passwordMatch = [regex]::Match($snapshot, 'textbox "管理员密码" \[ref=([^\]]+)\]')
    $loginMatch = [regex]::Match($snapshot, 'button [^\r\n]*?\[ref=([^\]]+)\]')
    if (-not $usernameMatch.Success -or -not $passwordMatch.Success -or -not $loginMatch.Success) {
        throw "Playwright snapshot did not expose the expected login controls: $snapshot"
    }
    [void](Invoke-Playwright @('fill', $usernameMatch.Groups[1].Value, $testUsername))
    [void](Invoke-Playwright @('fill', $passwordMatch.Groups[1].Value, $testPassword))
    [void](Invoke-Playwright @('click', $loginMatch.Groups[1].Value))
}

try {
    & $go build -o $fixtureExe ./cmd/d2-ui-fixture
    if ($LASTEXITCODE -ne 0) {
        throw 'Could not build the local D2 UI fixture.'
    }

    $disabledURL = Start-Fixture -RemoteSearchEnabled:$false -ArtifactTTLSeconds 600
    [void](Invoke-Playwright @('open', $disabledURL))
    Login-With-TestCredentials
    Invoke-PageCode -Phase 'disabled'
    Stop-Fixture

    $enabledURL = Start-Fixture -RemoteSearchEnabled:$true -ArtifactTTLSeconds 600
    [void](Invoke-Playwright @('goto', $enabledURL))
    Login-With-TestCredentials
    Invoke-PageCode -Phase 'enabled'
    Stop-Fixture

    $expiryURL = Start-Fixture -RemoteSearchEnabled:$true -ArtifactTTLSeconds 1
    [void](Invoke-Playwright @('goto', $expiryURL))
    Login-With-TestCredentials
    Invoke-PageCode -Phase 'expiry'
    [void](Invoke-Playwright @('close'))
    Write-Output 'D2 UI Playwright E2E passed.'
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
    Remove-Item Env:D2_UI_TEST_TOKEN -ErrorAction SilentlyContinue
    Remove-Item Env:D2_UI_TEST_USERNAME -ErrorAction SilentlyContinue
    Remove-Item Env:D2_UI_TEST_PASSWORD -ErrorAction SilentlyContinue
    Remove-Item Env:D2_UI_REMOTE_SEARCH_ENABLED -ErrorAction SilentlyContinue
    Remove-Item Env:D2_UI_ARTIFACT_TTL_SECONDS -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $stdoutPath, $stderrPath, $fixtureExe -Force -ErrorAction SilentlyContinue
}
