param(
    [string]$Version = "2.0.0-qnap",
    [string]$GoVersion = "1.23.3",
    [string]$Platform = "linux/amd64",
    [string]$OutDir = "./qnap-build",
    [string]$Progress = ""
)

$ErrorActionPreference = "Stop"

Write-Host "==> Build QNAP ($Platform) ra $OutDir"

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw "Docker chưa cài hoặc không có trong PATH"
}

$buildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$arch = ($Platform -split "/")[1]

$cmd = @(
    "docker","buildx","build",
    "--platform", $Platform,
    "-f","Dockerfile.qnap",
    "--build-arg","VERSION=$Version",
    "--build-arg","GO_VERSION=$GoVersion",
    "--build-arg","BUILD_DATE=$buildDate",
    "--output","type=local,dest=$OutDir",
    "."
)

if ($Progress) { $cmd += @("--progress", $Progress) }

Write-Host "Chạy: $($cmd -join ' ')"
& $cmd[0] @($cmd[1..($cmd.Count-1)])
if ($LASTEXITCODE -ne 0) { throw "docker build thất bại ($LASTEXITCODE)" }

Write-Host "==> Hoàn tất."
Write-Host "    Binary: $OutDir/bin/{scanner,deleter,reporter,reporter_opt}"
Write-Host "    Gói QNAP: $OutDir/qnap-scandir-$Version-$arch.tar.gz"