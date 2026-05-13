$ErrorActionPreference = "Stop"

$golangciLintVersion = "v1.64.8"
$golangciLint = Join-Path (go env GOPATH) "bin\golangci-lint.exe"

Write-Host "Go:"
go version

if (!(Test-Path $golangciLint) -or -not ((& $golangciLint --version) -match [regex]::Escape($golangciLintVersion))) {
    Write-Host "Installing golangci-lint $golangciLintVersion with the active Go toolchain..."
    go install "github.com/golangci/golangci-lint/cmd/golangci-lint@$golangciLintVersion"
}

Write-Host "golangci-lint:"
& $golangciLint --version

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
