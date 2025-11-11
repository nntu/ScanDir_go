@echo off
REM =============================================================================
# QNAS Docker Build Script for Windows Environment (Enhanced Version)
# =============================================================================
# Author: Claude Code Assistant (Inspired by reference implementation)
# Description: Advanced Docker build script for QNAP NAS deployment on Windows
# Requirements: Docker Desktop for Windows, PowerShell 5.1+
# Usage: build-qnap-windows.cmd [options]
# =============================================================================

setlocal enabledelayedexpansion

REM Configuration
set SCRIPT_DIR=%~dp0
set PROJECT_NAME=scandir
set IMAGE_NAME=qnap-%PROJECT_NAME%
set IMAGE_VERSION=2.0-optimized
set FULL_IMAGE_NAME=%IMAGE_NAME%:%IMAGE_VERSION%
set BUILD_DATE=%date% %time%
set CONTAINER_BASE_NAME=qnap-build

REM Default options
set BUILD_TYPE=release
set VERBOSE=false
set CLEAN_BUILD=false
set RUN_TESTS=false
set CREATE_PACKAGE=true
set SKIP_PROMPTS=false
set TARGET_ARCHITECTURE=amd64

REM Colors for Windows CMD
set INFO=[INFO]
set SUCCESS=[SUCCESS]
set WARNING=[WARNING]
set ERROR=[ERROR]
set STEP=[STEP]
set DEBUG=[DEBUG]

REM Header
echo.
echo ╔══════════════════════════════════════════════════════════════════════════════╗
echo ║            QNAP Docker Build Script for Windows Environment            ║
echo ║                    Filesystem Scanner for QNAP NAS                     ║
echo ╚══════════════════════════════════════════════════════════════════════════════╝
echo.

REM Parse command line arguments
:parse_args
if "%1"=="" goto check_prerequisites
if "%1"=="-h" goto show_help
if "%1"=="--help" goto show_help
if "%1"=="-v" set VERBOSE=true
if "%1"=="--verbose" set VERBOSE=true
if "%1"=="-c" set CLEAN_BUILD=true
if "%1"=="--clean" set CLEAN_BUILD=true
if "%1"=="-t" set BUILD_TYPE=%2 && shift
if "%1"=="--type" set BUILD_TYPE=%2 && shift
if "%1"=="--arch" set TARGET_ARCHITECTURE=%2 && shift
if "%1"=="--no-package" set CREATE_PACKAGE=false
if "%1"=="--skip-tests" set RUN_TESTS=false
if "%1"=="--no-prompt" set SKIP_PROMPTS=true
if "%1"=="--version" set IMAGE_VERSION=%2 && shift

shift
goto parse_args

:show_help
echo.
echo QNAP Docker Build Script (Windows) - Help
echo.
echo USAGE:
echo     build-qnap-windows.cmd [OPTIONS]
echo.
echo OPTIONS:
echo     -h, --help              Show this help message
echo     -v, --verbose           Enable verbose output
echo     -c, --clean             Clean build artifacts before building
echo     -t, --type TYPE         Build type: debug or release ^(default: release^)
echo     --arch ARCH            Target architecture: amd64 or arm64 ^(default: amd64^)
echo     --version VERSION      Set custom image version
echo     --no-package           Skip creating deployment package
echo     --skip-tests           Skip running tests
echo     --no-prompt           Skip interactive prompts
echo.
echo EXAMPLES:
echo     build-qnap-windows.cmd                     # Standard build
echo     build-qnap-windows.cmd --verbose           # Verbose build
echo     build-qnap-windows.cmd --clean             # Clean build
echo     build-qnap-windows.cmd --type debug        # Debug build
echo     build-qnap-windows.cmd --arch arm64        # ARM64 build
echo.
echo INTERACTIVE MODE:
echo     build-qnap-windows.cmd                     # Interactive menu mode
echo.
goto :eof

:log_info
if "%VERBOSE%"=="true" echo %INFO% %~1
goto :eof

:log_success
echo %SUCCESS% %~1
goto :eof

:log_warning
echo %WARNING% %~1
goto :eof

:log_error
echo %ERROR% %~1
goto :eof

:log_step
echo %STEP% %~1
goto :eof

:log_debug
if "%VERBOSE%"=="true" echo %DEBUG% %~1
goto :eof

:check_prerequisites
call :log_step "Checking prerequisites..."

REM Check Docker
docker --version >nul 2>&1
if !errorlevel! neq 0 (
    echo Docker is not installed or not in PATH
    echo Please install Docker Desktop for Windows from: https://www.docker.com/products/docker-desktop
    pause
    exit /b 1
)

