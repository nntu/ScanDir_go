@echo off
REM =============================================================================
REM QNAS BUILD SCRIPT - Filesystem Scanner for QNAP NAS (Windows CMD Version)
REM =============================================================================
REM Author: Claude Code Assistant
REM Description: Build optimized binaries for QNAP NAS deployment
REM Usage: build-qnap.cmd [options]
REM =============================================================================

setlocal enabledelayedexpansion

REM Configuration
set SCRIPT_DIR=%~dp0
set PROJECT_NAME=scandir
set VERSION=2.0-optimized
set GO_VERSION=1.23.3
set BUILD_DATE=%date% %time%
set DOCKER_IMAGE=qnap-%PROJECT_NAME%-builder
set OUTPUT_DIR=%SCRIPT_DIR%qnap-build
set PACKAGE_NAME=qnap-%PROJECT_NAME%-%VERSION%

REM Default options
set BUILD_TYPE=release
set CLEAN_BUILD=false
set VERBOSE=false
set SKIP_TESTS=false
set CREATE_PACKAGE=true
set PUSH_TO_REGISTRY=false
set REGISTRY_URL=

REM Color codes (limited CMD support)
set INFO=[INFO]
set SUCCESS=[SUCCESS]
set WARNING=[WARNING]
set ERROR=[ERROR]
set STEP=[STEP]

REM Functions
:show_info
echo %INFO% %~1
goto :eof

:show_success
echo %SUCCESS% %~1
goto :eof

:show_warning
echo %WARNING% %~1
goto :eof

:show_error
echo %ERROR% %~1
goto :eof

:show_step
echo %STEP% %~1
goto :eof

:show_help
echo.
echo QNAS Build Script - Filesystem Scanner for QNAP NAS (Windows Version)
echo.
echo USAGE:
echo     build-qnap.cmd [OPTIONS]
echo.
echo OPTIONS:
echo     -h, --help              Show this help message
echo     -t, --type TYPE         Build type: 'debug' or 'release' ^(default: release^)
echo     -c, --clean             Clean build ^(remove old artifacts^)
echo     -v, --verbose           Verbose output
echo     --skip-tests            Skip running tests
echo     --no-package            Skip creating deployment package
echo     --push                  Push to container registry
echo     --registry URL          Container registry URL
echo     --go-version VERSION    Go version to use ^(default: 1.23.3^)
echo     --version VERSION       Build version ^(default: 2.0-optimized^)
echo.
echo EXAMPLES:
echo     build-qnap.cmd                                    - Standard release build
echo     build-qnap.cmd --type debug --verbose            - Debug build with verbose
echo     build-qnap.cmd --clean --skip-tests              - Clean build without tests
echo     build-qnap.cmd --push --registry my-registry.com  - Build and push to registry
echo.
goto :eof

:validate_requirements
call :show_step "Validating build requirements..."

REM Check if Docker is available
docker --version >nul 2>&1
if !errorlevel! neq 0 (
    call :show_error "Docker is required but not installed"
    exit /b 1
)

REM Check Docker daemon is running
docker info >nul 2>&1
if !errorlevel! neq 0 (
    call :show_error "Docker daemon is not running"
    exit /b 1
)

REM Check if we're in the right directory
if not exist "go.mod" (
    call :show_error "Please run this script from the project root directory"
    exit /b 1
)

if not exist "Dockerfile.qnap" (
    call :show_error "Dockerfile.qnap not found in current directory"
    exit /b 1
)

REM Validate build type
if not "%BUILD_TYPE%"=="debug" if not "%BUILD_TYPE%"=="release" (
    call :show_error "Build type must be 'debug' or 'release'"
    exit /b 1
)

call :show_success "Requirements validation passed"
goto :eof

:clean_build
if "%CLEAN_BUILD%"=="true" (
    call :show_step "Cleaning old build artifacts..."

    REM Remove Docker images
    docker images | findstr /C:"%DOCKER_IMAGE%" >nul
    if !errorlevel! equ 0 (
        call :show_info "Removing old Docker images..."
        for /f "tokens=3" %%i in ('docker images %DOCKER_IMAGE% --format "{{.ID}}"') do (
            docker rmi -f %%i >nul 2>&1
        )
    )

    REM Remove build directory
    if exist "%OUTPUT_DIR%" (
        call :show_info "Removing build directory: %OUTPUT_DIR%"
        rmdir /s /q "%OUTPUT_DIR%"
    )

    REM Remove package files
    for %%f in (qnap-scanner-optimized.tar.gz %PACKAGE_NAME%.tar.gz) do (
        if exist "%%f" del /f "%%f" >nul 2>&1
    )

    call :show_success "Clean completed"
)
goto :eof

