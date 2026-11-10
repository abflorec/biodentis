$ErrorActionPreference = 'Stop'
$project = Split-Path -Parent $PSScriptRoot
$binary = Join-Path $project 'bin\BIODENTIS.exe'
$demoData = Join-Path $project 'demo-data'
$site = 'http://127.0.0.1:8090'

function Get-LocalDemo {
    try {
        Invoke-RestMethod -UseBasicParsing -Uri "$site/health" -TimeoutSec 2
    } catch {
        $null
    }
}

try {
    if (-not (Test-Path -LiteralPath $binary -PathType Leaf)) {
        throw 'Falta bin\BIODENTIS.exe en la carpeta de la demo.'
    }

    $health = Get-LocalDemo
    if ($null -eq $health) {
        New-Item -ItemType Directory -Path $demoData -Force | Out-Null
        $startOptions = @{
            FilePath = $binary
            ArgumentList = '-demo', '-port', '8090', '-data', 'demo-data'
            WorkingDirectory = $project
            WindowStyle = 'Hidden'
            PassThru = $true
            RedirectStandardOutput = (Join-Path $demoData 'salida.log')
            RedirectStandardError = (Join-Path $demoData 'arranque.log')
        }
        $process = Start-Process @startOptions
        for ($attempt = 0; $attempt -lt 45; $attempt++) {
            $health = Get-LocalDemo
            if ($null -ne $health) { break }
            if ($process.HasExited) { break }
            Start-Sleep -Milliseconds 200
        }
    }

    if ($health.app -ne 'biodentis' -or $health.demo -ne $true) {
        throw 'No se pudo abrir la demo en el puerto 8090. Revisa demo-data\arranque.log o si otro programa usa ese puerto.'
    }
    Start-Process $site
} catch {
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}
