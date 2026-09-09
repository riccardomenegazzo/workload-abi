#!/usr/bin/env bash
set -euo pipefail

version="${VERSION:-dev}"
dist="${DIST_DIR:-dist}"
platforms="${WABI_RELEASE_PLATFORMS:-linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64}"
include_schemas="${WABI_RELEASE_INCLUDE_SCHEMAS:-true}"

mkdir -p "$dist"

build_platform() {
  local os="$1"
  local arch="$2"
  local ext=""
  local name="wabi_${version}_${os}_${arch}"
  local root="${dist}/${name}"

  if [[ "$os" == "windows" ]]; then
    ext=".exe"
  fi

  mkdir -p "$root"

  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w -X main.version=${version}" \
    -o "${root}/wabi${ext}" ./cmd/wabi

  if [[ "$os" == "linux" ]]; then
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
      go build -trimpath -ldflags "-s -w -X main.version=${version}" \
      -o "${root}/wabi-native" ./cmd/wabi-native
  fi

  if [[ "$os" == "windows" ]]; then
    (
      cd "$dist"
      zip -q -r "${name}.zip" "$name"
    )
  else
    tar -C "$dist" -czf "${dist}/${name}.tar.gz" "$name"
  fi

  rm -rf "$root"
}

for platform in $platforms; do
  os="${platform%/*}"
  arch="${platform#*/}"
  if [[ -z "$os" || -z "$arch" || "$os" == "$arch" ]]; then
    echo "invalid release platform: $platform" >&2
    exit 2
  fi
  build_platform "$os" "$arch"
done

if [[ "$include_schemas" == "true" ]]; then
  tar -C schemas -czf "${dist}/wabi_${version}_schemas.tar.gz" .
fi

while IFS= read -r -d '' archive; do
  sha256sum "$archive"
done < <(find "$dist" -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.zip' \) -print0 | sort -z) \
  > "${dist}/checksums.txt"

for platform in $platforms; do
  os="${platform%/*}"
  arch="${platform#*/}"
  [[ "$os" == "linux" ]] || continue

  archive="${dist}/wabi_${version}_${os}_${arch}.tar.gz"
  contents="$(tar -tzf "$archive")"
  grep -q "/wabi$" <<<"$contents"
  grep -q "/wabi-native$" <<<"$contents"
done
