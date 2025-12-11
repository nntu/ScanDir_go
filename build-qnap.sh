#!/usr/bin/env bash
# Build nhanh binary QNAP bằng Docker (Linux/macOS/Windows + WSL/Docker Desktop)
# Sử dụng buildx --output để lấy binary và gói .tar.gz ra host.

set -euo pipefail

VERSION="${VERSION:-2.0.0-qnap}"
GO_VERSION="${GO_VERSION:-1.23.3}"
PLATFORM="${PLATFORM:-linux/amd64}"
OUT_DIR="${OUT_DIR:-./qnap-build}"
BUILD_DATE="${BUILD_DATE:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"
PROGRESS="${PROGRESS:-}" # đặt PROGRESS=plain để xem log chi tiết

ARCH="${PLATFORM#*/}"

echo "==> Build QNAP (${PLATFORM}) ra ${OUT_DIR}"
echo "    VERSION=${VERSION}, GO_VERSION=${GO_VERSION}, BUILD_DATE=${BUILD_DATE}"

cmd=(
  docker buildx build
  --platform "${PLATFORM}"
  -f Dockerfile.qnap
  --build-arg "VERSION=${VERSION}"
  --build-arg "GO_VERSION=${GO_VERSION}"
  --build-arg "BUILD_DATE=${BUILD_DATE}"
  --output "type=local,dest=${OUT_DIR}"
  .
)

if [[ -n "${PROGRESS}" ]]; then
  cmd+=(--progress "${PROGRESS}")
fi

"${cmd[@]}"

echo "==> Hoàn tất."
echo "    Binary: ${OUT_DIR}/bin/{scanner,deleter,reporter,reporter_opt}"
echo "    Gói QNAP: ${OUT_DIR}/qnap-scandir-${VERSION}-${ARCH}.tar.gz"