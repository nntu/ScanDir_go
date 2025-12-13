@echo off
REM =============================================================================
# WINDOWS MAKEFILE SIMULATOR - Filesystem Scanner Project
# =============================================================================
# Author: Claude Code Assistant
# Description: Windows batch file alternative to Makefile
# Usage: make.bat [target] [options]
# =============================================================================

setlocal enabledelayedexpansion

REM Configuration
set SCRIPT_DIR=%~dp0
set SCANNER_BIN=scanner
set DELETER_BIN=deleter
set REPORTER_BIN=reporter
set REPORTER_OPT_BIN=reporter_opt
set CHECKDUP_BIN=checkdup

REM Default target
if "%1"=="" set TARGET=all

REM Parse target
set TARGET=%1
shift

REM Colors for output (Windows CMD limited support)
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
echo Windows Makefile Simulator - Filesystem Scanner Project
echo.
echo USAGE:
echo     make.bat [TARGET] [OPTIONS]
echo.
echo TARGETS:
echo     all                  Build all executables ^(default^)
echo     build-local          Build optimized local binaries
echo     build-release        Build release binaries with static linking
echo     test                 Run tests
echo     clean                Clean build artifacts
echo     deps                 Install dependencies
echo     deps-optimize        Optimize dependencies
echo     verify               Verify binaries
echo     info                 Show build information
echo.
echo     QNAS TARGETS:
echo     qnap                 Build QNAS optimized version
echo     qnap-verbose         Build QNAS with verbose output
echo     qnap-clean           Clean QNAS build artifacts
echo     qnap-test            Build and test QNAS binaries
echo     qnap-package         Create QNAS deployment package
echo     qnap-info            Show QNAS build information
echo     help-qnap            Show QNAS build help
echo.
echo EXAMPLES:
echo     make.bat all                    - Build all binaries
echo     make.bat build-local            - Build local optimized binaries
echo     make.bat build-release           - Build release binaries
echo     make.bat test                   - Run tests
echo     make.bat clean                  - Clean all artifacts
echo     make.bat qnap                   - Build QNAS version
echo     make.bat qnap-verbose           - QNAS build with verbose output
echo.
goto :eof

:check_go
go version >nul 2>&1
if !errorlevel! neq 0 (
    echo Go is not installed or not in PATH
    echo Please install Go from https://golang.org/dl/
    exit /b 1
)
goto :eof

:build_local
call :show_step "Building optimized local binaries..."

call :check_go

echo Building scanner...
go build -tags scanner -trimpath -ldflags="-s -w" -o %SCANNER_BIN% .
if !errorlevel! neq 0 (
    call :show_error "Failed to build scanner"
    exit /b 1
)

echo Building deleter...
go build -tags deleter -trimpath -ldflags="-s -w" -o %DELETER_BIN% .
if !errorlevel! neq 0 (
    call :show_error "Failed to build deleter"
    exit /b 1
)

echo Building reporter...
go build -tags reporter -trimpath -ldflags="-s -w" -o %REPORTER_BIN% .
if !errorlevel! neq 0 (
    call :show_error "Failed to build reporter"
    exit /b 1
)

echo Building checkdup...
go build -tags checkdup -trimpath -ldflags="-s -w" -o %CHECKDUP_BIN% .
if !errorlevel! neq 0 (
    call :show_error "Failed to build checkdup"
    exit /b 1
)

echo Building optimized reporter...
go build -tags reporter_optimized -trimpath -ldflags="-s -w" -o %REPORTER_OPT_BIN% .
if !errorlevel! neq 0 (
    call :show_warning "Failed to build optimized reporter (may not be needed)"
)

call :show_success "Local build complete!"
goto :eof

:build_release
call :show_step "Building release binaries with maximum optimizations..."

call :check_go

echo Building scanner ^(release^)...
go build -tags scanner -trimpath -ldflags="-s -w -extldflags '-static'" -o %SCANNER_BIN%_release .
if !errorlevel! neq 0 (
    call :show_error "Failed to build scanner (release)"
    exit /b 1
)

echo Building deleter ^(release^)...
go build -tags deleter -trimpath -ldflags="-s -w -extldflags '-static'" -o %DELETER_BIN%_release .
if !errorlevel! neq 0 (
    call :show_error "Failed to build deleter (release)"
    exit /b 1
)

echo Building reporter ^(release^)...
go build -tags reporter -trimpath -ldflags="-s -w -extldflags '-static'" -o %REPORTER_BIN%_release .
if !errorlevel! neq 0 (
    call :show_error "Failed to build reporter (release)"
    exit /b 1
)

