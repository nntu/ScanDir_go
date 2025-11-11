# Filesystem Scanner for QNAP NAS - Deployment Guide

## 📋 Tổng quan

Filesystem Scanner phiên bản QNAS được tối ưu hóa đặc biệt cho các hệ thống QNAP NAS, hỗ trợ cả kiến trúc x86_64 (Intel/AMD) và ARM64. Sản phẩm được xây dựng với Docker để đảm bảo tính tương thích và hiệu suất tối ưu.

## 🎯 Tính năng chính

- **✅ Tối ưu hóa cho QNAP**: Compatible với QTS 4.x+ và QuTS hero
- **✅ Multi-architecture**: Hỗ trợ Intel/AMD và ARM-based QNAP NAS
- **✅ Static binaries**: Không yêu cầu additional dependencies
- **✅ Performance optimized**: Dynamic batching, memory-aware worker pools
- **✅ Multiple output formats**: Excel, HTML, JSON, Console reports
- **✅ Safe deletion**: Dry-run mode và path validation
- **✅ Structured logging**: Detailed metrics và performance tracking

## 🏗️ Build Options

### Option 1: Quick Build (Recommended)
```bash
# Build cho QNAP x86_64 (phổ biến nhất)
./build-qnap.sh

# Build với verbose output
./build-qnap.sh --verbose

# Build version cụ thể
./build-qnap.sh --version 1.0.0-qnap
```

### Option 2: Multi-Architecture Build
```bash
# Build cả x86_64 và ARM64
./build-qnap-compose.sh --arm64

# Build và push tới registry
./build-qnap-compose.sh --arm64 --push --registry your-registry.com
```

### Option 3: Development Build
```bash
# Build debug version
./build-qnap.sh --type debug --verbose

# Build skip tests
./build-qnap.sh --skip-tests
```

## 📦 Các file được tạo

Sau khi build, bạn sẽ có:

### Binary files:
- `scanner` - Main filesystem scanning tool
- `deleter` - Database cleanup tool (safe deletion only)
- `reporter` - Basic report generator
- `reporter_opt` - Optimized report generator với caching

### Deployment packages:
- `qnap-scanner-{version}.tar.gz` - Complete deployment package
- `qnap-scanner-{version}-amd64.tar.gz` - AMD64 specific package
- `qnap-scanner-{version}-arm64.tar.gz` - ARM64 specific package

## 🚀 Deployment trên QNAP NAS

### Phương pháp 1: Automatic Deployment Script

1. **Copy package to QNAP**:
```bash
scp qnap-scanner-2.0-qnap.tar.gz admin@your-nas-ip:/share/Public/
```

2. **SSH vào QNAP**:
```bash
ssh admin@your-nas-ip
cd /share/Public/
```

3. **Extract và deploy**:
```bash
tar -xzf qnap-scanner-2.0-qnap.tar.gz
cd qnap-scanner-2.0-qnap/
./deploy.sh
```

### Phương pháp 2: Manual Installation

1. **Create directories**:
```bash
# Detect QNAP storage paths
QPKG_DIR="/share/CACHEDEV1_DATA/.qpkg/scanner"
mkdir -p "$QPKG_DIR"/{bin,config,data,logs,output}
```

2. **Copy binaries**:
```bash
cp scanner deleter reporter reporter_opt "$QPKG_DIR/bin/"
chmod +x "$QPKG_DIR/bin/"*
```

3. **Create configuration**:
```bash
cp config.ini.example "$QPKG_DIR/config/config.ini"
```

4. **Edit configuration**:
```bash
vi "$QPKG_DIR/config/config.ini"
```

## ⚙️ Configuration cho QNAP

Edit file `config.ini` với paths phù hợp cho QNAP:

```ini
[output]
; Sử dụng path trong QPKG directory
output_dir = /share/CACHEDEV1_DATA/.qpkg/scanner/output

[scan]
; QNAP thường có 4-8 cores
BATCH_SIZE = 3000
MAX_WORKERS = 6
EXCLUDE_DIRS = .git,.streams,@Recently-Snapshot,@Recycle,COREBanking,@get,@eaDir

[paths]
; Các shared folders trên QNAP
root1 = /share/CACHEDEV1_DATA/Public:Public
root2 = /share/CACHEDEV1_DATA/Multimedia:Multimedia
root3 = /share/CACHEDEV1_DATA/Downloads:Downloads
root4 = /share/CACHEDEV1_DATA/Home:Home
```

## 🏃 Sử dụng trên QNAP

### Basic Usage
```bash
# Change to scanner directory
cd /share/CACHEDEV1_DATA/.qpkg/scanner

# Run filesystem scan
./bin/scanner

# Check logs
tail -f logs/scanner.log

# Generate Excel report
./bin/reporter -dbfile output/scan_*.db -format excel -output reports/duplicate_files.xlsx
```

