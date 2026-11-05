# Sub2API 服务器更新指南（198.2.220.193 部署）

> 适用服务器：`198.2.220.193`（SSH 端口 33911），生产环境部署。
> 本文档描述该服务器的容器部署架构、标准更新流程与回滚方法。其他机器部署方式不同，请勿照搬。

## 一、部署架构总览

```
服务器 (Ubuntu, 64G RAM)
├── /opt/sub2api/deploy/                  # 运行时部署目录（数据 + env）
│   ├── data/                             # ★ 挂载到容器 /app/data（config.yaml、迁移产物、持久化数据）
│   ├── sub2api.env                       # env 文件（compose 模板用，当前容器实际 env 见 §3）
│   ├── postgres_data/                    # ★ PostgreSQL 18 数据（独立容器，勿动）
│   ├── redis_data/                       # Redis 数据（独立容器，勿动）
│   ├── docker-compose.yml                # 官方镜像的 compose 模板（当前容器并未用它编排）
│   └── sub2api.rollback.image            # 历史回滚镜像备份
├── /opt/sub2api-runtime-build/           # 二进制覆盖式构建目录（旧方式，2026-09 前使用）
└── /opt/sub2api-src/                     # 源码 checkout（当前更新方式，见 §2）
```

### 容器与网络

| 容器 | 镜像 | 说明 |
|---|---|---|
| `sub2api` | `sub2api-niuniu:merged` | 业务应用（本仓库构建），**唯一允许重启/重建的** |
| `sub2api-postgres` | postgres:18-alpine | 数据库，内部端口 5432，**勿动** |
| `sub2api-redis` | redis:8-alpine | 缓存，**勿动** |
| grok2api / leo2api / sleep-reg-* / ... | 其他 | 与本服务无关，**严禁触碰** |

- **网络**：`deploy_sub2api-network`（bridge），sub2api / postgres / redis 同网；容器通过主机名 `sub2api-postgres`、`sub2api-redis` 互访。**重建 sub2api 容器必须保持 `--network deploy_sub2api-network`。**
- **端口**：宿主机 `8080 -> 容器 8080`。
- **挂载**：`/opt/sub2api/deploy/data -> /app/data`（配置与持久化数据在宿主机，重建容器不丢数据）。
- **重启策略**：`unless-stopped`。

### 镜像说明

- `sub2api-niuniu:merged` = 本仓库 `deploy/Dockerfile` 多阶段构建产物：
  Stage1 node:24-alpine 构建**前端**（pnpm build → `backend/internal/web/dist`）→
  Stage2 golang:1.27.0-alpine 构建**后端**（`go build -tags embed`，前端内嵌进二进制）→
  Stage3 alpine:3.20 运行镜像。
- Dockerfile 已内置国内代理（GOPROXY=goproxy.cn），服务器可直接构建。
- 数据库迁移（`backend/migrations/*.sql`）随镜像内嵌，**应用启动时自动执行**（幂等，可安全重复）。

## 二、标准更新流程

> 原则：构建不影响运行中容器；停机窗口只有最后「rm + run」的几秒钟；其他容器零接触。

### 1. 本地提交并推送

```bash
git add -A && git commit -m "..."
git push origin sync-upstream-v0.2.0-image-billing
```

### 2. 服务器拉取源码

```bash
ssh -p 33911 root@198.2.220.193
cd /opt/sub2api-src
git fetch origin && git checkout sync-upstream-v0.2.0-image-billing && git pull origin sync-upstream-v0.2.0-image-billing
```

首次部署时：`git clone --branch sync-upstream-v0.2.0-image-billing https://github.com/niuniu-fht/sub2api.git /opt/sub2api-src`

### 3. 备份当前镜像（回滚保险）

```bash
docker tag sub2api-niuniu:merged sub2api-niuniu:rollback-$(date +%Y%m%d-%H%M)
```

### 4. 构建新镜像（运行中的容器不受影响）

```bash
cd /opt/sub2api-src
docker build -f deploy/Dockerfile -t sub2api-niuniu:merged .
# 构建约 3-10 分钟；日志里看到 "writing image" 即成功
```

### 5. 捕获当前容器运行参数（重建前必做）

```bash
# 完整 env（含 DATABASE_HOST 等运行配置）
docker inspect sub2api --format '{{range .Config.Env}}{{println .}}{{end}}' \
  | grep -vE '^(PATH|HOME|HOSTNAME)=' > /opt/sub2api-runtime-build/container.env
# 其余运行参数（网络/端口/挂载/重启策略），与 §1 核对
docker inspect sub2api --format 'NET={{.HostConfig.NetworkMode}} RESTART={{.HostConfig.RestartPolicy.Name}} PORTS={{json .HostConfig.PortBindings}} BINDS={{json .HostConfig.Binds}}'
```

### 6. 重建容器（唯一停机窗口，秒级）

```bash
docker rm -f sub2api
docker run -d --name sub2api \
  --network deploy_sub2api-network \
  -p 8080:8080 \
  --env-file /opt/sub2api-runtime-build/container.env \
  -v /opt/sub2api/deploy/data:/app/data \
  --restart unless-stopped \
  sub2api-niuniu:merged
```

### 7. 验证

```bash
sleep 10 && curl -s http://127.0.0.1:8080/health            # 期望 {"status":"ok"}
docker logs sub2api --tail 30                              # 无 ERROR/FATAL；能看到迁移执行日志
docker exec sub2api-postgres psql -U sub2api -d sub2api -At -c \
  "SELECT column_name FROM information_schema.columns WHERE table_name='usage_logs' AND column_name='account_attempts';"
# 期望输出 account_attempts（对应 migrations/242，仅本次更新需要验证）
docker ps --format '{{.Names}} {{.Status}}'                # 其他容器全部 Up，未受影响
```

外部访问 `http://198.2.220.193:8080`（或实际对外地址）登录验证前端。

## 三、回滚

```bash
docker tag sub2api-niuniu:rollback-<时间戳> sub2api-niuniu:merged
docker rm -f sub2api
docker run -d --name sub2api \
  --network deploy_sub2api-network -p 8080:8080 \
  --env-file /opt/sub2api-runtime-build/container.env \
  -v /opt/sub2api/deploy/data:/app/data \
  --restart unless-stopped \
  sub2api-niuniu:merged
```

- 数据库迁移为加列类幂等操作，旧版本二进制遇到多出来的列不受影响，无需回滚数据库。
- 极端情况可用 `/opt/sub2api/deploy/postgres_data` 整体恢复（先停 sub2api-postgres，恢复前必备份）。

## 四、注意事项

1. **只允许重建 `sub2api` 容器**；postgres/redis/grok2api/leo2api 等一律不碰。
2. 重建必须保持 `--network deploy_sub2api-network`，否则容器连不上数据库（报 `lookup sub2api-postgres: no such host`）。
3. `/opt/sub2api/deploy/data/config.yaml` 是**实际生效配置**（数据库 5432、端口 8080 等），与本地开发配置（backend/config.yaml，5433/18081）不同，勿混用。
4. 前端改动已内嵌进 Go 二进制（`-tags embed`），无需单独部署前端。
5. 构建失败优先看是否拉取基础镜像失败（网络），重跑 `docker build` 即可；构建过程不影响线上。
6. 每次更新后把新的回滚 tag 记录在 `/opt/sub2api-runtime-build/` 下（如 `rollback.log`），保留最近 3 个。
