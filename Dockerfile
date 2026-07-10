FROM golang:1.24-alpine AS builder

WORKDIR ${GOPATH}/src/github.com/sams96/rss-to-opds/

COPY go.mod go.sum ${GOPATH}/src/github.com/sams96/rss-to-opds/
RUN go mod download

COPY . ${GOPATH}/src/github.com/sams96/rss-to-opds/

RUN go build -o /go/bin/rss-to-opds .

FROM docker

ADD https://github.com/golang/go/raw/master/lib/time/zoneinfo.zip /zoneinfo.zip
ENV ZONEINFO=/zoneinfo.zip

COPY --from=builder /go/bin/rss-to-opds /usr/bin/rss-to-opds

ENTRYPOINT ["/usr/bin/rss-to-opds"]