### Advanced Usage
```bash
# Generate detailed report với caching
./bin/reporter_opt -dbfile output/scan_20241210_120000.db -format json -output reports/detailed_report.json --cache --verbose

# Safe deletion với dry-run
./bin/deleter -dbfile output/scan_20241210_120000.db -path "/share/CACHEDEV1_DATA/Public/old_folder" --dry-run --verbose

# Delete from database (không xóa files thật)
./bin/deleter -dbfile output/scan_20241210_120000.db -path "/share/CACHEDEV1_DATA/Public/temp_files"
```

## 📊 Performance Optimization cho QNAP

### Memory Management
```ini
# Quản lý memory cho QNAP 4GB RAM
[scan]
BATCH_SIZE = 2000  ; Giảm nếu memory thấp
MAX_WORKERS = 4    ; Tăng nếu CPU mạnh
```

### Storage Optimization
```ini
[output]
output_dir = /share/CACHEDEV1_DATA/.qpkg/scanner/output
; Sử dụng SSD cache nếu có
```

### Network Share Scanning
```ini
[paths]
# Include network shares
root1 = /share/CACHEDEV1_DATA/Public:Public
root2 = /share/external/DEV1_1/backup:ExternalBackup
```

## 📈 Performance Metrics

Với QNAP TS-x71 series (Intel Celeron J3455, 8GB RAM):

- **100K files**: ~15-20 minutes
- **1M files**: ~2-3 hours
- **Memory usage**: ~200-500MB
- **CPU usage**: 60-80% during hashing

## 🛠️ Troubleshooting

### Common Issues

1. **Permission denied**:
```bash
chmod +x /share/CACHEDEV1_DATA/.qpkg/scanner/bin/*
```

2. **SQLite lock error**:
```bash
# Ensure only one instance running
ps aux | grep scanner
killall scanner
```

3. **Memory issues**:
```ini
# Reduce batch size and workers
BATCH_SIZE = 1000
MAX_WORKERS = 2
```

4. **Network share access**:
```bash
# Check mount status
mount | grep /share
df -h
```

### Logging và Debugging

```bash
# Enable verbose logging
./bin/scanner 2>&1 | tee logs/debug.log

# Check system resources
top
htop
iostat -x 1
```

## 🔄 Scheduled Scanning

Tạo cron job trên QNAP:

```bash
# Edit crontab
vi /etc/config/crontab

# Add daily scan at 2 AM
0 2 * * * admin /share/CACHEDEV1_DATA/.qpkg/scanner/bin/scanner >> /share/CACHEDEV1_DATA/.qpkg/scanner/logs/daily_scan.log 2>&1

# Restart cron
/etc/init.d/crond.sh restart
```

## 📱 QNAP App Integration

Tạo QPKG package cho integration với QNAP App Center:

1. **Tạo QPKG config**:
```bash
mkdir -p qnap-scanner/qpkgcfg
```

2. **Tạo file qpkg.cfg**:
```ini
[QPKG]
Name = Filesystem Scanner
Version = 2.0
Author = Claude Code Assistant
QPKG_File = qpkg.cfg
Date = 2024-12-10
Shell = /share/CACHEDEV1_DATA/.qpkg/scanner/scanner.sh
Install_Path = /share/CACHEDEV1_DATA/.qpkg/scanner
WebUI = /cgi-bin/index.html
Web_Port = 8080
Web_Path = /scanner
```

3. **Package creation**:
```bash
tar -czf qnap-scanner_2.0.qpkg qnap-scanner/
```

## 📞 Hỗ trợ

### Logs location:
- Application: `/share/CACHEDEV1_DATA/.qpkg/scanner/logs/`
- System: `/var/log/messages`
- Cron: `/var/log/cron.log`

### Configuration:
- Main config: `/share/CACHEDEV1_DATA/.qpkg/scanner/config/config.ini`
- Database: `/share/CACHEDEV1_DATA/.qpkg/scanner/output/`

### Performance monitoring:
```bash
# Monitor scanner process
top -p $(pgrep scanner)

# Check I/O usage
iostat -x 1

# Monitor memory
free -h
cat /proc/meminfo | grep MemAvailable
```

## 📄 License và Legal

- Open source với MIT license
- Không thu thập data cá nhân
- Chỉ scan filesystem metadata
- Database files được lưu local trên QNAP

---

**🎉 Chúc bạn sử dụng Filesystem Scanner trên QNAP thành công!**

Để được hỗ trợ thêm, vui lòng tham khảo project documentation hoặc tạo issue trên repository.