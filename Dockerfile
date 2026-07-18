# syntax=docker/dockerfile:1.7
# Multi-stage build for panya-charge-oss
# Produces a minimal static image with just the binary.

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
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /data

COPY --from=builder /out/panya-charge-oss /usr/local/bin/panya-charge-oss

EXPOSE 8887

ENTRYPOINT ["/usr/local/bin/panya-charge-oss"]
CMD ["-config", "/data/config.yaml"]
