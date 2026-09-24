$ErrorActionPreference = "Stop"

$repository = "benoitpetit/mira"
$version = if ($env:MIRA_VERSION) { $env:MIRA_VERSION } else { "latest" }
$installDir = if ($env:MIRA_INSTALL_DIR) { $env:MIRA_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "Mira\bin" }
$architecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()

switch ($architecture) {
  "X64" { $arch = "amd64" }
  "Arm64" { $arch = "arm64" }
  default { throw "MIRA does not support Windows architecture $architecture (supported: X64, Arm64)" }
}

$asset = "mira-windows-$arch.zip"
$downloadRoot = if ($version -eq "latest") {
  "https://github.com/$repository/releases/latest/download"
} else {
  "https://github.com/$repository/releases/download/v$version"
}

$temporaryDir = Join-Path ([System.IO.Path]::GetTempPath()) ("mira-install-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $temporaryDir | Out-Null
try {
  $archive = Join-Path $temporaryDir $asset
  $checksums = Join-Path $temporaryDir "SHA256SUMS"
  Invoke-WebRequest -Uri "$downloadRoot/$asset" -OutFile $archive
  Invoke-WebRequest -Uri "$downloadRoot/SHA256SUMS" -OutFile $checksums

  $checksumLine = Select-String -Path $checksums -Pattern ("^\s*([0-9a-fA-F]{64})\s+$([regex]::Escape($asset))\s*$") | Select-Object -First 1
  if (-not $checksumLine) { throw "SHA256SUMS does not contain $asset" }
  $expected = $checksumLine.Matches[0].Groups[1].Value.ToLowerInvariant()
  $actual = (Get-FileHash -Path $archive -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($expected -ne $actual) { throw "Checksum verification failed for $asset" }

  $extractedDir = Join-Path $temporaryDir "extracted"
  Expand-Archive -Path $archive -DestinationPath $extractedDir
  $binary = Join-Path $extractedDir "mira.exe"
  if (-not (Test-Path $binary)) { throw "Archive does not contain mira.exe" }
  New-Item -ItemType Directory -Force -Path $installDir | Out-Null
  Copy-Item -Force $binary (Join-Path $installDir "mira.exe")
  Write-Output "MIRA installed at $(Join-Path $installDir 'mira.exe')"
  if (-not (($env:Path -split [IO.Path]::PathSeparator) -contains $installDir)) {
    Write-Output "Add $installDir to your PATH to run: mira.exe --version"
  }
} finally {
  Remove-Item -Recurse -Force $temporaryDir -ErrorAction SilentlyContinue
}
