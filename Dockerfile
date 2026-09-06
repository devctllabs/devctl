FROM golang:1.26-alpine AS builder

ARG VERSION=dev
ARG COMMIT

WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build \
    -buildvcs=false \
    -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /devctl ./cmd/devctl

FROM node:24-alpine

RUN apk add --no-cache ca-certificates git \
    && npm install --global --no-update-notifier --no-fund quicktype@26.0.0 \
    && npm cache clean --force

COPY --from=builder /usr/local/go /usr/local/go
COPY --from=builder /devctl /devctl
RUN ln -s /devctl /usr/local/bin/devctl

ENV PATH="/usr/local/go/bin:/go/bin:/usr/local/bin:${PATH}"

ENTRYPOINT ["/devctl"]