:run_tests
if "%SKIP_TESTS%"=="false" (
    call :show_step "Running tests..."

    REM Check if Go is available locally
    go version >nul 2>&1
    if !errorlevel! equ 0 (
        go test ./...
        if !errorlevel! equ 0 (
            call :show_success "All tests passed"
        ) else (
            call :show_error "Tests failed"
            exit /b 1
        )
    ) else (
        call :show_warning "Go not found locally, skipping tests"
    )
) else (
    call :show_warning "Skipping tests as requested"
)
goto :eof

:build_docker_image
call :show_step "Building QNAS Docker image..."

set BUILD_ARGS=--build-arg GO_VERSION=%GO_VERSION% --build-arg TARGETARCH=amd64 --build-arg BUILD_DATE="%BUILD_DATE%" -t %DOCKER_IMAGE%:latest -t %DOCKER_IMAGE%:%VERSION% -f Dockerfile.qnap

if "%BUILD_TYPE%"=="debug" (
    set BUILD_ARGS=%BUILD_ARGS% --target qnap-builder
)

if "%VERBOSE%"=="true" (
    set BUILD_ARGS=%BUILD_ARGS% --progress=plain
)

call :show_info "Building with arguments: %BUILD_ARGS%"

docker build %BUILD_ARGS% .
if !errorlevel! equ 0 (
    call :show_success "Docker image built successfully"
) else (
    call :show_error "Docker image build failed"
    exit /b 1
)
goto :eof

:extract_binaries
call :show_step "Extracting binaries from Docker image..."

if not exist "%OUTPUT_DIR%" mkdir "%OUTPUT_DIR%"

set CONTAINER_NAME=qnap-extract-%RANDOM%

REM Create temporary container
docker create --name %CONTAINER_NAME% %DOCKER_IMAGE%:latest >nul
if !errorlevel! equ 0 (
    call :show_info "Created temporary container: %CONTAINER_NAME%"
) else (
    call :show_error "Failed to create temporary container"
    exit /b 1
)

REM Extract binaries
for %%b in (scanner deleter reporter reporter_opt) do (
    call :show_info "Extracting %%b..."
    docker cp %CONTAINER_NAME%:/%%b "%OUTPUT_DIR%/%%b" >nul 2>&1
    if exist "%OUTPUT_DIR%/%%b" (
        call :show_success "Extracted %%b"
    ) else (
        call :show_warning "Failed to extract %%b (may not exist)"
    )
)

REM Extract deployment package if requested
if "%CREATE_PACKAGE%"=="true" (
    call :show_info "Extracting deployment package..."
    docker cp %CONTAINER_NAME%:/qnap-scanner-optimized.tar.gz . >nul 2>&1
    if exist "qnap-scanner-optimized.tar.gz" (
        call :show_success "Extracted deployment package"
    ) else (
        call :show_warning "Failed to extract deployment package"
    )
)

REM Cleanup container
docker rm %CONTAINER_NAME% >nul 2>&1

call :show_success "Binary extraction completed"
goto :eof

:verify_binaries
call :show_step "Verifying extracted binaries..."

set ALL_VALID=true

for %%b in (scanner deleter reporter reporter_opt) do (
    set BINARY_PATH=%OUTPUT_DIR%\%%b

    if exist "!BINARY_PATH!" (
        REM Check if file has content
        for %%F in ("!BINARY_PATH!") do set SIZE=%%~zF
        if !SIZE! gtr 0 (
            call :show_success "%%b: ✓ Valid executable (!SIZE! bytes)"
        ) else (
            call :show_error "%%b: ✗ Empty file"
            set ALL_VALID=false
        )
    ) else (
        call :show_warning "%%b: Not found"
    )
)

if "%ALL_VALID%"=="true" (
    call :show_success "All binaries verified successfully"
) else (
    call :show_error "Binary verification failed"
    exit /b 1
)
goto :eof

