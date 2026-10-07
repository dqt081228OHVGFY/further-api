# 在宝塔面板部署 further-api

本文使用本仓库的 Docker Compose 文件从源码构建网关。需要可运行 Docker Compose 的宝塔面板、已备案或可使用的域名，以及足够的磁盘空间构建前端和 Go 程序。

## 准备源码

在宝塔终端中进入自己的站点目录：

```bash
git clone https://github.com/dqt081228OHVGFY/further-api.git
cd further-api
cp compose.env.example .env
```

编辑 `.env`，设置随机的 `POSTGRES_PASSWORD`、`REDIS_PASSWORD` 与 `SESSION_SECRET`、`CRYPTO_SECRET`。密码会进入连接 URL，使用十六进制等 URL 安全字符。不要把 `.env` 提交到 Git。

## 启动

```bash
docker compose up -d --build
docker compose ps
docker compose logs -f further-api
```

默认监听 `3000`。通过宝塔的反向代理将域名的 HTTPS 请求转发到本机 `3000` 端口。启用 HTTPS 后，在 `.env` 中设置 `SESSION_COOKIE_SECURE=true` 和对应的 `SESSION_COOKIE_TRUSTED_URL=https://你的域名`，然后重新启动服务。首次访问时完成管理员安装。

## 更新与备份

更新前备份 `data/` 与 PostgreSQL 的 `pg_data` 卷。拉取已审核的源码版本后运行 `docker compose up -d --build`。不要在需要保留数据时执行 `docker compose down -v`。

若已有 PostgreSQL 卷使用其他数据库用户或库名，应先迁移数据或调整连接设置。当前默认用户与数据库均为 `further_api`。详细信息见根目录 [README](../../README.md)。
