#!/bin/sh
# WeKnora Go 构建环境（Windows/Git Bash 用法：sh docker-go.sh <go 命令...>）
#
# 镜像:   weknora-go-builder —— golang:1.26 + build-essential + libsqlite3-dev
# 卷:     weknora-gomod-cache  → /go/pkg/mod        （Go 模块缓存，一次性预热）
#         weknora-gobuild-cache → /root/.cache/go-build （编译缓存，增量构建）
#
# 用法示例:
#   sh docs_for_bn/docker-go.sh build ./...
#   sh docs_for_bn/docker-go.sh test ./internal/application/service/ -count=1
#   sh docs_for_bn/docker-go.sh vet ./internal/...
#
# 首次使用前请先预热缓存（只需一次，见 README 或直接运行）:
#   sh docs_for_bn/docker-go.sh warmup

set -e

# Git Bash 的 pwd -W 直接给出 "C:/bn/gwz/WeKnora"（Docker 可用的正斜杠 Windows 路径）；
# 其他环境回退 pwd，若是 "/c/..." 形式则转成 "C:/..."。
REPO_DIR="$(cd "$(dirname "$0")/.." && pwd -W 2>/dev/null || true)"
if [ -z "$REPO_DIR" ]; then
  REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
  case "$REPO_DIR" in
    /[a-z]/*) REPO_DIR="$(echo "$REPO_DIR" | sed 's|^/\([a-zA-Z]\)/|\U\1:/|')" ;;
  esac
fi

# Git Bash 会把 /src 转换成 Windows 路径，禁掉
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) export MSYS_NO_PATHCONV=1 ;;
esac

if [ "$1" = "warmup" ]; then
  shift
  exec docker run --rm \
    -v "$REPO_DIR:/src" \
    -v weknora-gomod-cache:/go/pkg/mod \
    -v weknora-gobuild-cache:/root/.cache/go-build \
    -w /src \
    weknora-go-builder:latest sh -c "go mod download && go build ./... && echo WARMUP-DONE"
fi

exec docker run --rm \
  -v "$REPO_DIR:/src" \
  -v weknora-gomod-cache:/go/pkg/mod \
  -v weknora-gobuild-cache:/root/.cache/go-build \
  -w /src \
  weknora-go-builder:latest go "$@"
