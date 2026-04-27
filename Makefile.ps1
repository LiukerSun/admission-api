param(
    [string]$Target = "help",
    [string]$DataDir,
    [string]$Profile,
    [int]$SampleRows,
    [int]$MaxReadRows
)

function Write-Help {
    @"
Usage: .\Makefile.ps1 -Target <target> [options]

Targets:
    help        Show this help message
    db          Initialize database (start db/redis, run migrations)
    dev         Start development server
    run         Start production server
    down        Stop all containers
    logs        Show production logs
    build       Build Docker image
    setup       Configure git hooks
    gaokao-import          Import gaokao data
    gaokao-import-reset    Reset and import gaokao data
    gaokao-import-sample   Import sample gaokao data
    gaokao-import-dev      Import dev gaokao data

Options for gaokao-import targets:
    -DataDir      Path to CSV directory (required)
    -Profile      Profile name (optional)
    -SampleRows   Number of sample rows (optional)
    -MaxReadRows  Maximum rows to read (optional)
"@
}

function Invoke-GitHooksSetup {
    git config core.hooksPath .githooks 2>&1 | Out-Null
}

function Test-FileExists {
    param([string]$Path)
    return Test-Path -Path $Path -PathType Leaf
}

function Ensure-EnvFile {
    if (-not (Test-FileExists -Path ".env")) {
        Write-Host "Creating .env from .env.example..."
        Copy-Item -Path ".env.example" -Destination ".env" -Force
    }
}

function Update-EnvVariable {
    param(
        [string]$Name,
        [string]$DefaultValue
    )
    
    $envContent = Get-Content -Path ".env" -Raw
    $pattern = "^$Name=.*"
    
    if ($envContent -match $pattern) {
        $currentValue = [regex]::Match($envContent, "$Name=(.*)").Groups[1].Value
        if (-not $currentValue) {
            $currentValue = $DefaultValue
        }
        $envContent = [regex]::Replace($envContent, $pattern, "$Name=$currentValue")
    } else {
        $envContent += "`n$Name=$DefaultValue"
    }
    
    Set-Content -Path ".env" -Value $envContent -NoNewline
    return $currentValue
}

function Wait-ForDatabase {
    param(
        [string]$PostgresPort
    )
    Write-Host "Waiting for database to be ready..."
    $ready = $false
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $output = docker-compose exec -T db pg_isready -U app -d admission 2>&1
            if ($LASTEXITCODE -eq 0) {
                $ready = $true
                break
            }
        } catch {
            # Ignore errors during waiting
        }
        Start-Sleep -Seconds 1
    }
    if (-not $ready) {
        Write-Error "Database failed to become ready"
        exit 1
    }
}

function Wait-ForRedis {
    Write-Host "Waiting for redis to be ready..."
    $ready = $false
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $output = docker-compose exec -T redis redis-cli ping 2>&1
            if ($output -match "PONG") {
                $ready = $true
                break
            }
        } catch {
            # Ignore errors during waiting
        }
        Start-Sleep -Seconds 1
    }
    if (-not $ready) {
        Write-Error "Redis failed to become ready"
        exit 1
    }
}

function Invoke-DbTarget {
    Invoke-GitHooksSetup
    Ensure-EnvFile
    
    Write-Host "Ensuring component variables in .env..."
    $postgresPort = Update-EnvVariable -Name "POSTGRES_PORT" -DefaultValue "5432"
    $redisPort = Update-EnvVariable -Name "REDIS_PORT" -DefaultValue "6379"
    
    Write-Host "Starting infrastructure containers..."
    docker-compose up -d db redis
    
    Wait-ForDatabase -PostgresPort $postgresPort
    Wait-ForRedis
    
    Write-Host "Running database migrations..."
    go run ./cmd/api -migrate up
    
    Write-Host "Database initialized successfully!"
}

