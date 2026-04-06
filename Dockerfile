# Dockerfile
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

ARG TARGETOS TARGETARCH

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" \
    -o qbitctrl ./cmd/server/

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata curl bash && \
    rm -rf /var/cache/apk/*

RUN addgroup -g 1000 qbitctrl && \
    adduser -D -u 1000 -G qbitctrl qbitctrl

WORKDIR /app

COPY --from=builder /app/qbitctrl .

RUN chown -R qbitctrl:qbitctrl /app

USER qbitctrl

EXPOSE 9911

ENV TZ=UTC

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:9911/health || exit 1

ENTRYPOINT ["./qbitctrl"]
