#!/bin/bash

set -euo pipefail

declare -a TARGETS=(
    "linux   amd64"
    "linux   arm64"
)

rm -rf build/ dist/
echo "Using ${SOURCE_DATE_EPOCH} as epoch date for the archive"
mkdir -p dist/

for target in "${TARGETS[@]}"; do
    read -r GOOS GOARCH <<< "${target}"

    # Set the output filename – Windows needs .exe
    OUT_DIR="build/${GOOS}-${GOARCH}"
    OUT_PATH="${OUT_DIR}/locaccel"

    echo "Packaging for ${GOOS}/${GOARCH} in ${OUT_DIR}"
    # Build
    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -ldflags="-s -w" -o "$OUT_PATH" ./cmd/locaccel
    # Include license
    cp LICENSE README.md "${OUT_DIR}"
    # create archive
    tar \
        --sort=name \
        --mtime="@${SOURCE_DATE_EPOCH}" \
        --owner=0 \
        --group=0 \
        --numeric-owner \
        --directory="${OUT_DIR}" \
        --create \
        --verbose \
        --file="dist/locaccel.${GOOS}-${GOARCH}.tar.gz" \
        .
done