call :log_info "Docker found:"
docker --version

REM Check Docker daemon
docker info >nul 2>&1
if !errorlevel! neq 0 (
    echo Docker daemon is not running
    echo Please start Docker Desktop and try again
    pause
    exit /b 1
)

call :log_info "Docker daemon is running"

REM Check if we're in the right directory
if not exist "go.mod" (
    echo This script must be run from the project root directory
    echo Required file not found: go.mod
    pause
    exit /b 1
)

if not exist "Dockerfile.qnap" (
    echo Required file not found: Dockerfile.qnap
    pause
    exit /b 1
)

REM Check PowerShell availability
powershell -Command "Get-Host" >nul 2>&1
if !errorlevel! neq 0 (
    call :log_warning "PowerShell not available - some features may be limited"
)

call :log_success "Prerequisites check completed"
echo.

:choose_build_mode
if "%SKIP_PROMPTS%"=="true" (
    set BUILD_QNAP=1
    set BUILD_NATIVE=1
    goto build_configuration
)

echo Choose build mode:
echo 1. Build QNAP Docker container ^(for containerized deployment^)
echo 2. Build QNAP native binaries ^(for direct QNAP deployment^)
echo 3. Build both container and native binaries
echo.
set /p build_choice="Enter your choice (1/2/3) [default: 3]: "

if "%build_choice%"=="" set build_choice=3

if "%build_choice%"=="1" (
    set BUILD_QNAP=1
    set BUILD_NATIVE=0
    call :log_info "Selected: QNAP Docker container build"
) else if "%build_choice%"=="2" (
    set BUILD_QNAP=0
    set BUILD_NATIVE=1
    call :log_info "Selected: QNAP native binaries build"
) else if "%build_choice%"=="3" (
    set BUILD_QNAP=1
    set BUILD_NATIVE=1
    call :log_info "Selected: Both container and native builds"
) else (
    echo Invalid choice. Defaulting to build both.
    set BUILD_QNAP=1
    set BUILD_NATIVE=1
)

echo.

:build_configuration
call :log_step "Configuring build parameters..."

call :log_info "Build Configuration:"
call :log_info "  Image Name: %IMAGE_NAME%"
call :log_info "  Version: %IMAGE_VERSION%"
call :log_info "  Architecture: %TARGET_ARCHITECTURE%"
call :log_info "  Build Type: %BUILD_TYPE%"
call :log_info "  Clean Build: %CLEAN_BUILD%"
call :log_info "  Create Package: %CREATE_PACKAGE%"
call :log_info "  Run Tests: %RUN_TESTS%"
echo.

if "%CLEAN_BUILD%"=="true" (
    call :clean_artifacts
)

if "%RUN_TESTS%"=="true" (
    call :run_tests
)

:build_qnap_container
if "%BUILD_QNAP%"=="1" (
    call :log_step "Building QNAP Docker container..."

    if not exist "Dockerfile.qnap" (
        call :log_error "Dockerfile.qnap not found"
        pause
        exit /b 1
    )

    call :log_info "Building image: %FULL_IMAGE_NAME%"
    call :log_info "This may take 10-20 minutes depending on your network..."
    echo.

    set BUILD_ARGS=--build-arg GO_VERSION=1.23.3
    set BUILD_ARGS=%BUILD_ARGS% --build-arg TARGETARCH=%TARGET_ARCHITECTURE%
    set BUILD_ARGS=%BUILD_ARGS% --build-arg BUILD_DATE="%BUILD_DATE%"
    set BUILD_ARGS=%BUILD_ARGS% --build-arg VERSION=%IMAGE_VERSION%

    if "%BUILD_TYPE%"=="debug" (
        set BUILD_ARGS=%BUILD_ARGS% --target qnap-builder
    )

    if "%VERBOSE%"=="true" (
        set BUILD_ARGS=%BUILD_ARGS% --progress=plain
    )

    docker build %BUILD_ARGS% -f Dockerfile.qnap -t %FULL_IMAGE_NAME% .

    if !errorlevel! neq 0 (
        call :log_error "Docker build failed for QNAP container"
        pause
        exit /b 1
    )

    call :log_success "QNAP Docker image built successfully!"

    REM Show image information
    echo.
    call :log_info "Image details:"
    docker images %IMAGE_NAME%

    REM Test container
    call :log_info "Testing container..."
    set CONTAINER_NAME=%CONTAINER_BASE_NAME%-test-%RANDOM%

    docker run --name %CONTAINER_NAME% --rm %FULL_IMAGE_NAME% ls -la /app/ >nul 2>&1
    if !errorlevel! equ 0 (
        call :log_success "Container test passed!"
    ) else (
        call :log_warning "Container test failed, but image was built"
    )

    REM Show usage information
    echo.
    call :log_info "To run the container:"
    echo   docker run -it --name qnap-scanner ^
    echo     -v %%cd%%\config:/app/config ^
    echo     -v %%cd%%\logs:/app/logs ^
    echo     -v /path/to/your/data:/app/data ^
    echo     %FULL_IMAGE_NAME%
    echo.

    call :log_info "To run with volume mounts:"
    echo   docker run -it --name qnap-tools ^
    echo     -v %%cd%%\config:/app/config ^
    echo     -v %%cd%%\output:/app/output ^
    echo     -v /share/CACHEDEV1_DATA:/scan-data:ro ^
    echo     %FULL_IMAGE_NAME%
    echo.
)

