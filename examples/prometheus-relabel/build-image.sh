#!/usr/bin/env sh
# Copyright The OpenTelemetry Authors
# SPDX-License-Identifier: Apache-2.0
set -eu
cd "$(dirname "$0")"
tag=${1:?usage: build-image.sh IMAGE_TAG}
stage=$(mktemp -d)
cleanup() {
	for artifact in relabel-collector relabelbench; do
		if [ -f "$stage/$artifact" ]; then unlink "$stage/$artifact"; fi
	done
	rmdir "$stage"
}
trap cleanup EXIT
revision=$(git rev-parse HEAD)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o "$stage/relabel-collector" ./cmd/collector
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o "$stage/relabelbench" ./cmd/bench
docker build --platform linux/amd64 --build-arg "REVISION=$revision" -t "$tag" -f Dockerfile "$stage"
