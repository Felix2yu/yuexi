# 运行时镜像：二进制由 CI 预编译并下载到 bin/ 后拼装。
#
# 基镜 ghcr.io/felix2yu/base:alpine-3.24 已提供 ca-certificates、tzdata、su-exec
# 与 PUID/PGID 降权入口（ENTRYPOINT /usr/local/bin/docker-entrypoint.sh），
# 因此这里不再 apk add，也不自带入口脚本。契约见 Felix2yu/.github/docs/docker.md。
FROM ghcr.io/felix2yu/base:alpine-3.24

WORKDIR /app

# 二进制装进 /usr/local/bin（基镜契约），而不是 /app：装在 /app 时它不在 PATH 上，
# `docker exec <c> yuexi backup` 与「按镜像名推导导出命令」都解析不到。
COPY --chmod=755 bin/yuexi /usr/local/bin/yuexi

RUN mkdir -p /app/data

ENV YUEXI_PORT=8080 \
    YUEXI_DB_PATH=/app/data/yuexi.db \
    APP_DATA_DIR=/app/data

VOLUME /app/data

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1

# ENTRYPOINT 由基镜提供（PUID/PGID 降权），这里只给服务命令
CMD ["yuexi"]