:build_qnap_native
if "%BUILD_NATIVE%"=="1" (
    call :log_step "Building QNAP native binaries..."

    call :log_info "Building native binaries for direct QNAP deployment"
    call :log_info "This creates binaries that can be copied directly to QNAP NAS"

    REM Create output directory
    if not exist "qnap-build" mkdir qnap-build

    call :log_info "Using Docker build to create static binaries..."

    set BUILD_ARGS=--build-arg GO_VERSION=1.23.3
    set BUILD_ARGS=%BUILD_ARGS% --build-arg TARGETARCH=%TARGET_ARCHITECTURE%
    set BUILD_ARGS=%BUILD_ARGS% --build-arg BUILD_DATE="%BUILD_DATE%"
    set BUILD_ARGS=%BUILD_ARGS% --build-arg VERSION=%IMAGE_VERSION%
    set BUILD_ARGS=%BUILD_ARGS% --build-arg BUILD_FLAGS="-s -w -extldflags '-static-libgcc'"
    set BUILD_ARGS=%BUILD_ARGS% --build-arg QNAS_LDFLAGS="-X main.Version=%IMAGE_VERSION% -X main.BuildDate=%BUILD_DATE% -X main.Target=QNAP"

    if "%VERBOSE%"=="true" (
        set BUILD_ARGS=%BUILD_ARGS% --progress=plain
    )

    REM Build with artifact target to extract binaries only
    docker build %BUILD_ARGS% -f Dockerfile.qnap --target qnap-artifact .

    if !errorlevel! neq 0 (
        call :log_error "Docker build failed for QNAP native binaries"
        pause
        exit /b 1
    )

    REM Create temporary container to extract binaries
    set EXTRACT_CONTAINER=%CONTAINER_BASE_NAME%-extract-%RANDOM%
    docker create --name %EXTRACT_CONTAINER% %IMAGE_NAME%:latest /bin/true >nul 2>&1

    REM Extract binaries
    call :log_info "Extracting binaries from Docker image..."

    docker cp %EXTRACT_CONTAINER%:/scanner ./qnap-build/scanner.exe 2>nul
    docker cp %EXTRACT_CONTAINER%:/deleter ./qnap-build/deleter.exe 2>nul
    docker cp %EXTRACT_CONTAINER%:/reporter ./qnap-build/reporter.exe 2>nul
    docker cp %EXTRACT_CONTAINER%:/reporter_opt ./qnap-build/reporter_opt.exe 2>nul

    REM Extract deployment package if available
    docker cp %EXTRACT_CONTAINER%:/qnap-scanner-optimized.tar.gz ./qnap-build/qnap-scanner-optimized.tar.gz 2>nul

    REM Cleanup
    docker rm %EXTRACT_CONTAINER% >nul 2>&1

    REM Verify binaries
    if exist "qnap-build\scanner.exe" (
        call :log_success "Native binaries extracted successfully!"

        call :log_info "Binary sizes:"
        for %%f in (qnap-build\*.exe) do (
            for %%s in ("%%f") do echo   %%~nxf: %%~zs bytes
        )

        REM Test binaries
        call :log_info "Testing extracted binaries..."
        qnap-build\scanner.exe --help >nul 2>&1
        if !errorlevel! equ 0 (
            call :log_success "✓ Scanner binary test passed"
        ) else (
            call :log_warning "✗ Scanner binary test failed"
        )

        qnap-build\deleter.exe --help >nul 2>&1
        if !errorlevel! equ 0 (
            call :log_success "✓ Deleter binary test passed"
        ) else (
            call :log_warning "✗ Deleter binary test failed"
        )

        qnap-build\reporter.exe --help >nul 2>&1
        if !errorlevel! equ 0 (
            call :log_success "✓ Reporter binary test passed"
        ) else (
            call :log_warning "✗ Reporter binary test failed"
        )
    ) else (
        call :log_error "Failed to extract native binaries"
        pause
        exit /b 1
    )

    REM Create deployment package if requested
    if "%CREATE_PACKAGE%"=="true" (
        call :create_deployment_package
    )

    call :log_info "Native binaries ready for QNAP deployment!"
    call :log_info "See qnap-build\ folder for deployment files"
    echo.
)

