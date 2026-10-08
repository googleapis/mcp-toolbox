# Copyright 2024 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
ARG BASE_IMAGE="golang:1"
FROM --platform=$BUILDPLATFORM ${BASE_IMAGE} AS build

# Install Zig for CGO cross-compilation
RUN --mount=type=secret,id=airlock_token \
    if [ -f /run/secrets/airlock_token ]; then \
        export TOKEN=$(cat /run/secrets/airlock_token) && \
        . /etc/os-release && \
        if [ "$ID" = "ubuntu" ]; then REPO="ubuntu-${VERSION_CODENAME}-3p-trusted"; else REPO="standard-debian-${VERSION_CODENAME}-3p-l1"; fi && \
        echo "machine us-apt.pkg.dev login oauth2accesstoken password ${TOKEN}" > /etc/apt/auth.conf && \
        rm -f /etc/apt/sources.list.d/* /etc/apt/sources.list && \
        echo "deb [trusted=yes] https://us-apt.pkg.dev/projects/artifact-foundry-prod ${REPO} main" > /etc/apt/sources.list; \
    fi && \
    apt-get update && apt-get install -y xz-utils && \
    rm -f /etc/apt/auth.conf
RUN curl -fL "https://ziglang.org/download/0.15.2/zig-x86_64-linux-0.15.2.tar.xz" -o zig.tar.xz && \
    mkdir -p /zig && \
    tar -xf zig.tar.xz -C /zig --strip-components=1 && \
    rm zig.tar.xz

WORKDIR /go/src/mcp-toolbox
COPY . .

ARG TARGETOS
ARG TARGETARCH
ARG BUILD_TYPE="container.dev"
ARG COMMIT_SHA=""

ARG GOPROXY
ENV GOPROXY=${GOPROXY}
ARG GONOSUMDB
ENV GONOSUMDB=${GONOSUMDB}
RUN --mount=type=secret,id=airlock_token \
    if [ -f /run/secrets/airlock_token ]; then \
        echo "machine us-go.pkg.dev login oauth2accesstoken password $(cat /run/secrets/airlock_token)" > ~/.netrc; \
    fi && \
    go get ./... && \
    export ZIG_TARGET="" && \
    case "${TARGETARCH}" in \
      ("amd64") ZIG_TARGET="x86_64-linux-gnu" ;; \
      ("arm64") ZIG_TARGET="aarch64-linux-gnu" ;; \
      (*) echo "Unsupported architecture: ${TARGETARCH}" && exit 1 ;; \
    esac && \
    CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    CC="/zig/zig cc -target ${ZIG_TARGET}" \
    CXX="/zig/zig c++ -target ${ZIG_TARGET}" \
    go build \
    -ldflags "-X github.com/googleapis/mcp-toolbox/cmd.buildType=${BUILD_TYPE} -X github.com/googleapis/mcp-toolbox/cmd.commitSha=${COMMIT_SHA}" \
    -o mcp-toolbox . && \
    rm -f ~/.netrc

# Final Stage
FROM gcr.io/distroless/cc-debian12:nonroot

WORKDIR /app
COPY --from=build --chown=nonroot /go/src/mcp-toolbox/mcp-toolbox /toolbox
USER nonroot

LABEL io.modelcontextprotocol.server.name="io.github.googleapis/mcp-toolbox"

ENTRYPOINT ["/toolbox"] 
