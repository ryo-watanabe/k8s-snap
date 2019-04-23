FROM alpine
RUN apk add --no-cache ca-certificates
COPY k8s-backup-controller /
