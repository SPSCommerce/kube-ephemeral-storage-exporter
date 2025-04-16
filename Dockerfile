FROM --platform=$BUILDPLATFORM golang:1.22.3-alpine3.19 AS builder
COPY . /sources
WORKDIR /sources/cmd

# BuildX will set this automatically
ARG TARGETARCH

# Cross-compile for target architecture
RUN GOARCH=${TARGETARCH} GOOS=linux go build -ldflags "-s" -o run

FROM --platform=$TARGETPLATFORM golang:1.22.3-alpine3.19
COPY --from=builder /sources/cmd/run /app/run
WORKDIR /app
ENTRYPOINT ["/app/run"]
CMD ["--port", "9000", "--refresh-interval", "60", "--plain-logs", "false"]