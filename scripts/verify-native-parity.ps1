$ErrorActionPreference = 'Stop'

# Run only synthetic loopback fixtures using independently pinned release assets.
$taskPlatform = "$(go env GOOS)/$(go env GOARCH)"
$taskPins = @{
    'linux/amd64' = @('prodex-x86_64-unknown-linux-gnu', '5efa06fef57b0026eae7ec3cad23ac76b40c3e5d8a3afaee6c20feaeb5737923')
    'linux/arm64' = @('prodex-aarch64-unknown-linux-gnu', '4f743322e3b5653f47c52423263c769dfea543e24c1dfbddd1050942bfa2276d')
    'darwin/amd64' = @('prodex-x86_64-apple-darwin', '14e13b57ac15bcc0ff826ffd471c74735ce8802a2f72a0e28a40516030015c7a')
    'darwin/arm64' = @('prodex-aarch64-apple-darwin', 'a7b8d4af5ba9454ea22df64f36a69d45b3de8b54a65ba7e38cfded0df7f7ced8')
    'windows/amd64' = @('prodex-x86_64-pc-windows-msvc.exe', '1f5ba38c456219e64044d4bc7b84fb16ca63b6385f6f2f493c357e693496417f')
    'windows/arm64' = @('prodex-aarch64-pc-windows-msvc.exe', '27da114337ce9cf799615e2aeddad332947263b2eb0f2cb1fec6447481fc2403')
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
        Invoke-WebRequest -Uri "https://github.com/christiandoxa/prodex/releases/download/0.438.3/$taskName" -OutFile (Join-Path $taskRoot $taskName) -MaximumRetryCount 3
    }
    $taskProdex = Join-Path $taskRoot $taskAsset
    if ((Get-FileHash -Algorithm SHA256 $taskProdex).Hash.ToLowerInvariant() -ne $taskDigest) { throw 'official native executable digest mismatch' }
    $taskManifest = Join-Path $taskRoot 'release-manifest.json'
    if ((Get-FileHash -Algorithm SHA256 $taskManifest).Hash.ToLowerInvariant() -ne '300a8f046041a95f9757e4cbdb72917d37927683b6e1af442492254e2f1a37da') { throw 'official release manifest digest mismatch' }
    if (-not (Get-Content (Join-Path $taskRoot 'SHA256SUMS')).Contains("$taskDigest  $taskAsset")) { throw 'native executable absent from official SHA256SUMS' }
    $taskManifestValue = Get-Content -Raw $taskManifest | ConvertFrom-Json
    if ($taskManifestValue.version -ne '0.438.3' -or $taskManifestValue.commit -ne '42042d765100fc06895a27e0904ffb64842092e6') { throw 'release source or version changed' }
    $taskOracleSource = Join-Path $taskRoot 'prodex-source'
    git clone --quiet --depth 1 --branch 0.438.3 https://github.com/christiandoxa/prodex.git $taskOracleSource
    if ($LASTEXITCODE -ne 0 -or (git -C $taskOracleSource rev-parse HEAD).Trim() -ne '42042d765100fc06895a27e0904ffb64842092e6') { throw 'native oracle source pin mismatch' }
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
    go test ./tools/continuationparity -run '^TestProdex0438[23]' -count=1 -timeout=45s
    if ($LASTEXITCODE -ne 0) { throw 'native oracle negative controls failed' }
    if ($taskFailures.Count -ne 0) { throw "native $taskPlatform parity failed: $($taskFailures -join ', ')" }
    Write-Output "PASS native Prodex 0.438.3 executable parity: $taskPlatform Godex $taskCommit"
} finally {
    Remove-Item -LiteralPath $taskRoot -Recurse -Force
}
