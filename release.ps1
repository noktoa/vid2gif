# Interactive release: build vid2gif.exe and vid2gif-app.exe, write checksums.txt, publish a GitHub release via gh.
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

$tag = Read-Host 'Git tag (e.g. v0.1.0)'
if ($tag -notmatch '^v?\d+\.\d+\.\d+(-[\w.]+)?$') { throw "'$tag' doesn't look like a version tag" }
if (git tag -l $tag) { throw "Tag $tag already exists locally" }
if (git status --porcelain) { Write-Warning 'Working tree has uncommitted changes; the release is tagged at the last commit without them.' }

$dist = Join-Path $PSScriptRoot 'dist'
Remove-Item $dist -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory $dist | Out-Null

Write-Host 'Building vid2gif.exe...'
$env:GOOS = 'windows'; $env:GOARCH = 'amd64'
go build -trimpath -ldflags '-s -w' -o "$dist\vid2gif.exe" ./cmd/vid2gif
if ($LASTEXITCODE) { throw 'go build failed' }
# deps.json must sit next to the exe, so it ships with it
Copy-Item deps.json $dist

# the desktop app embeds deps.json, so it ships alone
Write-Host 'Building vid2gif-app.exe...'
go build -trimpath -tags desktop,production -ldflags '-H windowsgui -s -w' -o "$dist\vid2gif-app.exe" ./cmd/vid2gif-app
if ($LASTEXITCODE) { throw 'go build (app) failed' }

# sha256sum format; LF endings, no BOM
$sums = Get-ChildItem $dist -File | ForEach-Object { "$((Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLower())  $($_.Name)" }
[IO.File]::WriteAllText("$dist\checksums.txt", ($sums -join "`n") + "`n")
$sums | Write-Host

if ((Read-Host "Push tag and publish release $tag? [y/N]") -ne 'y') { Write-Host 'Aborted; artifacts are in dist/.'; return }

git tag $tag
git push origin $tag
if ($LASTEXITCODE) { throw 'git push failed' }
gh release create $tag "$dist\vid2gif.exe" "$dist\vid2gif-app.exe" "$dist\deps.json" "$dist\checksums.txt" --title $tag --generate-notes
if ($LASTEXITCODE) { throw 'gh release create failed' }
