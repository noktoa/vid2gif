# Builds the MSI as 0.0.1 and 0.0.2, then installs, upgrades and uninstalls it
# per-user (no admin) in a temp folder and checks what is left behind.
# Needs `wix` with WixToolset.UI.wixext and WixToolset.Util.wixext, Go (only if
# dist\vid2gif-app.exe is missing) and the network the first time (the MSI
# carries the pinned tools, which build\installtools fetches into bin\, about
# 250 MB, and reuses after that).
# Refuses to run if vid2gif is already installed. %LOCALAPPDATA%\vid2gif is
# moved aside for the test and put back afterwards, even on failure.
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot
$dist = Join-Path $root 'dist'
$exe = Join-Path $dist 'vid2gif-app.exe'
$upgradeCode = if ((Get-Content "$PSScriptRoot\vid2gif.wxs" -Raw) -match 'UpgradeCode="([^"]+)"') { $Matches[1] } else { throw 'FAIL: no UpgradeCode in vid2gif.wxs' }
$menuLnk = Join-Path ([Environment]::GetFolderPath('Programs')) 'vid2gif.lnk'
$dataDir = Join-Path $env:LOCALAPPDATA 'vid2gif'
$msiApi = New-Object -ComObject WindowsInstaller.Installer

# Installed-apps entries for this user and the machine. A per-user MSI is
# registered under Installer\UserData\<SID>, not in HKCU\...\Uninstall.
function Entries {
    @($msiApi.RelatedProducts("{$upgradeCode}")) | ForEach-Object {
        [pscustomobject]@{ Version = $msiApi.ProductInfo($_, 'VersionString'); PerUser = $msiApi.ProductInfo($_, 'AssignmentType') -eq '0' }
    }
}
function Check($ok, $what) { if (-not $ok) { throw "FAIL: $what" }; Write-Host "ok: $what" }
function Msi($argList, $log) {
    $p = Start-Process msiexec.exe -ArgumentList "$argList /qn /l*v `"$log`"" -Wait -PassThru
    if ($p.ExitCode -notin 0, 3010) { throw "FAIL: msiexec $argList exited $($p.ExitCode); see $log" }
}
function Build($v, $appExe) {
    $out = Join-Path $dist "vid2gif-$v.msi"
    wix build "$PSScriptRoot\vid2gif.wxs" -ext WixToolset.UI.wixext -ext WixToolset.Util.wixext -arch x64 -d Version=$v -d AppExe=$appExe -d AppIcon="$root\cmd\vid2gif-app\icon.ico" -d ToolsDir="$root\bin" -o $out
    if ($LASTEXITCODE) { throw "FAIL: wix build $v" }
    $out
}

# (a) never touch a real install
if (@(Entries).Count) {
    throw 'vid2gif is already installed on this machine. Uninstall it first; this test will not upgrade or remove a real install.'
}

if (-not (Test-Path $exe)) {
    Write-Host 'Building vid2gif-app.exe...'
    Push-Location $root
    try { go build -trimpath -tags desktop,production -ldflags '-H windowsgui -s -w' -o $exe ./cmd/vid2gif-app } finally { Pop-Location }
    if ($LASTEXITCODE) { throw 'FAIL: go build' }
}
Push-Location $root
try { go run ./build/installtools deps.json bin } finally { Pop-Location }
if ($LASTEXITCODE) { throw 'FAIL: fetching the helper tools' }
# 0.0.2 gets a different exe (one byte appended; Windows ignores it), so the
# upgrade check can tell the files apart by hash
$exe2 = Join-Path $dist 'vid2gif-app-test2.exe'
Copy-Item $exe $exe2
[IO.File]::AppendAllText($exe2, "`n")
$msi1 = Build '0.0.1' $exe
$msi2 = Build '0.0.2' $exe2

$t = Join-Path ([IO.Path]::GetTempPath()) "vid2gif-msi-test-$([guid]::NewGuid().ToString('N').Substring(0, 8))"
$parent = Join-Path $t 'a b'
$dir = Join-Path $parent 'vid2gif'
$backup = $null
$dataSet = $false
try {
    New-Item -ItemType Directory $parent, "$t\clips", "$t\logs" | Out-Null
    Set-Content "$parent\other.txt" 'sibling'
    Set-Content "$t\clips\a.gif" 'clip'
    # (b) keep the user's real settings and logs out of the way
    if (Test-Path $dataDir) {
        $backup = "$dataDir.test-backup-$([guid]::NewGuid().ToString('N').Substring(0, 8))"
        Move-Item $dataDir $backup
        Write-Host "moved $dataDir aside to $backup"
    }
    $dataSet = $true
    New-Item -ItemType Directory $dataDir | Out-Null
    $marker = [guid]::NewGuid().ToString()
    Set-Content "$dataDir\settings.json" "{`"marker`":`"$marker`"}"

    # 1. per-user install of 0.0.1 into a folder with a space
    Msi "/i `"$msi1`" MSIINSTALLPERUSER=1 ALLUSERS=`"`" APPLICATIONFOLDER=`"$dir`"" "$t\logs\install1.log"
    Check (Test-Path "$dir\vid2gif-app.exe") 'exe installed'
    Check (Test-Path $menuLnk) 'Start Menu shortcut'
    $e = @(Entries)
    Check ($e.Count -eq 1 -and $e[0].PerUser -and $e[0].Version -eq '0.0.1') 'one per-user installed-apps entry, 0.0.1'
    foreach ($tool in 'yt-dlp.exe', 'ffmpeg.exe', 'deno.exe') { Check ((Test-Path "$dir\bin\$tool") -and (Get-FileHash "$dir\bin\$tool").Hash -eq (Get-FileHash "$root\bin\$tool").Hash) "bin\$tool installed from the package" }
    Check ((Test-Path "$dir\LICENSE.txt") -and (Test-Path "$dir\THIRD-PARTY-NOTICES.txt")) 'license and third-party notices installed'

    # 2. upgrade to 0.0.2 without naming the folder: it must stay where it was
    Msi "/i `"$msi2`" MSIINSTALLPERUSER=1 ALLUSERS=`"`"" "$t\logs\install2.log"
    $e = @(Entries)
    Check ($e.Count -eq 1 -and $e[0].Version -eq '0.0.2') 'one uninstall entry after upgrade, 0.0.2'
    Check ((Test-Path "$dir\vid2gif-app.exe") -and (Get-FileHash "$dir\vid2gif-app.exe").Hash -eq (Get-FileHash $exe2).Hash) 'new exe in the same folder'
    Check ((@('yt-dlp.exe', 'ffmpeg.exe', 'deno.exe') | Where-Object { -not (Test-Path "$dir\bin\$_") }).Count -eq 0) 'upgrade: tools still installed'
    Check ((Get-Content "$dataDir\settings.json" -Raw) -match $marker) 'upgrade keeps settings'

    # 3. uninstall; the leftovers of an interrupted tool download must go too
    Set-Content "$dir\bin\ffmpeg.exe.new", "$dir\bin\.deno.download" 'partial'
    Msi "/x `"$msi2`"" "$t\logs\uninstall1.log"
    Check (-not (Test-Path "$dir\vid2gif-app.exe")) 'exe removed'
    Check (-not (Test-Path $menuLnk)) 'shortcut removed'
    Check (-not (Test-Path "$dir\bin")) 'bin removed'
    Check (-not (Test-Path $dataDir)) "$dataDir removed"
    Check (-not (Test-Path $dir)) 'empty install folder removed'
    Check (@(Entries).Count -eq 0) 'uninstall entry removed'
    Check (Test-Path "$parent\other.txt") 'sibling file kept'
    Check (Test-Path "$t\clips\a.gif") 'clips kept'

    # 4. install into a folder the user already had, with its own bin folder
    $own = Join-Path $t 'my apps'
    New-Item -ItemType Directory "$own\bin" | Out-Null
    Set-Content "$own\keep.txt", "$own\bin\mine.txt" 'mine'
    Msi "/i `"$msi2`" MSIINSTALLPERUSER=1 ALLUSERS=`"`" APPLICATIONFOLDER=`"$own`"" "$t\logs\install3.log"
    Check (Test-Path "$own\vid2gif-app.exe") 'exe installed into existing folder'
    Msi "/x `"$msi2`"" "$t\logs\uninstall2.log"
    Check ((Test-Path "$own\keep.txt") -and -not (Test-Path "$own\vid2gif-app.exe")) 'existing folder and its file kept, exe removed'
    Check ((Test-Path "$own\bin\mine.txt") -and @(Get-ChildItem "$own\bin" -Force).Count -eq 1) 'existing bin keeps the user''s file, tools removed'
    # 5. tools are optional features: none ticked installs none; one ticked
    #    installs just that one; unticking it later in Change removes its file
    $sel = Join-Path $t 'sel'
    $own2 = @('MSIINSTALLPERUSER=1', 'ALLUSERS=""', "APPLICATIONFOLDER=`"$sel`"") -join ' '
    Msi "/i `"$msi2`" $own2 ADDLOCAL=Main" "$t\logs\install4.log"
    Check ((Test-Path "$sel\vid2gif-app.exe") -and -not (Test-Path "$sel\bin")) 'no tool ticked: app installed, no bin'
    # a tool the app downloaded itself (first launch, nothing on PATH) goes too
    New-Item -ItemType Directory "$sel\bin" | Out-Null
    Set-Content "$sel\bin\yt-dlp.exe", "$sel\bin\.yt-dlp.download" 'app-downloaded'
    Msi "/x `"$msi2`"" "$t\logs\uninstall3.log"
    Check (-not (Test-Path "$sel\bin")) 'uninstall removes a tool the app downloaded, and bin'
    Msi "/i `"$msi2`" $own2 ADDLOCAL=Main,ToolFfmpeg" "$t\logs\install5.log"
    Check ((Test-Path "$sel\bin\ffmpeg.exe") -and -not (Test-Path "$sel\bin\yt-dlp.exe") -and -not (Test-Path "$sel\bin\deno.exe")) 'only the ticked tool is in bin'
    Msi "/i `"$msi2`" REMOVE=ToolFfmpeg" "$t\logs\change1.log"
    Check ((Test-Path "$sel\vid2gif-app.exe") -and -not (Test-Path "$sel\bin\ffmpeg.exe")) 'unticking ffmpeg in Change removes its file, keeps the app'
    Msi "/x `"$msi2`"" "$t\logs\uninstall4.log"
    Write-Host 'PASS'
} finally {
    # nothing was installed before the test (checked above), so any vid2gif
    # product now is the test's own
    foreach ($code in @($msiApi.RelatedProducts("{$upgradeCode}"))) {
        $p = Start-Process msiexec.exe -ArgumentList "/x $code /qn" -Wait -PassThru
        if ($p.ExitCode -notin 0, 3010) { Write-Host "!!! could not uninstall the test install $code (exit $($p.ExitCode)); remove vid2gif in Installed apps" -ForegroundColor Red }
    }
    # only the fake data folder made above is deleted here, never the real one
    if ($dataSet -and (Test-Path $dataDir)) {
        try { Remove-Item $dataDir -Recurse -Force } catch { Write-Host "could not delete the test's $dataDir`: $_" -ForegroundColor Red }
    }
    if ($backup) {
        if (-not (Test-Path $dataDir)) { Move-Item $backup $dataDir; Write-Host "restored $dataDir" }
        if (Test-Path $backup) {
            Write-Host "!!! Your real $dataDir is saved at $backup. Delete $dataDir and rename the saved folder back by hand." -ForegroundColor Red
            throw "FAIL: could not restore $dataDir from $backup"
        }
    }
    Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
}
