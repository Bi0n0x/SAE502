FROM golang:1.23-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata wget && \
    adduser -D -u 10001 casino

COPY --from=builder /out/api /usr/local/bin/api

USER casino

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]