echo Building optimized reporter ^(release^)...
go build -tags reporter -trimpath -ldflags="-s -w -extldflags '-static'" -o %REPORTER_OPT_BIN%_release report_optimized.go
if !errorlevel! neq 0 (
    call :show_warning "Failed to build optimized reporter (release)"
)

call :show_success "Release build complete!"
goto :eof

:test
call :show_step "Running tests..."
call :check_go

go test -v ./...
if !errorlevel! equ 0 (
    call :show_success "All tests passed!"
) else (
    call :show_error "Tests failed!"
    exit /b 1
)
goto :eof

:clean
call :show_step "Cleaning build artifacts..."

for %%f in (%SCANNER_BIN% %DELETER_BIN% %REPORTER_BIN% %REPORTER_OPT_BIN%) do (
    if exist "%%f" (
        del /f "%%f" >nul 2>&1
        if !errorlevel! equ 0 (
            echo Deleted %%f
        )
    )
    if exist "%%f_release" (
        del /f "%%f_release" >nul 2>&1
        if !errorlevel! equ 0 (
            echo Deleted %%f_release
        )
    )
)

REM Clean QNAS artifacts
for %%f in (qnap-scanner-*.tar.gz qnap-scanner-*.zip) do (
    if exist "%%f" del /f "%%f" >nul 2>&1
)

REM Clean directories
for %%d in (qnap-build qnap-output qnap-packages) do (
    if exist "%%d" (
        rmdir /s /q "%%d" >nul 2>&1
        if !errorlevel! equ 0 (
            echo Removed directory %%d
        )
    )
)

REM Clean Go cache
go clean -cache 2>nul

call :show_success "Cleanup complete!"
goto :eof

:deps
call :show_step "Installing dependencies..."
call :check_go

go mod download
if !errorlevel! neq 0 (
    call :show_error "Failed to download dependencies"
    exit /b 1
)

go mod tidy
if !errorlevel! neq 0 (
    call :show_error "Failed to tidy dependencies"
    exit /b 1
)

call :show_success "Dependencies installed successfully!"
goto :eof

:deps_optimize
call :show_step "Optimizing dependencies..."
call :check_go

go mod tidy -go=1.24
if !errorlevel! neq 0 (
    call :show_error "Failed to optimize dependencies"
    exit /b 1
)

go mod verify
if !errorlevel! neq 0 (
    call :show_error "Failed to verify dependencies"
    exit /b 1
)

call :show_success "Dependencies optimized successfully!"
goto :eof

:verify
call :show_step "Verifying binaries..."

call :build_local

echo Verifying scanner binary...
if exist "%SCANNER_BIN%" (
    %SCANNER_BIN% --help >nul 2>&1
    if !errorlevel! equ 0 (
        echo ✅ Scanner binary OK
    ) else (
        echo ❌ Scanner binary verification failed
    )
) else (
    echo ❌ Scanner binary not found
)

echo Verifying deleter binary...
if exist "%DELETER_BIN%" (
    %DELETER_BIN% --help >nul 2>&1
    if !errorlevel! equ 0 (
        echo ✅ Deleter binary OK
    ) else (
        echo ❌ Deleter binary verification failed
    )
) else (
    echo ❌ Deleter binary not found
)

echo Verifying reporter binary...
if exist "%REPORTER_BIN%" (
    %REPORTER_BIN% --help >nul 2>&1
    if !errorlevel! equ 0 (
        echo ✅ Reporter binary OK
    ) else (
        echo ❌ Reporter binary verification failed
    )
) else (
    echo ❌ Reporter binary not found
)

echo Verifying optimized reporter binary...
if exist "%REPORTER_OPT_BIN%" (
    %REPORTER_OPT_BIN% --help >nul 2>&1
    if !errorlevel! equ 0 (
        echo ✅ Optimized reporter binary OK
    ) else (
        echo ❌ Optimized reporter binary verification failed
    )
) else (
    echo ❌ Optimized reporter binary not found
)

call :show_success "Binary verification complete!"
goto :eof

:info
call :show_step "Build Information"

echo Build Information:
for /f "tokens=3" %%v in ('go version') do echo   Go Version: %%v
echo   OS/Arch: Windows %PROCESSOR_ARCHITECTURE%
if exist go.mod (
    for /f "tokens=2" %%m in ('findstr /C:"module" go.mod') do echo   Module: %%m
)

