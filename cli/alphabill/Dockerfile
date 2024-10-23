FROM golang:1.23-bookworm AS builder

RUN mkdir -p /target/lib && cp /lib/$(uname -m)-linux-gnu/libgcc_s.so.1 /target/lib/libgcc_s.so.1

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN make build

FROM gcr.io/distroless/base-debian12:debug-nonroot
SHELL [ "/busybox/sh", "-c" ]
WORKDIR /app
ENV PATH="/app:${PATH}"
COPY --from=builder /usr/src/app/build/alphabill /app/
COPY --from=builder /target/lib/libgcc_s.so.1 /lib/libgcc_s.so.1
ENTRYPOINT ["alphabill"]
