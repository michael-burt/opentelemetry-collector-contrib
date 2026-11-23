# Copyright The OpenTelemetry Authors
# SPDX-License-Identifier: Apache-2.0

FROM scratch
ARG REVISION
LABEL org.opencontainers.image.revision=$REVISION
COPY relabel-collector /relabel-collector
COPY relabelbench /relabelbench
ENV TMPDIR=/tmp
USER 65532:65532
ENTRYPOINT ["/relabel-collector"]