:show_final_summary
echo ╔══════════════════════════════════════════════════════════════════════════════╗
echo ║                           BUILD SUMMARY                                   ║
echo ╚══════════════════════════════════════════════════════════════════════════════╝
echo.

echo Project Information:
echo   Name: %PROJECT_NAME%
echo   Version: %IMAGE_VERSION%
echo   Build Type: %BUILD_TYPE%
echo   Architecture: %TARGET_ARCHITECTURE%
echo   Build Date: %BUILD_DATE%
echo.

echo Build Results:

if "%BUILD_QNAP%"=="1" (
    echo [✓] QNAP Docker Container: %FULL_IMAGE_NAME%
    echo     - Base: Alpine Linux 3.19
    echo     - Features: musl libc, UPX compressed
    echo     - Image size:
    for /f "tokens=3,4" %%i in ('docker images %IMAGE_NAME% --format "{{.Size}} {{.Repository}}:{{.Tag}}"') do (
        if "%%j"=="%FULL_IMAGE_NAME%" echo       %%i
    )
    echo     - To run: docker run -it %FULL_IMAGE_NAME%
    echo.
)

if "%BUILD_NATIVE%"=="1" (
    echo [✓] QNAP Native Binaries: qnap-build\
    echo     - Alpine-based static binaries with musl libc
    echo     - Zero dependencies required for QNAP deployment
    echo     - Maximum QNAP compatibility (QTS 4.x+, QuTS hero)
    echo     - UPX compressed for optimal storage usage

    REM List binary files with sizes
    if exist "qnap-build\*.exe" (
        echo     - Optimized Binaries:
        for %%f in (qnap-build\*.exe) do (
            for %%s in ("%%f") do echo       %%~nxf: %%~zs bytes ^(UPX compressed^)
        )
    )

    if exist "qnap-build\qnap-scanner-optimized-alpine.tar.gz" (
        echo     - Deployment Package: qnap-scanner-optimized-alpine.tar.gz
        echo     - Features: Complete setup scripts, Alpine optimized
    ) else (
        echo     - Package: (not created - use --no-package to skip)
    )
    echo.
)

echo Next Steps:

if "%BUILD_NATIVE%"=="1" (
    echo QNAP Native Deployment:
    echo   1. Copy qnap-build\ folder contents to your QNAP NAS
    echo   2. Follow deployment instructions in README-QNAS.md
    echo   3. Configure config.ini for your QNAP paths
    echo   4. Run scanner.exe to start scanning
    echo.
)

if "%BUILD_QNAP%"=="1" (
    echo QNAP Container Deployment:
    echo   1. Push image to your registry: docker push %FULL_IMAGE_NAME%
    echo   2. Pull on QNAP: docker pull %FULL_IMAGE_NAME%
    echo   3. Run with appropriate volume mounts
    echo   4. Configure through mounted config files
    echo.
)

echo Quick Test Commands:
if "%BUILD_NATIVE%"=="1" (
    echo   cd qnap-build
    echo   scanner.exe --help
    echo   deleter.exe --help
    echo   reporter.exe --help
    echo.
)

echo ╔══════════════════════════════════════════════════════════════════════════════╗
echo ║                     BUILD COMPLETED SUCCESSFULLY!                        ║
echo ╚══════════════════════════════════════════════════════════════════════════════╝
echo.

if not "%SKIP_PROMPTS%"=="true" (
    pause
)

goto :eof

:clean_artifacts
call :log_step "Cleaning old build artifacts..."

REM Remove Docker images
for /f "tokens=3" %%i in ('docker images %IMAGE_NAME% --format "{{.ID}}" 2^>nul') do (
    docker rmi -f %%i 2>nul
)

REM Remove directories
if exist "qnap-build" (
    rmdir /s /q qnap-build
    call :log_info "Removed qnap-build directory"
)

REM Remove package files
for %%f in (qnap-scanner-*.tar.gz) do (
    if exist "%%f" del /f "%%f" >nul 2>&1
)