:create_deployment_package
if "%CREATE_PACKAGE%"=="false" (
    call :show_warning "Skipping deployment package creation"
    goto :eof
)

call :show_step "Creating QNAS deployment package..."

set PACKAGE_DIR=%OUTPUT_DIR%\%PACKAGE_NAME%
if not exist "%PACKAGE_DIR%" mkdir "%PACKAGE_DIR%"

REM Copy binaries
xcopy "%OUTPUT_DIR%\*.exe" "%PACKAGE_DIR%\" /q >nul 2>&1
xcopy "%OUTPUT_DIR%\scanner*" "%PACKAGE_DIR%\" /q >nul 2>&1

REM Copy configuration if exists
if exist "config.ini" (
    copy "config.ini" "%PACKAGE_DIR%\config.ini.example" >nul
)

REM Create deployment script
(
echo @echo off
echo echo 🚀 Deploying Filesystem Scanner to QNAP NAS...
echo.
echo REM Detect QNAP paths
echo if exist "C:\share\CACHEDEV1_DATA\.qpkg" ^(
echo     set QPKG_DIR=C:\share\CACHEDEV1_DATA\.qpkg\scanner
echo ^) else if exist "\\share\CACHEDEV1_DATA\.qpkg" ^(
echo     set QPKG_DIR=\\share\CACHEDEV1_DATA\.qpkg\scanner
echo ^) else ^(
echo     set QPKG_DIR=%%TEMP%%\scanner
echo     mkdir "%%QPKG_DIR%%"
echo ^)
echo.
echo REM Create directories
echo if not exist "%%QPKG_DIR%%\bin" mkdir "%%QPKG_DIR%%\bin"
echo if not exist "%%QPKG_DIR%%\config" mkdir "%%QPKG_DIR%%\config"
echo if not exist "%%QPKG_DIR%%\data" mkdir "%%QPKG_DIR%%\data"
echo if not exist "%%QPKG_DIR%%\logs" mkdir "%%QPKG_DIR%%\logs"
echo if not exist "%%QPKG_DIR%%\output" mkdir "%%QPKG_DIR%%\output"
echo.
echo REM Copy binaries
echo copy scanner* "%%QPKG_DIR%%\bin\" ^>nul
echo.
echo REM Copy configuration
echo if not exist "%%QPKG_DIR%%\config\config.ini" ^(
echo     copy config.ini.example "%%QPKG_DIR%%\config\config.ini" ^>nul
echo     echo 📝 Created default configuration
echo ^)
echo.
echo echo ✅ Deployment complete!
echo echo 📍 Installation: %%QPKG_DIR%%
echo echo 📖 Configuration: %%QPKG_DIR%%\config\config.ini
echo echo 🏃 Run: %%QPKG_DIR%%\bin\scanner.exe
) > "%PACKAGE_DIR%\deploy.bat"

REM Create README
(
echo # Filesystem Scanner for QNAP NAS
echo.
echo ## Version: %VERSION%
echo ## Build Date: %BUILD_DATE%
echo.
echo ## Quick Start ^(Windows Build^)
echo.
echo 1. Copy this package to your QNAP NAS
echo 2. Run: `deploy.bat`
echo 3. Edit: `config.ini` to set your scan paths
echo 4. Run: `bin\scanner.exe`
echo.
echo ## Files
echo.
echo - `scanner.exe` - Main filesystem scanner
echo - `deleter.exe` - Database cleanup tool
echo - `reporter.exe` - Basic report generator
echo - `reporter_opt.exe` - Optimized report generator
echo - `config.ini.example` - Example configuration
echo.
echo ## Support
echo.
echo For detailed documentation, please refer to the project repository.
) > "%PACKAGE_DIR%\README.md"

REM Create tar.gz package using tar (if available) or zip as fallback
tar --version >nul 2>&1
if !errorlevel! equ 0 (
    tar -czf "%OUTPUT_DIR%\%PACKAGE_NAME%.tar.gz" -C "%OUTPUT_DIR%" "%PACKAGE_NAME%"
    call :show_success "Created deployment package: %OUTPUT_DIR%\%PACKAGE_NAME%.tar.gz"
) else (
    REM Fallback to zip
    powershell -command "Compress-Archive -Path '%PACKAGE_DIR%' -DestinationPath '%OUTPUT_DIR%\%PACKAGE_NAME%.zip'"
    call :show_success "Created deployment package: %OUTPUT_DIR%\%PACKAGE_NAME%.zip"
)
goto :eof

