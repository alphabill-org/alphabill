FROM scratch AS go-dependency
# Empty

FROM golang:1.24-bookworm AS builder
ARG DOCKER_GO_DEPENDENCY=/go-dependency

RUN mkdir -p /target/lib && cp /lib/$(uname -m)-linux-gnu/libgcc_s.so.1 /target/lib/libgcc_s.so.1

WORKDIR /usr/src/app

COPY go.mod go.sum ./
RUN --mount=from=go-dependency,target=$DOCKER_GO_DEPENDENCY \
    go mod download && go mod verify

COPY . .
RUN --mount=from=go-dependency,target=$DOCKER_GO_DEPENDENCY \
    make build

FROM gcr.io/distroless/base-debian12:debug-nonroot
USER nonroot
SHELL [ "/busybox/sh", "-c" ]
WORKDIR /app
ENV PATH="/app:${PATH}"
COPY --from=builder /usr/src/app/build/alphabill /app/
COPY --from=builder /target/lib/libgcc_s.so.1 /lib/libgcc_s.so.1
ENTRYPOINT ["alphabill"]
