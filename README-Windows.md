# Filesystem Scanner for QNAP NAS - Windows Build Guide

## 🖥️ Windows Build System

This guide covers building QNAP-optimized filesystem scanner binaries on Windows environments.

## 📋 Prerequisites

### Required Software
- **Windows 10/11** or Windows Server 2016+
- **Docker Desktop for Windows** (latest version)
- **PowerShell 5.1+** (built-in on Windows 10+)
- **Git for Windows** (optional, for version control)

### Docker Setup
1. Install [Docker Desktop for Windows](https://www.docker.com/products/docker-desktop)
2. Start Docker Desktop
3. Ensure WSL 2 backend is enabled (recommended for performance)
4. Test installation: `docker --version` and `docker info`

### Windows Subsystem for Linux (Optional but Recommended)
```powershell
# Install WSL for better performance
wsl --install
wsl --set-default-version 2
```

## 🚀 Quick Start

### Option 1: Simple Windows CMD Build
```cmd
# Build QNAS version with default settings
build-qnap-windows.cmd

# Verbose build with detailed output
build-qnap-windows.cmd --verbose

# Clean build (remove old artifacts first)
build-qnap-windows.cmd --clean
```

### Option 2: PowerShell Advanced Build
```powershell
# Standard build
.\build-qnap.ps1

# Build with custom version
.\build-qnap.ps1 -Version "1.0.0-custom"

# Debug build with verbose output
.\build-qnap.ps1 -Type debug -Verbose

# Build without creating packages
.\build-qnap.ps1 -NoPackage -SkipTests
```

### Option 3: Makefile Simulator
```cmd
# Build all binaries (equivalent to make all)
make.bat

# Build QNAS optimized version
make.bat qnap

# Build with verbose output
make.bat qnap-verbose

# Clean and rebuild
make.bat qnap-clean
make.bat qnap

# Test binaries
make.bat qnap-test
```

## 📁 Build Scripts Overview

### Primary Build Scripts

| Script | Type | Features | Usage |
|--------|------|----------|-------|
| `build-qnap-windows.cmd` | CMD | Interactive menu, verbose output | `build-qnap-windows.cmd` |
| `build-qnap.ps1` | PowerShell | Advanced options, error handling | `.\build-qnap.ps1` |
| `build-qnap.cmd` | CMD | Basic build, 15+ options | `build-qnap.cmd --help` |
| `make.bat` | CMD | Makefile simulator | `make.bat qnap` |

### Script Comparison

| Feature | CMD Script | PowerShell Script |
|---------|------------|-------------------|
| Interactive Mode | ✅ | ❌ |
| Advanced Options | ⚠️ Basic | ✅ Comprehensive |
| Error Handling | ⚠️ Basic | ✅ Advanced |
| Progress Reporting | ✅ Colored output | ✅ Structured logging |
| Registry Push | ✅ | ✅ |
| Auto-completion | ❌ | ✅ PowerShell |

## 🛠️ Build Options Reference

### CMD Script Options
```cmd
build-qnap-windows.cmd [OPTIONS]

OPTIONS:
  -h, --help              Show help message
  -v, --verbose           Enable verbose output
  -c, --clean             Clean build artifacts
  -t, --type TYPE         Build type: debug/release
  --arch ARCH            Target architecture: amd64/arm64
  --version VERSION      Custom image version
  --no-package           Skip deployment package
  --skip-tests           Skip running tests
  --no-prompt           Skip interactive prompts
```

### PowerShell Script Options
```powershell
.\build-qnap.ps1 [OPTIONS]

PARAMETERS:
  -Version <string>       Build version (default: 2.0-optimized)
  -GoVersion <string>     Go version (default: 1.23.3)
  -Type <string>         Build type: debug/release (default: release)
  -Clean                 Clean build artifacts
  -Verbose               Verbose output
  -SkipTests             Skip running tests
  -NoPackage             Skip creating deployment package
  -Push                  Push images to registry
  -Registry <string>     Container registry URL
  -Help                  Show help message
```

### Makefile Simulator Options
```cmd
make.bat [TARGET]

QNAS TARGETS:
  qnap                 Build QNAS version
  qnap-verbose         Build with verbose output
  qnap-clean           Clean build artifacts
  qnap-test            Build and test binaries
  qnap-package         Create deployment package
  qnap-info            Show QNAS build info
  help-qnap            Show QNAS build help

GENERAL TARGETS:
  all                  Build all executables
  build-local          Build local binaries
  test                 Run tests
  clean                Clean artifacts
  verify               Verify binaries
```

## 📦 Build Outputs

### Directory Structure
```
ScanDir_go/
├── build-qnap-windows.cmd       # Main Windows build script
├── build-qnap.ps1               # PowerShell build script
├── build-qnap.cmd                # Basic CMD build script
├── make.bat                       # Makefile simulator
├── Dockerfile.qnap               # QNAP Dockerfile
├── go.mod                        # Go modules
├── qnap-build/                    # Build output directory
│   ├── scanner.exe               # Main scanner
│   ├── deleter.exe               # Database cleanup
│   ├── reporter.exe               # Basic reporter
│   ├── reporter_opt.exe          # Optimized reporter
│   └── qnap-scanner-optimized.tar.gz  # Deployment package
├── qnap-deploy/                  # Deployment files (if created)
│   ├── deploy-qnap.cmd           # Windows deployment helper
│   └── README.md                  # Deployment instructions
└── README-Windows.md            # This file
```

### Binary Files Generated
- **scanner.exe** - Main filesystem scanning tool
- **deleter.exe** - Database cleanup tool (safe deletion only)
- **reporter.exe** - Basic report generator
- **reporter_opt.exe** - Advanced report generator with caching

## 🐳 Docker Build Configuration

### Multi-Stage Build Process
1. **Builder Stage**: Alpine 3.19 with Go 1.23.3 and musl libc
2. **Runtime Stage**: Alpine-based minimal runtime
3. **Artifact Stage**: Binary extraction only
4. **Package Stage**: Deployment package creation with UPX compression

### Build Targets
```dockerfile
# Dockerfile.qnap build targets
--target qnap-builder      # Development build with tools
--target qnap-runtime     # Minimal runtime container
--target qnap-artifact    # Binary extraction only
--target qnap-package     # Deployment package creation
```

### Platform Support
- **AMD64**: Intel/AMD based QNAP NAS (primary target)
- **ARM64**: ARM-based QNAP NAS (secondary target)

## 🎯 Build Modes

### Interactive Mode (Default)
```cmd
build-qnap-windows.cmd
```
Features:
- Interactive menu system
- Option selection for container vs native binaries
- Progress indicators with colored output
- Automatic testing and verification

### Non-Interactive Mode
```cmd
# Build both container and native binaries
build-qnap-windows.cmd --no-prompt

# Build only native binaries
build-qnap-windows.cmd --no-prompt --mode native

# Build with custom options
build-qnap-windows.cmd --clean --verbose --version "1.0.0" --no-package
```

## 🔧 Advanced Configuration

### Custom Build Parameters
```cmd
# Custom Go version
build-qnap-windows.cmd --go-version "1.22.0"

# ARM64 architecture (for ARM-based QNAP)
build-qnap-windows.cmd --arch arm64

# Debug build with detailed logging
build-qnap-windows.cmd --type debug --verbose

# Custom version and registry push
build-qnap-windows.cmd --version "custom-1.0" --push --registry "myregistry.com"
```

### Environment Variables (Optional)
```cmd
# Set custom Go version
set GO_VERSION=1.23.3

# Set custom image name
set IMAGE_NAME=my-qnap-scanner

# Set custom output directory
set OUTPUT_DIR=custom-build

# Run build with custom environment
build-qnap-windows.cmd
```

## 📊 Performance Monitoring

### Build Time Benchmarks (Windows 10, Docker Desktop)
| Operation | Average Time | Description |
|-----------|-------------|-------------|
| Docker Image Build | 6-12 minutes | Alpine-based container build |
| Binary Extraction | 1-2 minutes | Extract from built image |
| UPX Compression | 30-45 seconds | Binary compression |
| Package Creation | 30 seconds | Create deployment package |
| Total Time | 8-15 minutes | Complete build process |

### Resource Usage
- **Docker Memory**: 1-3 GB during build (Alpine optimized)
- **Disk Space**: 1-3 GB for Docker layers (Alpine smaller footprint)
- **CPU Usage**: 60-80% during compilation
- **Final Package Size**: ~5-10 MB (UPX compressed vs ~30-50 MB CentOS)

## 🔍 Troubleshooting

### Common Windows Issues

#### Docker Desktop Issues
```cmd
# Check Docker status
docker --version
docker info

# Restart Docker Desktop
# Use system tray or services.msc

# Clear Docker cache
docker system prune -a
```

#### Permission Issues
```cmd
# Run PowerShell as Administrator
# Or use Developer Mode in Windows Settings

# Check file permissions
icacls . /grant Users:F /T
```

#### Network Issues
```cmd
# Check internet connectivity
ping google.com

# Check Docker registry access
docker pull alpine:latest

# Reset Docker network
docker network prune
```

### Build-Specific Issues

#### Build Failures
```cmd
# Enable verbose logging
build-qnap-windows.cmd --verbose

# Clean build
build-qnap-windows.cmd --clean

# Check Docker logs
docker logs <container-name>
```

#### Binary Extraction Issues
```cmd
# Manually extract from Docker image
docker create --name temp <image-name>
docker cp temp:/scanner ./scanner.exe
docker rm temp
```

## 📱 QNAP Deployment

### Windows to QNAP Transfer
```cmd
# Method 1: Network Share
copy qnap-build\*.exe \\192.168.1.100\Public\scanner\bin\

# Method 2: SCP (if SSH enabled)
scp qnap-build\*.exe admin@192.168.1.100:/share/Public/scanner/bin/

# Method 3: QNAP File Station
# 1. Open QNAP File Station
# 2. Upload files to Public share
# 3. Use SSH to execute
```

### QNAP Configuration
```ini
# config.ini for QNAP
[output]
output_dir = /share/CACHEDEV1_DATA/.qpkg/scanner/output

[scan]
BATCH_SIZE = 3000
MAX_WORKERS = 6
EXCLUDE_DIRS = .git,.streams,@Recently-Snapshot,@Recycle

[paths]
root1 = /share/CACHEDEV1_DATA/Public:Public
root2 = /share/CACHEDEV1_DATA/Multimedia:Multimedia
root3 = /share/CACHEDEV1_DATA/Home:Home
```

## 🔄 Automation

### Windows Task Scheduler
```cmd
# Create scheduled build task
schtasks /create /tn "QNAS Scanner Build" /tr "C:\Path\To\build-qnap-windows.cmd" /sc weekly /d SUN /st 02:00
```

### PowerShell Automation
```powershell
# Automated build script
$buildScript = "C:\Path\To\build-qnap-windows.cmd"
$logFile = "C:\Path\To\build.log"

try {
    & $buildScript --verbose --no-prompt 2>&1 | Out-File $logFile -Append
    Write-Host "Build completed successfully"
}
catch {
    Write-Host "Build failed: $_"
    # Send notification or email
}
```

### CI/CD Integration
```yaml
# GitHub Actions (Windows runner)
name: Build QNAS Scanner
on: [push, pull_request]
jobs:
  build-windows:
    runs-on: windows-latest
    steps:
    - uses: actions/checkout@v3
    - name: Setup Docker
      run: docker version
    - name: Build QNAS Scanner
      run: .\build-qnap-windows.cmd --verbose --no-prompt
    - name: Upload Artifacts
      uses: actions/upload-artifact@v3
      with:
        name: qnap-build-windows
        path: qnap-build/
```

## 📚 Additional Resources

### Documentation
- [README-QNAS.md](./README-QNAS.md) - QNAS-specific deployment guide
- [CLAUDE.md](./CLAUDE.md) - General development guide
- [Project Repository](https://github.com/your-repo/scandir) - Source code

### Tools and Utilities
- [Docker Desktop](https://www.docker.com/products/docker-desktop) - Container platform
- [Windows Terminal](https://github.com/microsoft/terminal) - Enhanced terminal
- [PowerShell Gallery](https://www.powershellgallery.com/) - Additional modules

### Community Support
- [Issues](https://github.com/your-repo/scandir/issues) - Bug reports and feature requests
- [Discussions](https://github.com/your-repo/scandir/discussions) - General questions
- [Wiki](https://github.com/your-repo/scandir/wiki) - Additional documentation

---

## 🎉 Summary

The Windows build system provides multiple options for building QNAP-optimized filesystem scanner:

1. **🖥️ User-Friendly**: Interactive CMD script for beginners
2. **⚡ Advanced**: PowerShell script for automation
3. **🔧 Compatible**: Makefile simulator for existing workflows
4. **🐳 Containerized**: Alpine-based Docker builds for consistency
5. **📦 Production Ready**: UPX-optimized binaries for QNAP deployment
6. **🏔️ Alpine Linux**: Smaller, more secure, zero-dependency binaries

Choose the build method that best fits your workflow and expertise level! 🚀