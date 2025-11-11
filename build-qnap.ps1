# =============================================================================
# QNAS BUILD SCRIPT - Filesystem Scanner for QNAP NAS (PowerShell Version)
# =============================================================================
# Author: Claude Code Assistant
# Description: Advanced build script for QNAP NAS deployment
# Usage: .\build-qnap.ps1 [options]
# Requirements: PowerShell 5.1+, Docker Desktop
# =============================================================================

[CmdletBinding()]
param (
    [string]$Version = "2.0-optimized",
    [string]$GoVersion = "1.23.3",
    [ValidateSet("debug", "release")]
    [string]$Type = "release",
    [switch]$Clean,
    [switch]$Verbose,
    [switch]$SkipTests,
    [switch]$NoPackage,
    [switch]$Push,
    [string]$Registry = "",
    [switch]$Help
)

# Global variables
$ScriptDir = $PSScriptRoot
$ProjectName = "scandir"
$DockerImage = "qnap-$($ProjectName)-builder"
$OutputDir = Join-Path $ScriptDir "qnap-build"
$PackageName = "qnap-$($ProjectName)-$($Version)"
$BuildDate = Get-Date -Format "yyyy-MM-dd HH:mm:ss UTC"

# ANSI color codes for PowerShell console
$Colors = @{
    Red = "Red"
    Green = "Green"
    Yellow = "Yellow"
    Blue = "Blue"
    Magenta = "Magenta"
    Cyan = "Cyan"
    White = "White"
    Gray = "Gray"
}

# Utility functions
function Write-HostColored {
    param(
        [string]$Message,
        [ConsoleColor]$Color = $Colors.White
    )
    Write-Host $Message -ForegroundColor $Color
}

function Write-Info {
    param([string]$Message)
    Write-HostColored "[INFO] $Message" $Colors.Cyan
}

function Write-Success {
    param([string]$Message)
    Write-HostColored "[SUCCESS] $Message" $Colors.Green
}

function Write-Warning {
    param([string]$Message)
    Write-HostColored "[WARNING] $Message" $Colors.Yellow
}

function Write-Error {
    param([string]$Message)
    Write-HostColored "[ERROR] $Message" $Colors.Red
}

function Write-Step {
    param([string]$Message)
    Write-HostColored "[STEP] $Message" $Colors.Magenta
}

function Show-Help {
    @"
QNAS Build Script - Filesystem Scanner for QNAP NAS (PowerShell Version)

USAGE:
    .\build-qnap.ps1 [OPTIONS]

OPTIONS:
    -Version VERSION       Build version (default: 2.0-optimized)
    -GoVersion VERSION     Go version (default: 1.23.3)
    -Type TYPE            Build type: debug or release (default: release)
    -Clean               Clean build (remove old artifacts)
    -Verbose             Verbose output
    -SkipTests           Skip running tests
    -NoPackage           Skip creating deployment package
    -Push                Push images to registry
    -Registry URL        Container registry URL
    -Help                Show this help message

EXAMPLES:
    .\build-qnap.ps1                              # Standard release build
    .\build-qnap.ps1 -Type debug -Verbose       # Debug build with verbose
    .\build-qnap.ps1 -Clean -SkipTests         # Clean build without tests
    .\build-qnap.ps1 -Push -Registry registry.example.com  # Build and push

ADVANCED EXAMPLES:
    .\build-qnap.ps1 -Version "1.0.0-custom" -Clean -Verbose
    .\build-qnap.ps1 -GoVersion "1.22.0" -Type debug
    .\build-qnap.ps1 -Push -Registry "myregistry.com" -NoPackage

"@
}

function Test-Prerequisites {
    Write-Step "Validating build prerequisites..."

    # Check PowerShell version
    if ($PSVersionTable.PSVersion.Major -lt 5) {
        Write-Error "PowerShell 5.1+ is required. Current version: $($PSVersionTable.PSVersion)"
        return $false
    }

    # Check Docker
    try {
        $null = Get-Command docker -ErrorAction Stop
        docker --version | Out-Null
    }
    catch {
        Write-Error "Docker is required but not installed or not in PATH"
        return $false
    }

    # Check Docker daemon
    try {
        $null = docker info 2>$null
        if ($LASTEXITCODE -ne 0) {
            Write-Error "Docker daemon is not running"
            return $false
        }
    }
    catch {
        Write-Error "Failed to connect to Docker daemon"
        return $false
    }

    # Check required files
    $requiredFiles = @("go.mod", "Dockerfile.qnap")
    foreach ($file in $requiredFiles) {
        if (-not (Test-Path $file)) {
            Write-Error "Required file not found: $file"
            return $false
        }
    }

    Write-Success "Prerequisites validation passed"
    return $true
}

