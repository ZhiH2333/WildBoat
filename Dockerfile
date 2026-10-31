# Pinned toolchain: Go 1.26.x / Alpine 3.22
FROM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/wildboat ./cmd/api

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata wget \
	&& addgroup -S app \
	&& adduser -S -G app app
WORKDIR /app
COPY --from=build /out/wildboat /app/wildboat
USER app
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=5 \
	CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/wildboat"]
