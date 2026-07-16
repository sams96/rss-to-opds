FROM golang:1.26-alpine AS builder

WORKDIR ${GOPATH}/src/github.com/sams96/rss-to-opds/

COPY go.mod go.sum ${GOPATH}/src/github.com/sams96/rss-to-opds/
RUN go mod download

RUN apk add --no-cache build-base pkgconfig vips-dev

COPY . ${GOPATH}/src/github.com/sams96/rss-to-opds/

RUN CGO_ENABLED=1 GOOS=linux go build -o /go/bin/rss-to-opds .

FROM alpine:3.23

RUN apk add --no-cache vips

COPY --from=builder /usr/local/go/lib/time/zoneinfo.zip /zoneinfo.zip
ENV ZONEINFO=/zoneinfo.zip

COPY --from=builder /go/bin/rss-to-opds /usr/bin/rss-to-opds

ENTRYPOINT ["/usr/bin/rss-to-opds"]
