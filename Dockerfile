FROM golang:1.11.5-alpine as builder

WORKDIR /go/src/github.com/ryo-watanabe/k8s-snap/

RUN apk --no-cache add curl tar git ca-certificates && \
    update-ca-certificates

RUN curl -sL https://github.com/golang/dep/releases/download/v0.5.4/dep-linux-amd64 -o /usr/local/bin/dep && \
    chmod +x /usr/local/bin/dep

ADD . .

RUN dep ensure -v && \
    CGO_ENABLED=0 go build -ldflags "-X main.version=$APP_VERSION -X main.revision=$APP_REVISION"

FROM alpine

COPY --from=builder /go/src/github.com/ryo-watanabe/k8s-snap/k8s-snap /
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENTRYPOINT ["/k8s-snap"]
