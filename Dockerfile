FROM golang:1.22.1-alpine3.19
COPY . /sources
WORKDIR /sources/cmd
RUN go build -ldflags "-s" -o run

FROM golang:1.22.1-alpine3.19
COPY --from=0 /sources/cmd/run /app/run
WORKDIR /app
ENTRYPOINT ["/app/run"]
CMD ["--port", "9000", "--refresh-interval", "60", "--plain-logs", "false"]
