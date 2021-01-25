FROM golang:1.14-alpine as builder

ARG APP_VERSION=undef
ARG APP_REVISION=undef

WORKDIR /go/src/github.com/ryo-watanabe/k8s-snap/

RUN apk --no-cache add curl tar git ca-certificates && \
    update-ca-certificates

ADD . .

RUN CGO_ENABLED=0 go build -ldflags "-X main.version=$APP_VERSION -X main.revision=$APP_REVISION"

FROM alpine

COPY --from=builder /go/src/github.com/ryo-watanabe/k8s-snap/k8s-snap /
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENTRYPOINT ["/k8s-snap"]

