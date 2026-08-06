FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /yuexi .

FROM alpine:3.24

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /yuexi .

ENV YUEXI_PORT=8080
ENV YUEXI_DB_PATH=/app/data/yuexi.db

VOLUME /app/data

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["./yuexi"]
