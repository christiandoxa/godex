$ErrorActionPreference = 'Stop'

# Run only synthetic loopback fixtures using independently pinned release assets.
$taskPlatform = "$(go env GOOS)/$(go env GOARCH)"
$taskPins = @{
    'linux/amd64' = @('prodex-x86_64-unknown-linux-gnu', '3f2fcd776fee0335dc49910d0651adc43a55c8627913556c307755b1575dac3f')
    'linux/arm64' = @('prodex-aarch64-unknown-linux-gnu', 'd1c18604695bd5b457872103e3671f2c3ab47fdf01d383a23d3d02df18254106')
    'darwin/amd64' = @('prodex-x86_64-apple-darwin', '36ac2ab9f12652503d0c96369bd5cf21f3fe9275677af883c18ad4df108cf412')
    'darwin/arm64' = @('prodex-aarch64-apple-darwin', 'e7d878c1570bc62113b3683bdd21d9685c2050814f0467e5bd499da82051a406')
    'windows/amd64' = @('prodex-x86_64-pc-windows-msvc.exe', '61de5108f1e3f97a32cc132ade3847199042b438acfd61a2c6f5d0a30ebb9448')
    'windows/arm64' = @('prodex-aarch64-pc-windows-msvc.exe', '7a1690400b3b0a32dcf571f347a2871a5bb66fc0c084a95a3b72097576ec8554')
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
        Invoke-WebRequest -Uri "https://github.com/christiandoxa/prodex/releases/download/0.438.2/$taskName" -OutFile (Join-Path $taskRoot $taskName) -MaximumRetryCount 3
    }
    $taskProdex = Join-Path $taskRoot $taskAsset
    if ((Get-FileHash -Algorithm SHA256 $taskProdex).Hash.ToLowerInvariant() -ne $taskDigest) { throw 'official native executable digest mismatch' }
    $taskManifest = Join-Path $taskRoot 'release-manifest.json'
    if ((Get-FileHash -Algorithm SHA256 $taskManifest).Hash.ToLowerInvariant() -ne '93b5b5f90d59b23f800e47d17127ee7671d8a16ef72c6cabf30edd1093617725') { throw 'official release manifest digest mismatch' }
    if (-not (Get-Content (Join-Path $taskRoot 'SHA256SUMS')).Contains("$taskDigest  $taskAsset")) { throw 'native executable absent from official SHA256SUMS' }
    $taskManifestValue = Get-Content -Raw $taskManifest | ConvertFrom-Json
    if ($taskManifestValue.version -ne '0.438.2' -or $taskManifestValue.commit -ne 'decfed45f884fe2bef20911303beab7b198698b7') { throw 'release source or version changed' }
    $taskOracleSource = Join-Path $taskRoot 'prodex-source'
    git clone --quiet --depth 1 --branch 0.438.2 https://github.com/christiandoxa/prodex.git $taskOracleSource
    if ($LASTEXITCODE -ne 0 -or (git -C $taskOracleSource rev-parse HEAD).Trim() -ne 'decfed45f884fe2bef20911303beab7b198698b7') { throw 'native oracle source pin mismatch' }
    $taskSuffix = if ($IsWindows) { '.exe' } else { '' }
    if (-not $IsWindows) { chmod +x $taskProdex }
    $taskGodex = Join-Path $taskRoot "godex$taskSuffix"
    $taskOracle = Join-Path $taskRoot "continuationparity$taskSuffix"
    go build -trimpath -o $taskGodex ./cmd/godex
    if ($LASTEXITCODE -ne 0) { throw 'build native Godex candidate failed' }
    go build -trimpath -o $taskOracle ./tools/continuationparity
    if ($LASTEXITCODE -ne 0) { throw 'build native executable oracle failed' }
    foreach ($taskMode in @('--websocket-realtime', '--websocket', '--websocket-capacity', '--websocket-committed-idle', '--websocket-lookahead-idle', '--websocket-deadline')) {
        & $taskOracle --prodex $taskProdex --prodex-source $taskOracleSource --godex $taskGodex --godex-source $taskSource --godex-commit $taskCommit $taskMode
        if ($LASTEXITCODE -ne 0) { throw "native $taskPlatform parity failed: $taskMode" }
    }
    go test ./tools/continuationparity -run '^TestProdex04382' -count=1 -timeout=45s
    if ($LASTEXITCODE -ne 0) { throw 'native oracle negative controls failed' }
    Write-Output "PASS native Prodex 0.438.2 executable parity: $taskPlatform Godex $taskCommit"
} finally {
    Remove-Item -LiteralPath $taskRoot -Recurse -Force
}
