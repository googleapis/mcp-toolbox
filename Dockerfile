ARG _AR_REPO_NAME=toolbox
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
FROM --platform=$BUILDPLATFORM us-central1-docker.pkg.dev/mcp-toolbox/${_AR_REPO_NAME}/golang:1@sha256:3680233e3204827fbdc66088528ae6d4b3d034f51d03a99d454f6de034888244 AS build

# Install prerequisites
RUN apt-get update && apt-get install -y xz-utils

WORKDIR /go/src/mcp-toolbox
COPY . .

# Install Zig for CGO cross-compilation
RUN mkdir -p /zig && \
    tar -xf zig.tar.xz -C /zig --strip-components=1 && \
    rm -f zig.tar.xz

ARG TARGETOS
ARG TARGETARCH
ARG BUILD_TYPE="container.dev"
ARG COMMIT_SHA=""

RUN go get ./...

RUN export ZIG_TARGET="" && \
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
    -o mcp-toolbox .

# Final Stage
FROM  us-central1-docker.pkg.dev/mcp-toolbox/${_AR_REPO_NAME}/gcr.io/distroless/cc-debian12:nonroot@sha256:9dac0a79194e45a7da0158a9c6da57b217585af0786db3845d1f0ec1a0dd182f

WORKDIR /app
COPY --from=build --chown=nonroot /go/src/mcp-toolbox/mcp-toolbox /toolbox
USER nonroot

LABEL io.modelcontextprotocol.server.name="io.github.googleapis/mcp-toolbox"

ENTRYPOINT ["/toolbox"] 
