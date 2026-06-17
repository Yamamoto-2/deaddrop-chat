# syntax=docker/dockerfile:1

# 1) Build the frontend.
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ ./
RUN npm run build

# 2) Build the Go binary with the frontend embedded.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./web/dist
# Cross-compile the native CLI clients the server embeds + serves at /cli/bin.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o cli-dist/deaddrop-linux-amd64 ./cmd/cli \
 && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o cli-dist/deaddrop-linux-arm64 ./cmd/cli
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /deaddrop .

# 3) Minimal runtime: one static binary, one port.
FROM gcr.io/distroless/static-debian12 AS run
COPY --from=build /deaddrop /deaddrop
EXPOSE 7337
ENV PORT=7337
ENTRYPOINT ["/deaddrop"]
