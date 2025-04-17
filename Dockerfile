FROM --platform=$BUILDPLATFORM golang:1.24.2-alpine3.21 AS builder
COPY . /sources
WORKDIR /sources/cmd

# BuildX will set this automatically
ARG TARGETARCH

RUN apk add --no-cache git

RUN cd .. && go mod download

# Cross-compile for target architecture
RUN GOARCH=${TARGETARCH} GOOS=linux GOTOOLCHAIN=auto go build -ldflags "-s" -o run

FROM --platform=$TARGETPLATFORM golang:1.24.2-alpine3.21
COPY --from=builder /sources/cmd/run /app/run
WORKDIR /app
ENTRYPOINT ["/app/run"]
CMD ["--port", "9000", "--refresh-interval", "60", "--plain-logs", "false"]