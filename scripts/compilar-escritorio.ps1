$ErrorActionPreference = 'Stop'
$project = Split-Path -Parent $PSScriptRoot
Push-Location -LiteralPath $project
try {
    if (-not (Test-Path -LiteralPath 'node_modules/.bin/vite.cmd')) {
        npm.cmd ci
        if ($LASTEXITCODE -ne 0) { throw 'No se pudieron instalar las dependencias de la interfaz.' }
    }
    npm.cmd run build
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo compilar la interfaz.' }

    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    go build -tags desktop -trimpath -ldflags '-H windowsgui -s -w' -o bin/BIODENTIS-Escritorio.exe ./server
    if ($LASTEXITCODE -ne 0) { throw 'No se pudo compilar BIODENTIS Escritorio.' }
    Write-Host 'Listo: bin/BIODENTIS-Escritorio.exe'
} finally {
    Pop-Location
}
