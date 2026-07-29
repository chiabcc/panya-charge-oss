# syntax=docker/dockerfile:1.7

# ---------- Build stage ----------
FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build \
	-trimpath \
	-ldflags="-s -w" \
	-o /out/panya-charge-oss \
	./cmd/panya-charge-oss/

# ---------- Runtime stage ----------
FROM alpine:3.20

RUN apk add --no-cache bash jq curl

COPY --from=builder /out/panya-charge-oss /usr/local/bin/panya-charge-oss
COPY ha-addon/run.sh /run.sh
COPY ha-addon/icon.png /icon.png
COPY ha-addon/logo.png /logo.png
RUN chmod +x /run.sh

WORKDIR /data

EXPOSE 8887 8888

LABEL \
	io.hass.version="VERSION" \
	io.hass.type="app" \
	io.hass.arch="aarch64,amd64"

CMD ["/run.sh"]