REM Get git info if available
git rev-parse --short HEAD >nul 2>&1
if !errorlevel! equ 0 (
    for /f %%g in ('git rev-parse --short HEAD') do echo   Git Commit: %%g
) else (
    echo   Git Commit: N/A
)

echo   Build Time: %date% %time%
goto :eof

REM QNAS Targets
:qnap
call :show_step "Building QNAS optimized version..."
call build-qnap.cmd
goto :eof

:qnap-verbose
call :show_step "Building QNAS optimized version (verbose)..."
call build-qnap.cmd --verbose
goto :eof

:qnap-clean
call :show_step "Cleaning QNAS build artifacts..."
call build-qnap.cmd --clean
goto :eof

:qnap-test
call :show_step "Building and testing QNAS binaries..."
call build-qnap.cmd --skip-tests
if exist qnap-build (
    cd qnap-build
    if exist scanner.exe (
        scanner.exe --help >nul 2>&1
        if !errorlevel! equ 0 echo ✅ Scanner binary OK else echo ❌ Scanner binary failed
    )
    if exist deleter.exe (
        deleter.exe --help >nul 2>&1
        if !errorlevel! equ 0 echo ✅ Deleter binary OK else echo ❌ Deleter binary failed
    )
    if exist reporter.exe (
        reporter.exe --help >nul 2>&1
        if !errorlevel! equ 0 echo ✅ Reporter binary OK else echo ❌ Reporter binary failed
    )
    cd ..
)
goto :eof

:qnap-package
call :show_step "Creating QNAS deployment package..."
call build-qnap.cmd --clean --verbose
goto :eof

:qnap-info
call :show_step "QNAS Build Information"
echo QNAS Build Information:
echo   Target Platform: QNAP NAS ^(x86_64/ARM64^)
echo   Compatible: QTS 4.x+, QuTS hero
echo   Build Tools: Docker + Multi-stage builds
echo   Output: Static binaries ^(no dependencies^)
echo.
echo Quick Commands:
echo   make.bat qnap         - Build QNAS version
echo   make.bat qnap-verbose - Verbose build
echo   make.bat qnap-test    - Build and test
echo   make.bat qnap-clean   - Clean artifacts
goto :eof

:help-qnap
call :show_step "QNAS Build Targets Help"
echo.
echo QNAS Build Targets:
echo.
echo Build Commands:
echo   qnap              - Build QNAS version ^(AMD64^)
echo   qnap-verbose      - Build with verbose output
echo   qnap-version      - Build with specific version
echo   qnap-amd64        - Build AMD64 only
echo   qnap-arm64        - Build ARM64 binaries
echo   qnap-compose      - Multi-architecture build
echo   qnap-all          - Build all architectures
echo.
echo Testing ^& Quality:
echo   qnap-test         - Build and test binaries
echo   qnap-deploy-test  - Test deployment process
echo.
echo Package ^& Deploy:
echo   qnap-package      - Create deployment package
echo   qnap-push         - Build and push to registry
echo.
echo Maintenance:
echo   qnap-clean        - Clean build artifacts
echo   qnap-info         - Show QNAS build info
echo   help-qnap         - Show this help
echo.
echo Examples:
echo   make.bat qnap VERSION=1.0.0
echo   make.bat qnap-push REGISTRY=registry.example.com
echo   make.bat qnap-all VERSION=latest
echo.
goto :eof

:help
call :show_help
goto :eof

:all
call :show_step "Building all executables..."
call :build_local
goto :eof

REM Invalid target
:invalid_target
call :show_error "Unknown target: %TARGET%"
echo.
call :show_help
exit /b 1

REM Main execution logic
if "%TARGET%"=="all" goto all
if "%TARGET%"=="build-local" goto build_local
if "%TARGET%"=="build-release" goto build_release
if "%TARGET%"=="test" goto test
if "%TARGET%"=="clean" goto clean
if "%TARGET%"=="deps" goto deps
if "%TARGET%"=="deps-optimize" goto deps_optimize
if "%TARGET%"=="verify" goto verify
if "%TARGET%"=="info" goto info
if "%TARGET%"=="qnap" goto qnap
if "%TARGET%"=="qnap-verbose" goto qnap-verbose
if "%TARGET%"=="qnap-clean" goto qnap-clean
if "%TARGET%"=="qnap-test" goto qnap-test
if "%TARGET%"=="qnap-package" goto qnap-package
if "%TARGET%"=="qnap-info" goto qnap-info
if "%TARGET%"=="help-qnap" goto help_qnap
if "%TARGET%"=="help" goto help
if "%TARGET%"=="-h" goto help
if "%TARGET%"=="--help" goto help

goto invalid_target