# 运行时镜像：二进制由 CI 预编译并下载到 bin/ 后拼装，
# 镜像内不再拉 Go 工具链（构建从 ~10min 降到 ~1min）。
FROM alpine:3.24

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --chmod=755 bin/yuexi /app/yuexi

ENV YUEXI_PORT=8080
ENV YUEXI_DB_PATH=/app/data/yuexi.db

VOLUME /app/data

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/yuexi"]
