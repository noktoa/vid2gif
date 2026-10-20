# Interactive release: build vid2gif.exe, vid2gif-app.exe and the MSI (with the pinned yt-dlp, ffmpeg and deno inside, about 250 MB of downloads the first time), write checksums.txt, publish a GitHub release via gh.
# The MSI needs WiX: dotnet tool install --global wix --version 6.0.2
#   wix extension add --global WixToolset.UI.wixext/6.0.2 WixToolset.Util.wixext/6.0.2
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

$tag = Read-Host 'Git tag (e.g. v0.1.0)'
if ($tag -notmatch '^v?\d+\.\d+\.\d+(-[\w.]+)?$') { throw "'$tag' doesn't look like a version tag" }
# A tag left over from a failed release (local, remote or both) is fine; only a published release blocks it.
$released = gh release list --limit 1000 --json tagName --jq '.[].tagName'
if ($LASTEXITCODE) { throw 'gh release list failed (is gh installed and logged in?)' }
if ($released -contains $tag) { throw "A release for $tag already exists on GitHub" }
$localTag = [bool](git tag -l $tag)
$remoteTag = [bool](git ls-remote --tags origin "refs/tags/$tag")
if ($localTag -or $remoteTag) { Write-Warning "Tag $tag already exists (local: $localTag, remote: $remoteTag) with no release; it will be reused where it points, and the files are built from the working tree." }
if (git status --porcelain) { Write-Warning 'Working tree has uncommitted changes; the release is tagged at the last commit without them.' }

$dist = Join-Path $PSScriptRoot 'dist'
# no SilentlyContinue: a running dist\vid2gif-app.exe can't be deleted, and that should stop the release here
if (Test-Path $dist) {
    try { Remove-Item $dist -Recurse -Force -ErrorAction Stop }
    catch { throw "Can't clear dist\ ($($_.Exception.Message)). Close vid2gif-app.exe if it is running from dist\ and retry." }
}
New-Item -ItemType Directory $dist | Out-Null

Write-Host 'Building vid2gif.exe...'
$env:GOOS = 'windows'; $env:GOARCH = 'amd64'
go build -trimpath -ldflags '-s -w' -o "$dist\vid2gif.exe" ./cmd/vid2gif
if ($LASTEXITCODE) { throw 'go build failed' }
# deps.json must sit next to the exe, so it ships with it
Copy-Item deps.json $dist

# the desktop app embeds deps.json, so it ships alone
Write-Host 'Building vid2gif-app.exe...'
$ver = $tag.TrimStart('v')
# main.version is what the troubleshooting log reports ("vid2gif 0.1.0 on Windows ...")
go build -trimpath -tags desktop,production -ldflags "-H windowsgui -s -w -X main.version=$ver" -o "$dist\vid2gif-app.exe" ./cmd/vid2gif-app
if ($LASTEXITCODE) { throw 'go build (app) failed' }

# MSI versions are a.b.c only, so a pre-release tag (v0.1.0-rc1) ships the exes without an MSI
if ($ver -match '^\d+\.\d+\.\d+$') {
    # the MSI carries yt-dlp, ffmpeg and deno: fetch the pinned ones into bin\ (a no-op for any already there)
    Write-Host 'Fetching the helper tools...'
    go run ./build/installtools deps.json bin
    if ($LASTEXITCODE) { throw 'fetching the helper tools failed' }
    Write-Host "Building vid2gif-$ver.msi..."
    wix build installer\vid2gif.wxs -ext WixToolset.UI.wixext -ext WixToolset.Util.wixext -arch x64 -d Version=$ver -d AppExe=dist\vid2gif-app.exe -d AppIcon=cmd\vid2gif-app\icon.ico -d ToolsDir=bin -o "dist\vid2gif-$ver.msi"
    if ($LASTEXITCODE) { throw 'wix build failed (is WiX installed? see the top of this script)' }
    Remove-Item "$dist\*.wixpdb"
} else {
    Write-Warning "'$tag' is not a.b.c, so no MSI is built for it."
}

# sha256sum format; LF endings, no BOM
$sums = Get-ChildItem $dist -File | ForEach-Object { "$((Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLower())  $($_.Name)" }
[IO.File]::WriteAllText("$dist\checksums.txt", ($sums -join "`n") + "`n")
$sums | Write-Host

if ((Read-Host "Push tag and publish release $tag? [y/N]") -ne 'y') { Write-Host 'Aborted; artifacts are in dist/.'; return }

# a remote-only tag is left alone: tagging here would put a second, different tag at HEAD
if (-not ($localTag -or $remoteTag)) { git tag $tag }
if (-not $remoteTag) {
    git push origin $tag
    if ($LASTEXITCODE) { throw 'git push failed' }
}
gh release create $tag (Get-ChildItem $dist -File).FullName --title $tag --generate-notes
if ($LASTEXITCODE) { throw 'gh release create failed' }
