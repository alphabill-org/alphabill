FROM golang:1.23-bullseye AS builder

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN make build

FROM gcr.io/distroless/base-debian11:debug-nonroot
SHELL [ "/busybox/sh", "-c" ]
WORKDIR /app
ENV PATH="/app:${PATH}"
COPY --from=builder /usr/src/app/build/alphabill /app/
COPY --from=builder /lib/x86_64-linux-gnu/libgcc_s.so.1 /lib/x86_64-linux-gnu/libgcc_s.so.1
ENTRYPOINT ["alphabill"]
