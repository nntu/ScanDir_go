# ===== Stage 1: Builder (glibc 2.17 baseline)
FROM quay.io/pypa/manylinux2014_x86_64 AS builder

ARG GO_VERSION=1.24.0
ENV GOROOT=/usr/local/go \
    GOPATH=/go \
    PATH=/usr/local/go/bin:/go/bin:$PATH \
    CGO_ENABLED=1

# Cài toolchain (GCC có sẵn trong manylinux2014), tải Go 1.24.0
RUN curl -fsSL https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz -o /tmp/go.tgz \
 && tar -C /usr/local -xzf /tmp/go.tgz \
 && rm -f /tmp/go.tgz

WORKDIR /src
# copy module files trước để cache deps (tùy repo của bạn)
COPY go.mod go.sum ./
RUN go env -w GOMODCACHE=/go/pkg/mod && go mod download

# copy code
COPY . .

# Build CGO (liên kết động tới glibc baseline 2.17)
# thêm -ldflags "-s -w" để giảm kích thước
RUN go build -tags scanner -trimpath -ldflags="-s -w" -o /out/scanner .
RUN go build -tags deleter -trimpath -ldflags="-s -w" -o /out/deleter .
RUN go build -tags reporter -trimpath -ldflags="-s -w" -o /out/reporter .

# Kiểm tra các symbol GLIBC yêu cầu (tuỳ chọn)
RUN ldd /out/scanner && ldd /out/deleter && ldd /out/reporter && (strings -a /out/scanner /out/deleter /out/reporter | grep -o 'GLIBC_[0-9.]*' | sort -u || true)

# ===== Stage 2: Artifact (xuất binary)
FROM scratch AS artifact
COPY --from=builder /out/scanner /scanner
COPY --from=builder /out/deleter /deleter
COPY --from=builder /out/reporter /reporter