function Invoke-CleanBuild {
    if ($Clean) {
        Write-Step "Cleaning old build artifacts..."

        # Remove Docker images
        try {
            $images = docker images --format "{{.ID}}" $DockerImage 2>$null
            if ($images) {
                Write-Info "Removing old Docker images..."
                $images | ForEach-Object { docker rmi -f $_ 2>$null | Out-Null }
            }
        }
        catch {
            Write-Warning "Failed to remove Docker images: $($_.Exception.Message)"
        }

        # Remove build directory
        if (Test-Path $OutputDir) {
            Write-Info "Removing build directory: $OutputDir"
            Remove-Item -Path $OutputDir -Recurse -Force -ErrorAction SilentlyContinue
        }

        # Remove package files
        $packageFiles = @("qnap-scanner-optimized.tar.gz", "$PackageName.tar.gz", "$PackageName.zip")
        foreach ($file in $packageFiles) {
            if (Test-Path $file) {
                Remove-Item -Path $file -Force -ErrorAction SilentlyContinue
            }
        }

        Write-Success "Clean completed"
    }
}

function Invoke-Tests {
    if (-not $SkipTests) {
        Write-Step "Running tests..."

        try {
            $null = Get-Command go -ErrorAction Stop
            $result = go test ./... 2>&1
            if ($LASTEXITCODE -eq 0) {
                Write-Success "All tests passed"
            } else {
                Write-Error "Tests failed: $result"
                return $false
            }
        }
        catch {
            Write-Warning "Go not found locally, skipping tests"
        }
    }
    else {
        Write-Warning "Skipping tests as requested"
    }
    return $true
}

function Invoke-DockerBuild {
    Write-Step "Building QNAS Docker image..."

    $buildArgs = @(
        "--build-arg", "GO_VERSION=$GoVersion",
        "--build-arg", "TARGETARCH=amd64",
        "--build-arg", "BUILD_DATE=$BuildDate",
        "-t", "$($DockerImage):latest",
        "-t", "$($DockerImage):$($Version)",
        "-f", "Dockerfile.qnap"
    )

    if ($Type -eq "debug") {
        $buildArgs += @("--target", "qnap-builder")
    }

    if ($Verbose) {
        $buildArgs += @("--progress", "plain")
    }

    Write-Info "Building with arguments: $($buildArgs -join ' ')"

    try {
        docker build $buildArgs . 2>&1 | Write-Host
        if ($LASTEXITCODE -eq 0) {
            Write-Success "Docker image built successfully"
            return $true
        } else {
            Write-Error "Docker image build failed"
            return $false
        }
    }
    catch {
        Write-Error "Docker build error: $($_.Exception.Message)"
        return $false
    }
}

function Invoke-ExtractBinaries {
    Write-Step "Extracting binaries from Docker image..."

    if (-not (Test-Path $OutputDir)) {
        New-Item -Path $OutputDir -ItemType Directory -Force | Out-Null
    }

    $containerName = "qnap-extract-$(Get-Random)"
    $binaries = @("scanner", "deleter", "reporter", "reporter_opt")

    try {
        # Create temporary container
        $null = docker create --name $containerName "$($DockerImage):latest"
        if ($LASTEXITCODE -ne 0) {
            throw "Failed to create temporary container"
        }
        Write-Info "Created temporary container: $containerName"

        # Extract binaries
        foreach ($binary in $binaries) {
            Write-Info "Extracting $binary..."
            $outputPath = Join-Path $OutputDir $binary
            $null = docker cp "$($containerName):/$($binary)" $outputPath 2>$null

            if (Test-Path $outputPath) {
                $size = (Get-Item $outputPath).Length
                Write-Success "Extracted $binary ($size bytes)"
            } else {
                Write-Warning "Failed to extract $binary (may not exist)"
            }
        }

        # Extract deployment package if requested
        if ($NoPackage -eq $false) {
            Write-Info "Extracting deployment package..."
            $null = docker cp "$($containerName):/qnap-scanner-optimized.tar.gz" "." 2>$null
            if (Test-Path "qnap-scanner-optimized.tar.gz") {
                Write-Success "Extracted deployment package"
            } else {
                Write-Warning "Failed to extract deployment package"
            }
        }
    }
    catch {
        Write-Error "Error during binary extraction: $($_.Exception.Message)"
        return $false
    }
    finally {
        # Cleanup container
        docker rm $containerName 2>$null | Out-Null
    }

    Write-Success "Binary extraction completed"
    return $true
}

