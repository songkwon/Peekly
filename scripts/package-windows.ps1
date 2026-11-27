param(
  [Parameter(Mandatory = $true)]
  [ValidateSet('amd64', 'arm64')]
  [string]$Architecture
)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
Set-Location (Join-Path $PSScriptRoot '..')
$root = (Get-Location).Path
$expectedMachine = @{ amd64 = 0x8664; arm64 = 0xaa64 }[$Architecture]
$stage = Join-Path ([System.IO.Path]::GetTempPath()) ("peekly-package-" + [guid]::NewGuid())
$resource = Join-Path $root "wails_windows_$Architecture.syso"
$zipName = "Peekly-windows-$Architecture.zip"
$original = @{ GOOS = $env:GOOS; GOARCH = $env:GOARCH; CGO_ENABLED = $env:CGO_ENABLED; GOBIN = $env:GOBIN }
New-Item -ItemType Directory -Path $stage | Out-Null

if (Test-Path $resource) {
  Remove-Item -LiteralPath $stage
  throw "Remove the existing generated resource before packaging. $resource"
}

try {
  if ($env:PEEKLY_SKIP_FRONTEND -ne '1') {
    npm --prefix frontend ci
    npm --prefix frontend run build
  }

  $version = (Get-Content frontend/package.json -Raw | ConvertFrom-Json).version
  $metadata = Get-Content build/windows/info.json -Raw | ConvertFrom-Json
  if ($metadata.fixed.file_version -ne $version -or $metadata.fixed.product_version -ne $version) {
    throw 'Windows and frontend versions differ.'
  }
  if ($env:PEEKLY_RELEASE_VERSION -and $env:PEEKLY_RELEASE_VERSION -ne $version) {
    throw 'Windows build version does not match the release tag.'
  }

  $env:GOOS = ''
  $env:GOARCH = ''
  $env:CGO_ENABLED = ''
  $env:GOBIN = Join-Path $stage 'tool'
  go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0
  & (Join-Path $env:GOBIN 'wails3.exe') generate syso -arch $Architecture -icon build/windows/icon.ico -manifest build/windows/wails.exe.manifest -info build/windows/info.json -out $resource

  $env:GOOS = 'windows'
  $env:GOARCH = $Architecture
  $env:CGO_ENABLED = '0'
  $bundle = Join-Path $stage 'bundle'
  New-Item -ItemType Directory -Path $bundle | Out-Null
  $exe = Join-Path $bundle 'Peekly.exe'
  go build -mod=readonly -tags production -trimpath -buildvcs=false -ldflags '-s -w -H windowsgui' -o $exe .

  $file = [System.IO.File]::OpenRead($exe)
  $reader = [System.IO.BinaryReader]::new($file)
  try {
    if ($reader.ReadUInt16() -ne 0x5a4d) { throw 'Missing DOS signature.' }
    $file.Position = 0x3c
    $peOffset = $reader.ReadInt32()
    $file.Position = $peOffset
    if ($reader.ReadUInt32() -ne 0x4550) { throw 'Missing PE signature.' }
    if ($reader.ReadUInt16() -ne $expectedMachine) { throw 'Unexpected Windows architecture.' }
  } finally {
    $reader.Dispose()
  }

  $zip = Join-Path $stage $zipName
  Compress-Archive -LiteralPath $exe -DestinationPath $zip
  $archive = [System.IO.Compression.ZipFile]::OpenRead($zip)
  try {
    if ($null -eq $archive.GetEntry('Peekly.exe')) { throw 'Package is missing Peekly.exe.' }
  } finally {
    $archive.Dispose()
  }
  New-Item -ItemType Directory -Path build-out -Force | Out-Null
  Move-Item -LiteralPath $zip -Destination (Join-Path 'build-out' $zipName) -Force
  Write-Output (Join-Path $root "build-out/$zipName")
} finally {
  foreach ($name in $original.Keys) {
    [Environment]::SetEnvironmentVariable($name, $original[$name], 'Process')
  }
  if (Test-Path $resource) { Remove-Item -LiteralPath $resource -Force }
  if (Test-Path $stage) { Remove-Item -LiteralPath $stage -Recurse -Force }
}
