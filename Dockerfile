# syntax=docker/dockerfile:1

FROM golang:1.27-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -ldflags='-s -w' -o /out/mcp-monday-projects ./cmd/mcp-server
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -ldflags='-s -w' -o /out/mcp-monday-oauth ./cmd/mcp-oauth

FROM golang:1.27-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS race-builder

WORKDIR /src
RUN apk add --no-cache build-base=0.5-r4
COPY go.mod go.sum ./
RUN go mod download
COPY . .

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

LABEL org.opencontainers.image.title="Monday.com MCP Server" \
      org.opencontainers.image.description="Go MCP server for monday.com" \
      org.opencontainers.image.source="https://github.com/jersonmartinez/mcp-monday-projects"

COPY --from=builder /out/mcp-monday-projects /mcp-monday-projects
COPY --from=builder /out/mcp-monday-oauth /mcp-monday-oauth
# Profiles are mounted read-only at /profiles (see docs/CAPABILITIES.md#profiles).
ENV MCP_PROFILES_DIR=/profiles
USER nonroot:nonroot
ENTRYPOINT ["/mcp-monday-projects"]