function Test-Binaries {
    Write-Step "Verifying extracted binaries..."

    $binaries = @("scanner", "deleter", "reporter", "reporter_opt")
    $allValid = $true

    foreach ($binary in $binaries) {
        $binaryPath = Join-Path $OutputDir $binary

        if (Test-Path $binaryPath) {
            try {
                $fileInfo = Get-Item $binaryPath
                if ($fileInfo.Length -gt 0) {
                    Write-Success "$($binary): ✓ Valid executable ($($fileInfo.Length) bytes)"
                } else {
                    Write-Error "$($binary): ✗ Empty file"
                    $allValid = $false
                }
            }
            catch {
                Write-Error "$($binary): ✗ Error accessing file"
                $allValid = $false
            }
        } else {
            Write-Warning "$($binary): Not found"
        }
    }

    if ($allValid) {
        Write-Success "All binaries verified successfully"
        return $true
    } else {
        Write-Error "Binary verification failed"
        return $false
    }
}

function New-DeploymentPackage {
    if ($NoPackage) {
        Write-Warning "Skipping deployment package creation"
        return
    }

    Write-Step "Creating QNAS deployment package..."

    $packageDir = Join-Path $OutputDir $PackageName
    if (-not (Test-Path $packageDir)) {
        New-Item -Path $packageDir -ItemType Directory -Force | Out-Null
    }

    # Copy binaries
    $binaries = Get-ChildItem -Path $OutputDir -File | Where-Object { $_.Name -match "^(scanner|deleter|reporter)" }
    foreach ($binary in $binaries) {
        Copy-Item -Path $binary.FullName -Destination $packageDir -Force
    }

    # Copy configuration if exists
    if (Test-Path "config.ini") {
        Copy-Item -Path "config.ini" -Destination "$($packageDir)\config.ini.example" -Force
    }

    # Create deployment script
    $deployScript = @"
@echo off
echo 🚀 Deploying Filesystem Scanner to QNAP NAS...

REM Detect QNAP paths
if exist "C:\share\CACHEDEV1_DATA\.qpkg" (
    set QPKG_DIR=C:\share\CACHEDEV1_DATA\.qpkg\scanner
) else if exist "\\share\CACHEDEV1_DATA\.qpkg" (
    set QPKG_DIR=\\share\CACHEDEV1_DATA\.qpkg\scanner
) else (
    set QPKG_DIR=%TEMP%\scanner
    mkdir "%QPKG_DIR%"
)

REM Create directories
if not exist "%QPKG_DIR%\bin" mkdir "%QPKG_DIR%\bin"
if not exist "%QPKG_DIR%\config" mkdir "%QPKG_DIR%\config"
if not exist "%QPKG_DIR%\data" mkdir "%QPKG_DIR%\data"
if not exist "%QPKG_DIR%\logs" mkdir "%QPKG_DIR%\logs"
if not exist "%QPKG_DIR%\output" mkdir "%QPKG_DIR%\output"

REM Copy binaries
copy scanner* "%QPKG_DIR%\bin\" >nul

REM Copy configuration
if not exist "%QPKG_DIR%\config\config.ini" (
    copy config.ini.example "%QPKG_DIR%\config\config.ini" >nul
    echo 📝 Created default configuration
)

echo ✅ Deployment complete!
echo 📍 Installation: %QPKG_DIR%
echo 📖 Configuration: %QPKG_DIR%\config\config.ini
echo 🏃 Run: %QPKG_DIR%\bin\scanner.exe
"@

    $deployScript | Out-File -FilePath "$($packageDir)\deploy.bat" -Encoding ASCII

    # Create README
    $readmeContent = @"
# Filesystem Scanner for QNAP NAS

## Version: $Version
## Build Date: $BuildDate
## Build Platform: Windows PowerShell

## Quick Start (Windows Build)

1. Copy this package to your QNAP NAS
2. Run: `deploy.bat`
3. Edit: `config.ini` to set your scan paths
4. Run: `bin\scanner.exe`

## Files

- `scanner.exe` - Main filesystem scanner
- `deleter.exe` - Database cleanup tool
- `reporter.exe` - Basic report generator
- `reporter_opt.exe` - Optimized report generator
- `config.ini.example` - Example configuration
- `deploy.bat` - Windows deployment script

## Build Information

- Built using: PowerShell $($PSVersionTable.PSVersion)
- Docker image: $($DockerImage):$($Version)
- Go version: $GoVersion
- Build type: $Type

## Support

For detailed documentation, please refer to the project repository.
"@

    $readmeContent | Out-File -FilePath "$($packageDir)\README.md" -Encoding UTF8

    # Create package using tar if available, otherwise zip
    $tarAvailable = Get-Command tar -ErrorAction SilentlyContinue
    if ($tarAvailable) {
        $tarPath = Join-Path $OutputDir "$($PackageName).tar.gz"
        Push-Location $OutputDir
        try {
            tar -czf "$($PackageName).tar.gz" $PackageName
            Write-Success "Created deployment package: $tarPath"
        }
        finally {
            Pop-Location
        }
    } else {
        # Fallback to PowerShell Compress-Archive
        $zipPath = Join-Path $OutputDir "$($PackageName).zip"
        try {
            Compress-Archive -Path $packageDir -DestinationPath $zipPath -Force
            Write-Success "Created deployment package: $zipPath"
        }
        catch {
            Write-Warning "Failed to create ZIP package: $($_.Exception.Message)"
        }
    }
}