function Invoke-DevTarget {
    Invoke-GitHooksSetup
    Ensure-EnvFile
    
    if (-not (Test-FileExists -Path "docs/docs.go")) {
        Write-Host "Generating swagger docs..."
        go run github.com/swaggo/swag/cmd/swag@v1.8.12 init -g cmd/api/main.go
    }
    
    docker-compose up -d
    
    Write-Host "Waiting for db..."
    Start-Sleep -Seconds 3
    
    go run ./cmd/api -migrate up
    go run ./cmd/api
}

function Invoke-RunTarget {
    docker-compose -f docker-compose.prod.yml up --build -d
}

function Invoke-DownTarget {
    docker-compose down
    docker-compose -f docker-compose.prod.yml down
}

function Invoke-LogsTarget {
    docker-compose -f docker-compose.prod.yml logs -f app
}

function Invoke-BuildTarget {
    docker build -t admission-api .
}

function Invoke-SetupTarget {
    git config core.hooksPath .githooks
    Write-Host "Git hooks configured. All commits will be validated."
}

function Invoke-GaokaoImport {
    param(
        [string]$DataDir,
        [string]$Profile,
        [int]$SampleRows,
        [int]$MaxReadRows,
        [switch]$Truncate,
        [switch]$SkipXgk
    )
    
    if (-not $DataDir) {
        Write-Error "Usage: .\Makefile.ps1 -Target gaokao-import -DataDir /absolute/path/to/csv-dir"
        exit 1
    }
    
    $args = @("-data-dir", "`"$DataDir`"")
    
    if ($Truncate) {
        $args += "-truncate"
    }
    
    if ($Profile) {
        $args += "-profile", $Profile
    }
    
    if ($SampleRows -gt 0) {
        $args += "-sample-rows", $SampleRows.ToString()
    }
    
    if ($MaxReadRows -gt 0) {
        $args += "-max-read-rows", $MaxReadRows.ToString()
    }
    
    if ($SkipXgk) {
        $args += "-skip-xgk"
    }
    
    Write-Host "Running: go run ./cmd/importer $($args -join ' ')"
    & go run ./cmd/importer $args
}

switch ($Target) {
    "help" {
        Write-Help
    }
    "db" {
        Invoke-DbTarget
    }
    "dev" {
        Invoke-DevTarget
    }
    "run" {
        Invoke-RunTarget
    }
    "down" {
        Invoke-DownTarget
    }
    "logs" {
        Invoke-LogsTarget
    }
    "build" {
        Invoke-BuildTarget
    }
    "setup" {
        Invoke-SetupTarget
    }
    "gaokao-import" {
        Invoke-GaokaoImport -DataDir $DataDir -Profile $Profile -SampleRows $SampleRows
    }
    "gaokao-import-reset" {
        Invoke-GaokaoImport -DataDir $DataDir -Profile $Profile -SampleRows $SampleRows -Truncate
    }
    "gaokao-import-sample" {
        if ($SampleRows -eq 0) {
            Write-Error "Usage: .\Makefile.ps1 -Target gaokao-import-sample -DataDir /path/to/csv-dir -SampleRows 1000"
            exit 1
        }
        Invoke-GaokaoImport -DataDir $DataDir -SampleRows $SampleRows -Truncate
    }
    "gaokao-import-dev" {
        if (-not $DataDir) {
            Write-Error "Usage: .\Makefile.ps1 -Target gaokao-import-dev -DataDir /path/to/csv-dir"
            exit 1
        }
        $sampleRows = if ($SampleRows -gt 0) { $SampleRows } else { 1000 }
        $maxReadRows = if ($MaxReadRows -gt 0) { $MaxReadRows } else { 5000 }
        Invoke-GaokaoImport -DataDir $DataDir -Profile "dev" -SampleRows $sampleRows -MaxReadRows $maxReadRows -Truncate -SkipXgk
    }
    default {
        Write-Error "Unknown target: $Target"
        Write-Help
        exit 1
    }
}