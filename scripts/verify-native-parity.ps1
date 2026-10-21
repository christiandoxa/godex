$ErrorActionPreference = 'Stop'

# Run only synthetic loopback fixtures using independently pinned release assets.
$taskPlatform = "$(go env GOOS)/$(go env GOARCH)"
$taskPins = @{
    'linux/amd64' = @('prodex-x86_64-unknown-linux-gnu', 'b736e1b2430b80ff2af35cff18e884b42ca83e8028c4fd47df5285f5d0d584e9')
    'linux/arm64' = @('prodex-aarch64-unknown-linux-gnu', 'd9ca7888a62409c4be1bd5923b9ea52e3a9b4f9b1af3fee2b1a5e852558f1097')
    'darwin/amd64' = @('prodex-x86_64-apple-darwin', '514c066af8c78e2e45a14be5db6a9aa7cf5a45c387db53796f2b10823d5515d0')
    'darwin/arm64' = @('prodex-aarch64-apple-darwin', '1dc6957b2659faece91d5c8234f4e4f7ea9a8f7ddbea68c2d9041042157538a6')
    'windows/amd64' = @('prodex-x86_64-pc-windows-msvc.exe', '6f852a4c523bf301c11e88d83ed2ee17467a175942ffc00e6b656d4d50e6c7c3')
    'windows/arm64' = @('prodex-aarch64-pc-windows-msvc.exe', 'f57893b66fb7ae620f65ef11a0026f04cd0518d566adf21efe2fa668f7dc451c')
}
if (-not $taskPins.ContainsKey($taskPlatform)) { throw "unsupported native parity platform: $taskPlatform" }
$taskCommit = (git rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0 -or (git status --porcelain)) { throw 'native parity requires a clean committed worktree' }
$taskSource = (git rev-parse --show-toplevel).Trim()
$taskRoot = Join-Path ([IO.Path]::GetTempPath()) ('godex-native-parity-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $taskRoot | Out-Null
try {
    $taskAsset, $taskDigest = $taskPins[$taskPlatform]
    foreach ($taskName in @($taskAsset, 'release-manifest.json', 'SHA256SUMS')) {
        Invoke-WebRequest -Uri "https://github.com/christiandoxa/prodex/releases/download/0.438.4/$taskName" -OutFile (Join-Path $taskRoot $taskName) -MaximumRetryCount 3
    }
    $taskProdex = Join-Path $taskRoot $taskAsset
    if ((Get-FileHash -Algorithm SHA256 $taskProdex).Hash.ToLowerInvariant() -ne $taskDigest) { throw 'official native executable digest mismatch' }
    $taskManifest = Join-Path $taskRoot 'release-manifest.json'
    if ((Get-FileHash -Algorithm SHA256 $taskManifest).Hash.ToLowerInvariant() -ne '28b7a894cd10fb69c528245e25dc01b9f204cfcc78b5ee575e22b4967c72c650') { throw 'official release manifest digest mismatch' }
    if (-not (Get-Content (Join-Path $taskRoot 'SHA256SUMS')).Contains("$taskDigest  $taskAsset")) { throw 'native executable absent from official SHA256SUMS' }
    $taskManifestValue = Get-Content -Raw $taskManifest | ConvertFrom-Json
    if ($taskManifestValue.version -ne '0.438.4' -or $taskManifestValue.commit -ne '4cf03ccff2b6132058e2359fd253ad6e2d107011') { throw 'release source or version changed' }
    $taskOracleSource = Join-Path $taskRoot 'prodex-source'
    git clone --quiet --depth 1 --branch 0.438.4 https://github.com/christiandoxa/prodex.git $taskOracleSource
    if ($LASTEXITCODE -ne 0 -or (git -C $taskOracleSource rev-parse HEAD).Trim() -ne '4cf03ccff2b6132058e2359fd253ad6e2d107011') { throw 'native oracle source pin mismatch' }
    $taskSuffix = if ($IsWindows) { '.exe' } else { '' }
    if (-not $IsWindows) { chmod +x $taskProdex }
    $taskGodex = Join-Path $taskRoot "godex$taskSuffix"
    $taskOracle = Join-Path $taskRoot "continuationparity$taskSuffix"
    go build -trimpath -o $taskGodex ./cmd/godex
    if ($LASTEXITCODE -ne 0) { throw 'build native Godex candidate failed' }
    go build -trimpath -o $taskOracle ./tools/continuationparity
    if ($LASTEXITCODE -ne 0) { throw 'build native executable oracle failed' }
    $taskFailures = @()
    foreach ($taskMode in @('--websocket-realtime', '--websocket', '--websocket-capacity', '--websocket-committed-idle', '--websocket-lookahead-idle', '--websocket-deadline', '--websocket-transport', '--websocket-compact-lineage', '--http-keepalive', '--http-compact', '--http-turn-transport', '--http-session-conflict', '--http-prompt-cache')) {
        & $taskOracle --prodex $taskProdex --prodex-source $taskOracleSource --godex $taskGodex --godex-source $taskSource --godex-commit $taskCommit $taskMode
        if ($LASTEXITCODE -ne 0) { $taskFailures += $taskMode; Write-Output "FAIL native $taskPlatform parity: $taskMode" }
    }
    go test ./tools/continuationparity -run '^TestProdex0438[234]' -count=1 -timeout=45s
    if ($LASTEXITCODE -ne 0) { throw 'native oracle negative controls failed' }
    if ($taskFailures.Count -ne 0) { throw "native $taskPlatform parity failed: $($taskFailures -join ', ')" }
    Write-Output "PASS native Prodex 0.438.4 executable parity: $taskPlatform Godex $taskCommit"
} finally {
    Remove-Item -LiteralPath $taskRoot -Recurse -Force
}