function Invoke-RegistryPush {
    if (-not $Push) {
        return
    }

    if ([string]::IsNullOrEmpty($Registry)) {
        Write-Error "Registry URL required for pushing"
        return $false
    }

    Write-Step "Pushing to container registry..."

    $fullImage = "$($Registry)/$($DockerImage):$($Version)"

    try {
        docker tag "$($DockerImage):$($Version)" $fullImage
        if ($LASTEXITCODE -eq 0) {
            docker push $fullImage
            if ($LASTEXITCODE -eq 0) {
                Write-Success "Pushed to registry: $fullImage"
                return $true
            } else {
                Write-Error "Failed to push to registry"
                return $false
            }
        } else {
            Write-Error "Failed to tag image for registry"
            return $false
        }
    }
    catch {
        Write-Error "Registry push error: $($_.Exception.Message)"
        return $false
    }
}

function Show-Summary {
    Write-Step "Build Summary"

    Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
    Write-Host "Project:        $ProjectName"
    Write-Host "Version:        $Version"
    Write-Host "Build Type:     $Type"
    Write-Host "Go Version:     $GoVersion"
    Write-Host "Build Date:     $BuildDate"
    Write-Host "Output Directory: $OutputDir"
    Write-Host ""

    if (Test-Path $OutputDir) {
        Write-Host "Generated Files:" -ForegroundColor Green
        Get-ChildItem $OutputDir -File | ForEach-Object {
            $size = if ($_.Length -gt 0) { "$([math]::Round($_.Length / 1KB, 2))KB" } else { "0KB" }
            Write-Host "  $($_.Name.PadRight(30)) $($size.PadLeft(10))"
        }
    }

    Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan

    Write-Success "QNAS build completed successfully! 🎉"
}

# Main execution
function Main {
    Write-Host ""
    Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
    Write-Host "🚀 QNAS BUILD SCRIPT - Filesystem Scanner for QNAP NAS (PowerShell)" -ForegroundColor White
    Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Cyan
    Write-Host ""

    if ($Help) {
        Show-Help
        return
    }

    # Show build info
    Write-Info "Build Configuration:"
    Write-Info "  Version: $Version"
    Write-Info "  Go Version: $GoVersion"
    Write-Info "  Build Type: $Type"
    Write-Info "  Clean Build: $Clean"
    Write-Info "  Verbose: $Verbose"
    Write-Info "  Create Package: $(-not $NoPackage)"
    Write-Info "  Push to Registry: $Push"
    if ($Push) { Write-Info "  Registry: $Registry" }
    Write-Host ""

    # Validate prerequisites
    if (-not (Test-Prerequisites)) {
        exit 1
    }

    # Execute build pipeline
    try {
        Invoke-CleanBuild
        if (-not (Invoke-Tests)) { exit 1 }
        if (-not (Invoke-DockerBuild)) { exit 1 }
        if (-not (Invoke-ExtractBinaries)) { exit 1 }
        if (-not (Test-Binaries)) { exit 1 }
        New-DeploymentPackage
        if (-not (Invoke-RegistryPush)) { exit 1 }
        Show-Summary
    }
    catch {
        Write-Error "Build failed with error: $($_.Exception.Message)"
        exit 1
    }
}

# Handle Ctrl+C gracefully
[Console]::TreatControlCAsInput = $false
try {
    Main
}
catch [System.Management.Automation.HaltCommandException] {
    Write-Warning "Build interrupted by user"
    exit 1
}
catch {
    Write-Error "Unexpected error: $($_.Exception.Message)"
    exit 1
}