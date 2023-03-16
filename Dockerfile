FROM golang:1.19.5-alpine3.17
COPY . /sources
WORKDIR /sources
RUN go build -ldflags "-s" -o run


FROM golang:1.19.5-alpine3.17
COPY --from=0 /sources/run /app/run
WORKDIR /app
ENTRYPOINT ["/app/run"]
CMD ["--port", "9000", "--refresh-interval", "60", "--plain-logs", "false"]
