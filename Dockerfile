FROM --platform=$BUILDPLATFORM golang:1.23.0-alpine3.19 AS builder
COPY . /sources
WORKDIR /sources

# BuildX will set this automatically
ARG TARGETARCH

RUN apk add --no-cache git

RUN go mod download

# Cross-compile for target architecture
RUN GOARCH=${TARGETARCH} GOOS=linux GOTOOLCHAIN=auto go build -ldflags "-s" -o run ./cmd

FROM --platform=$TARGETPLATFORM golang:1.23.0-alpine3.19
COPY --from=builder /sources/run /app/run
WORKDIR /app
ENTRYPOINT ["/app/run"]
CMD ["--port", "9000", "--refresh-interval", "60", "--plain-logs", "false"]