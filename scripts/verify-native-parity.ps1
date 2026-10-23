$ErrorActionPreference = 'Stop'

# Run only synthetic loopback fixtures using independently pinned release assets.
$taskPlatform = "$(go env GOOS)/$(go env GOARCH)"
$taskPins = @{
    'linux/amd64' = @('prodex-x86_64-unknown-linux-gnu', '2dec9565fd94eb59aeb83ae5e7cc4b772db130cbd45258d86a2104c8f4ba186a')
    'linux/arm64' = @('prodex-aarch64-unknown-linux-gnu', '4d9a3ccb61c9f779083ef7dee3a0d363dc0c027824e37acabeb66e469e5ed25f')
    'darwin/amd64' = @('prodex-x86_64-apple-darwin', '6dc28e85b458cc29dfa164ca6ab00a1da4e5b00a01ba35957283dec5f82c3a4e')
    'darwin/arm64' = @('prodex-aarch64-apple-darwin', 'c5be320524fb436f159026f7907a3daedfcd397fb795f07d7cbc51ff5061a276')
    'windows/amd64' = @('prodex-x86_64-pc-windows-msvc.exe', 'e75aa83ce901fbdd593b729038e738ff7c30c152b4373075129af36e84de9a85')
    'windows/arm64' = @('prodex-aarch64-pc-windows-msvc.exe', '804c0bf905cdecee2eb029efa0e492d6f2fea5b177f3c6302cc66f07895b9656')
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
        Invoke-WebRequest -Uri "https://github.com/christiandoxa/prodex/releases/download/0.439.0/$taskName" -OutFile (Join-Path $taskRoot $taskName) -MaximumRetryCount 3
    }
    $taskProdex = Join-Path $taskRoot $taskAsset
    if ((Get-FileHash -Algorithm SHA256 $taskProdex).Hash.ToLowerInvariant() -ne $taskDigest) { throw 'official native executable digest mismatch' }
    $taskManifest = Join-Path $taskRoot 'release-manifest.json'
    if ((Get-FileHash -Algorithm SHA256 $taskManifest).Hash.ToLowerInvariant() -ne 'ba2f1f8d3430bd3fe64addd886ebee1207bf59b9db0d0e34b834b0dc7a0008f8') { throw 'official release manifest digest mismatch' }
    if (-not (Get-Content (Join-Path $taskRoot 'SHA256SUMS')).Contains("$taskDigest  $taskAsset")) { throw 'native executable absent from official SHA256SUMS' }
    $taskManifestValue = Get-Content -Raw $taskManifest | ConvertFrom-Json
    if ($taskManifestValue.version -ne '0.439.0' -or $taskManifestValue.commit -ne '433aad44bb249d5c25a231a6b06c2fa522a39a1f') { throw 'release source or version changed' }
    $taskOracleSource = Join-Path $taskRoot 'prodex-source'
    git clone --quiet --depth 1 --branch 0.439.0 https://github.com/christiandoxa/prodex.git $taskOracleSource
    if ($LASTEXITCODE -ne 0 -or (git -C $taskOracleSource rev-parse HEAD).Trim() -ne '433aad44bb249d5c25a231a6b06c2fa522a39a1f') { throw 'native oracle source pin mismatch' }
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
    Write-Output "PASS native Prodex 0.439.0 executable parity: $taskPlatform Godex $taskCommit"
} finally {
    Remove-Item -LiteralPath $taskRoot -Recurse -Force
}
