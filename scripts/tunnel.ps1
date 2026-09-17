<#
.SYNOPSIS
    Puts SADE behind a public HTTPS URL using a Cloudflare quick tunnel.

.DESCRIPTION
    The Go server speaks plain HTTP on localhost; cloudflared terminates TLS
    at Cloudflare's edge with a real, publicly trusted certificate and
    forwards the request in. That is what removes the browser's "not secure"
    warning, and it works on every device (phone included) with nothing to
    install on them - unlike a self-signed certificate, whose CA would have
    to be trusted per device.

    The script starts the tunnel, waits for the URL Cloudflare assigns, and
    writes it into .env as APP_PUBLIC_URL / APP_FRONTEND_URL /
    SERVER_CORS_ALLOWED_ORIGINS, plus AUTH_SESSION_COOKIE_SECURE=true. Those
    have to be set BEFORE the app starts: APP_PUBLIC_URL is what goes into
    every emailed magic-link and share link, so a stale value emails links
    that point at the old address.

    A quick tunnel gets a new random *.trycloudflare.com hostname on every
    run. For a stable hostname (and to register a Stripe webhook against it)
    use a named tunnel on your own Cloudflare domain - see README.md.

.EXAMPLE
    ./scripts/tunnel.ps1
    # then, in a second terminal, once it prints the URL:
    go run .
#>
[CmdletBinding()]
param(
    # The local address cloudflared forwards to. Must match SERVER_PORT.
    [string]$LocalUrl = 'http://localhost:8080',

    # Where to write the resolved URL.
    [string]$EnvFile = (Join-Path $PSScriptRoot '..\.env'),

    # Print the URL but leave .env alone.
    [switch]$NoEnvUpdate
)

$ErrorActionPreference = 'Stop'

if (-not (Get-Command cloudflared -ErrorAction SilentlyContinue)) {
    Write-Host 'cloudflared is not installed.' -ForegroundColor Yellow
    Write-Host 'Install it with:  winget install --id Cloudflare.cloudflared'
    exit 1
}

# Updates or appends KEY=VALUE, leaving every other line (secrets included)
# exactly as it was.
function Set-EnvValue {
    param([string]$Path, [string]$Key, [string]$Value)

    $lines = if (Test-Path $Path) { @(Get-Content -Path $Path) } else { @() }
    $pattern = '^\s*' + [regex]::Escape($Key) + '\s*='
    $found = $false
    $out = foreach ($line in $lines) {
        if ($line -match $pattern) { $found = $true; "$Key=$Value" } else { $line }
    }
    if (-not $found) { $out = @($out) + "$Key=$Value" }
    Set-Content -Path $Path -Value $out -Encoding utf8
}

$log = Join-Path ([System.IO.Path]::GetTempPath()) ("sade-tunnel-{0}.log" -f (Get-Date -Format 'yyyyMMdd-HHmmss'))
Write-Host "Starting quick tunnel to $LocalUrl ..." -ForegroundColor Cyan

# cloudflared writes its banner to stderr; merge both into one log we tail.
$proc = Start-Process -FilePath 'cloudflared' `
    -ArgumentList @('tunnel', '--no-autoupdate', '--url', $LocalUrl) `
    -RedirectStandardOutput $log -RedirectStandardError "$log.err" `
    -NoNewWindow -PassThru

$publicUrl = $null
$deadline = (Get-Date).AddSeconds(45)
while ((Get-Date) -lt $deadline -and -not $publicUrl) {
    Start-Sleep -Milliseconds 500
    foreach ($f in @($log, "$log.err")) {
        if (-not (Test-Path $f)) { continue }
        $m = Select-String -Path $f -Pattern 'https://[a-z0-9-]+\.trycloudflare\.com' -AllMatches |
             Select-Object -First 1
        if ($m) { $publicUrl = $m.Matches[0].Value; break }
    }
    if ($proc.HasExited -and -not $publicUrl) {
        Write-Host 'cloudflared exited before printing a URL. Log:' -ForegroundColor Red
        Get-Content "$log.err" -Tail 20
        exit 1
    }
}

if (-not $publicUrl) {
    Write-Host 'Timed out waiting for the tunnel URL.' -ForegroundColor Red
    Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
    exit 1
}

Write-Host ''
Write-Host "  Public HTTPS URL:  $publicUrl" -ForegroundColor Green
Write-Host ''

if (-not $NoEnvUpdate) {
    $EnvFile = [System.IO.Path]::GetFullPath($EnvFile)
    Set-EnvValue -Path $EnvFile -Key 'APP_PUBLIC_URL'   -Value $publicUrl
    Set-EnvValue -Path $EnvFile -Key 'APP_FRONTEND_URL' -Value $publicUrl
    # Same origin once the Go binary serves the built frontend; the Vite dev
    # server stays allowed so `npm run dev` keeps working alongside.
    Set-EnvValue -Path $EnvFile -Key 'SERVER_CORS_ALLOWED_ORIGINS' -Value "$publicUrl,http://localhost:5173"
    # The session cookie is only sent back over HTTPS from here on. The hop
    # into this process is still plain HTTP - that is the proxy's side of the
    # connection, not the browser's, so Secure is correct and not a lie.
    Set-EnvValue -Path $EnvFile -Key 'AUTH_SESSION_COOKIE_SECURE' -Value 'true'
    Write-Host "Wrote APP_PUBLIC_URL, APP_FRONTEND_URL, SERVER_CORS_ALLOWED_ORIGINS and"
    Write-Host "AUTH_SESSION_COOKIE_SECURE to $EnvFile"
}

Write-Host ''
Write-Host 'Now start the app in another terminal:  go run .' -ForegroundColor Cyan
Write-Host 'Leave this window open - closing it drops the tunnel. Ctrl+C to stop.'
Write-Host ''

try {
    while (-not $proc.HasExited) { Start-Sleep -Seconds 1 }
}
finally {
    if (-not $proc.HasExited) { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue }
    Write-Host 'Tunnel stopped.'
}
