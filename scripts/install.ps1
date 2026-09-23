# Installs the latest geoimg release on Windows.
#
#   irm https://github.com/osint-builders/geoimg/releases/latest/download/install.ps1 | iex
#
# Installs to %LOCALAPPDATA%\geoimg and adds it to your user PATH.
$ErrorActionPreference = 'Stop'
$repo = 'osint-builders/geoimg'
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$base = if ($env:GEOIMG_VERSION) { "https://github.com/$repo/releases/download/$env:GEOIMG_VERSION" } else { "https://github.com/$repo/releases/latest/download" }
$name = "geoimg_windows_$arch"
$dest = Join-Path $env:LOCALAPPDATA 'geoimg'
$tmp = Join-Path ([IO.Path]::GetTempPath()) ([Guid]::NewGuid())
New-Item -ItemType Directory -Force -Path $tmp, $dest | Out-Null

Write-Host "downloading $name.zip"
Invoke-WebRequest "$base/$name.zip" -OutFile "$tmp\$name.zip"
Invoke-WebRequest "$base/SHA256SUMS" -OutFile "$tmp\SHA256SUMS"
$expected = (Select-String -Path "$tmp\SHA256SUMS" -Pattern " $name.zip$").Line.Split(' ')[0]
$actual = (Get-FileHash "$tmp\$name.zip" -Algorithm SHA256).Hash.ToLower()
if ($expected -ne $actual) { throw "checksum mismatch for $name.zip" }

Expand-Archive "$tmp\$name.zip" -DestinationPath $tmp -Force
Copy-Item "$tmp\$name\geoimg.exe" "$dest\geoimg.exe" -Force
Remove-Item -Recurse -Force $tmp

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $dest) {
  [Environment]::SetEnvironmentVariable('Path', "$userPath;$dest", 'User')
  Write-Host "added $dest to your PATH (open a new terminal)"
}
& "$dest\geoimg.exe" -version
