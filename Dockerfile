# 运行时镜像：二进制由 CI 预编译并下载到 bin/ 后拼装。
#
# 基镜 ghcr.io/felix2yu/base:alpine-3.24 已提供 ca-certificates、tzdata、su-exec
# 与 PUID/PGID 降权入口（ENTRYPOINT /usr/local/bin/docker-entrypoint.sh），
# 因此这里不再 apk add，也不自带入口脚本。契约见 Felix2yu/.github/docs/docker.md。
FROM ghcr.io/felix2yu/base:alpine-3.24

WORKDIR /app

# 本仓库的容器布局：二进制在 /app，数据在 /app/data。
# /app 不在基镜的 PATH 上，所以启动命令与外部备份工具都用绝对路径
# （CMD ["/app/yuexi"]、BACKUP_SQLITE_CMD=/app/yuexi backup）。
COPY --chmod=755 bin/yuexi /app/yuexi

RUN mkdir -p /app/data

ENV YUEXI_PORT=8080 \
    YUEXI_DB_PATH=/app/data/yuexi.db \
    APP_DATA_DIR=/app/data

VOLUME /app/data

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1

# ENTRYPOINT 由基镜提供（PUID/PGID 降权），这里只给服务命令；/app 不在 PATH 上，用绝对路径
CMD ["/app/yuexi"]
