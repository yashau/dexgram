$ErrorActionPreference = "Stop"

$golangciLintVersion = "v1.64.8"
$golangciLint = Join-Path (go env GOPATH) "bin\golangci-lint.exe"
$goBin = Join-Path (go env GOPATH) "bin"
$goversioninfo = Join-Path $goBin "goversioninfo.exe"

Write-Host "Go:"
go version

if (!(Test-Path $golangciLint) -or -not ((& $golangciLint --version) -match [regex]::Escape($golangciLintVersion))) {
    Write-Host "Installing golangci-lint $golangciLintVersion with the active Go toolchain..."
    go install "github.com/golangci/golangci-lint/cmd/golangci-lint@$golangciLintVersion"
}

Write-Host "golangci-lint:"
& $golangciLint --version

if (!(Test-Path $goversioninfo)) {
    Write-Host "Installing goversioninfo with the active Go toolchain..."
    go install "github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest"
}
$env:Path = "$goBin;$env:Path"

Write-Host "Checking generated version files..."
$generatedFiles = @("cmd/dexgram/version.go", "cmd/dexgram/resource.syso")
$beforeGenerate = @{}
foreach ($file in $generatedFiles) {
    if (Test-Path $file) {
        $beforeGenerate[$file] = (Get-FileHash $file -Algorithm SHA256).Hash
    } else {
        $beforeGenerate[$file] = "<missing>"
    }
}
go generate ./cmd/dexgram
$changedGeneratedFiles = @()
foreach ($file in $generatedFiles) {
    if (Test-Path $file) {
        $afterGenerate = (Get-FileHash $file -Algorithm SHA256).Hash
    } else {
        $afterGenerate = "<missing>"
    }
    if ($beforeGenerate[$file] -ne $afterGenerate) {
        $changedGeneratedFiles += $file
    }
}
if ($changedGeneratedFiles) {
    $changedGeneratedFiles | ForEach-Object { Write-Host "generated file changed during check: $_" }
    git diff -- cmd/dexgram/version.go cmd/dexgram/resource.syso
    Write-Error "Generated version files are not up to date. Run go generate ./cmd/dexgram."
    exit 1
}

Write-Host "Checking gofmt..."
$files = gofmt -l .
if ($files) {
    $files | ForEach-Object { Write-Error "gofmt needed: $_" }
    exit 1
}

Write-Host "Running tests..."
go test ./...

Write-Host "Running coverage..."
go test -cover ./...

Write-Host "Running golangci-lint..."
& $golangciLint run ./...