:push_to_registry
if "%PUSH_TO_REGISTRY%"=="false" (
    goto :eof
)

if "%REGISTRY_URL%"=="" (
    call :show_error "Registry URL required for pushing"
    exit /b 1
)

call :show_step "Pushing to container registry..."

set FULL_IMAGE=%REGISTRY_URL%/%DOCKER_IMAGE%:%VERSION%

docker tag %DOCKER_IMAGE%:%VERSION% %FULL_IMAGE%
if !errorlevel! equ 0 (
    docker push %FULL_IMAGE%
    if !errorlevel! equ 0 (
        call :show_success "Pushed to registry: %FULL_IMAGE%"
    ) else (
        call :show_error "Failed to push to registry"
        exit /b 1
    )
) else (
    call :show_error "Failed to tag image for registry"
    exit /b 1
)
goto :eof

:show_summary
call :show_step "Build Summary"

echo ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo Project:        %PROJECT_NAME%
echo Version:        %VERSION%
echo Build Type:     %BUILD_TYPE%
echo Go Version:     %GO_VERSION%
echo Build Date:     %BUILD_DATE%
echo Output Directory: %OUTPUT_DIR%
echo.

if exist "%OUTPUT_DIR%" (
    echo Generated Files:
    dir /b "%OUTPUT_DIR%" 2>nul | findstr /v /C:""
)

echo ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

call :show_success "QNAS build completed successfully! 🎉"
goto :eof

:parse_args
set PARSE_DONE=false
:parse_loop
if "%PARSE_DONE%"=="true" goto :eof

if "%1"=="" goto parse_done

if "%1"=="-h" goto show_help
if "%1"=="--help" goto show_help

if "%1"=="-t" (
    set BUILD_TYPE=%2
    shift
    shift
    goto parse_loop
)
if "%1"=="--type" (
    set BUILD_TYPE=%2
    shift
    shift
    goto parse_loop
)

if "%1"=="-c" (
    set CLEAN_BUILD=true
    shift
    goto parse_loop
)
if "%1"=="--clean" (
    set CLEAN_BUILD=true
    shift
    goto parse_loop
)

if "%1"=="-v" (
    set VERBOSE=true
    shift
    goto parse_loop
)
if "%1"=="--verbose" (
    set VERBOSE=true
    shift
    goto parse_loop
)

if "%1"=="--skip-tests" (
    set SKIP_TESTS=true
    shift
    goto parse_loop
)

if "%1"=="--no-package" (
    set CREATE_PACKAGE=false
    shift
    goto parse_loop
)

if "%1"=="--push" (
    set PUSH_TO_REGISTRY=true
    shift
    goto parse_loop
)

if "%1"=="--registry" (
    set REGISTRY_URL=%2
    shift
    shift
    goto parse_loop
)

if "%1"=="--go-version" (
    set GO_VERSION=%2
    shift
    shift
    goto parse_loop
)

if "%1"=="--version" (
    set VERSION=%2
    shift
    shift
    goto parse_loop
)

call :show_error "Unknown option: %1"
goto show_help

:parse_done
set PARSE_DONE=true
goto :eof

:main
echo.
echo ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo 🚀 QNAS BUILD SCRIPT - Filesystem Scanner for QNAP NAS ^(Windows^)
echo ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo.

call :parse_args %*

call :validate_requirements
if !errorlevel! neq 0 exit /b 1

call :clean_build
if !errorlevel! neq 0 exit /b 1

call :run_tests
if !errorlevel! neq 0 exit /b 1

call :build_docker_image
if !errorlevel! neq 0 exit /b 1

call :extract_binaries
if !errorlevel! neq 0 exit /b 1

call :verify_binaries
if !errorlevel! neq 0 exit /b 1

call :create_deployment_package
if !errorlevel! neq 0 exit /b 1

call :push_to_registry
if !errorlevel! neq 0 exit /b 1

call :show_summary
goto :eof

REM Run main function with all arguments
call :main %*