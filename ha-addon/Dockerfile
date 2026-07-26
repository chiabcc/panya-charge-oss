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
FROM ghcr.io/home-assistant/base:latest

COPY --from=builder /out/panya-charge-oss /usr/local/bin/panya-charge-oss
COPY run.sh /run.sh
RUN chmod a+x /run.sh

WORKDIR /data

EXPOSE 8887

LABEL \
	io.hass.version="VERSION" \
	io.hass.type="app" \
	io.hass.arch="aarch64,amd64"

CMD ["/run.sh"]