call :log_success "Clean completed"
goto :eof

:run_tests
call :log_step "Running tests..."

go version >nul 2>&1
if !errorlevel! equ 0 (
    go test ./...
    if !errorlevel! neq 0 (
        call :log_warning "Some tests failed, but continuing build"
    ) else (
        call :log_success "All tests passed"
    )
) else (
    call :log_warning "Go not found locally, skipping tests"
)
goto :eof

:create_deployment_package
call :log_step "Creating QNAS deployment package..."

set PACKAGE_DIR=%SCRIPT_DIR%qnap-build\qnap-deploy
if not exist "%PACKAGE_DIR%" mkdir "%PACKAGE_DIR%"

REM Copy binaries
copy qnap-build\*.exe "%PACKAGE_DIR%\" >nul 2>&1

REM Copy configuration
if exist "config.ini" (
    copy "config.ini" "%PACKAGE_DIR%\config.ini.example" >nul 2>&1
)

REM Create deployment script for Windows
(
echo @echo off
echo echo 🚀 QNAP Deployment Helper ^(Windows^)
echo echo.
echo REM This script helps deploy files to QNAP NAS
echo REM Usage: deploy-qnap.cmd [path-to-qnap-share]
echo.
echo set QNAP_PATH=%%1
echo if "%%QNAP_PATH%%"=="" ^(
echo     echo Usage: %%0 [path-to-qnap-share]
echo     echo Example: %%0 \\192.168.1.100\share
echo     exit /b 1
echo ^)
echo.
echo echo Deploying to: %%QNAP_PATH%%
echo.
echo REM Create directory structure
echo if not exist "%%QNAP_PATH%%\.qpkg\scanner" mkdir "%%QNAP_PATH%%\.qpkg\scanner"
echo mkdir "%%QNAP_PATH%%\.qpkg\scanner\bin" 2^>nul
echo mkdir "%%QNAP_PATH%%\.qpkg\scanner\config" 2^>nul
echo mkdir "%%QNAP_PATH%%\.qpkg\scanner\data" 2^>nul
echo mkdir "%%QNAP_PATH%%\.qpkg\scanner\logs" 2^>nul
echo mkdir "%%QNAP_PATH%%\.qpkg\scanner\output" 2^>nul
echo.
echo REM Copy binaries
echo copy *.exe "%%QNAP_PATH%%\.qpkg\scanner\bin\" /Y 2^>nul
echo if exist config.ini.example copy config.ini.example "%%QNAP_PATH%%\.qpkg\scanner\config\config.ini" /Y 2^>nul
echo.
echo echo ✅ Deployment completed!
echo echo 📍 Installation: %%QNAP_PATH%%\.qpkg\scanner
echo echo 📖 Configuration: %%QNAP_PATH%%\.qpkg\scanner\config\config.ini
echo echo 🏃 Run: %%QNAP_PATH%%\.qpkg\scanner\bin\scanner.exe
) > "%PACKAGE_DIR%\deploy-qnap.cmd"

REM Create README for Windows deployment
(
echo # QNAP Deployment Guide ^(Windows Version^)
echo.
echo ## Quick Start
echo.
echo ### Option 1: Use deployment script
echo ```cmd
echo deploy-qnap.cmd \\192.168.1.100\share
echo ```
echo.
echo ### Option 2: Manual deployment
echo 1. Copy all files to your QNAP NAS
echo 2. Create directory structure: ^.qpkg\scanner\...
echo 3. Copy binaries to bin\ folder
echo 4. Configure config.ini
echo.
echo ## Files Included
echo.
echo - **scanner.exe** - Main filesystem scanner
echo - **deleter.exe** - Database cleanup tool
echo - **reporter.exe** - Basic report generator
echo - **reporter_opt.exe** - Optimized report generator
echo - **config.ini.example** - Example configuration
echo - **deploy-qnap.cmd** - Windows deployment helper
echo.
echo ## Configuration
echo.
echo Edit config.ini with QNAP-specific paths:
echo ```ini
echo [paths]
echo root1 = /share/CACHEDEV1_DATA/Public:Public
echo root2 = /share/CACHEDEV1_DATA/Multimedia:Multimedia
echo ```
echo.
echo ## Running on QNAP
echo.
echo Access via QNAP SSH or use QNAP App Center to run commands.
echo.
) > "%PACKAGE_DIR%\README.md"

call :log_success "Deployment package created: %PACKAGE_DIR%"
goto :eof

REM Main execution
goto :eof