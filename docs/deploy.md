# RDev 单机 Docker Compose 部署指南

## 概览

RDev 使用 Docker Compose 在单台服务器上部署完整的 AI 开发协作平台。

```
         Internet
            │
            ▼
    ┌───────────────┐
    │   rdev-web    │  :80  (nginx 反向代理)
    └───────┬───────┘
            │
   ┌────────┼────────┬──────────────┐
   ▼        ▼        ▼              ▼
rdev-frontend  rdev-server  rdev-gateway  gitea*
  :3000       :8080         :8090         :3000*
              │
              ▼
          postgres
           :5432
```

`*` 表示可选服务（通过 `--profile` 启用）

---

## 前置条件

- Docker >= 24.0
- Docker Compose >= 2.20
- 内存 ≥ 4 GB（启用 Ollama/vLLM 时建议 ≥ 16 GB）
- GPU 驱动（仅 `--profile vllm` 需要）

---

## 快速启动

### 1. 复制并编辑环境变量

```bash
cp deploy/.env.example deploy/.env
```

编辑 `deploy/.env`，**必须修改**以下变量：

| 变量 | 说明 |
|------|------|
| `POSTGRES_PASSWORD` | PostgreSQL 密码 |
| `JWT_SECRET` | JWT 签名密钥（`openssl rand -hex 32`）|
| `RDEV_ENCRYPTION_KEY` | 数据加密密钥（`openssl rand -hex 32`）|

### 2. 启动核心服务

```bash
docker compose -f deploy/docker-compose.yml up -d
```

首次启动约需 3–5 分钟（构建镜像）。

### 3. 检查服务状态

```bash
docker compose -f deploy/docker-compose.yml ps
docker compose -f deploy/docker-compose.yml logs -f rdev-server
```

### 4. 访问服务

| 服务 | 地址 |
|------|------|
| 前端 | http://localhost |
| 后端 API | http://localhost/api |
| 模型网关 | http://localhost/v1（内部：http://localhost:8090）|

---

## 可选服务

### 启用 Gitea（内网 Git）

编辑 `deploy/.env`：
```bash
GITEA_SECRET_KEY=$(openssl rand -hex 32)
GITEA_ROOT_URL=http://your-server-ip:3000
```

启动：
```bash
docker compose -f deploy/docker-compose.yml --profile gitea up -d
```

访问 http://localhost:3000 完成 Gitea 初始化向导。

初始化后，创建管理员 token 并填入 `.env`：
```bash
GITEA_URL=http://gitea:3000
GITEA_ADMIN_TOKEN=<从 Gitea 设置页面生成>
```

重启 rdev-server 以应用配置：
```bash
docker compose -f deploy/docker-compose.yml restart rdev-server
```

### 启用 Ollama（本地 LLM，CPU）

```bash
docker compose -f deploy/docker-compose.yml --profile ollama up -d

# 拉取模型
docker exec -it rdev-ollama-1 ollama pull qwen2.5:7b
```

在 `.env` 中设置网关使用 Ollama：
```bash
OLLAMA_URL=http://ollama:11434
MODEL_BACKEND_URL=http://rdev-gateway:8090
```

### 启用 vLLM（GPU 推理）

需要 NVIDIA GPU 和 nvidia-container-toolkit。

编辑 `.env`：
```bash
VLLM_MODEL=Qwen/Qwen2.5-7B-Instruct
HF_TOKEN=<your_huggingface_token>   # 下载受限模型时需要
```

```bash
docker compose -f deploy/docker-compose.yml --profile vllm up -d
```

### 同时启用多个可选服务

```bash
docker compose -f deploy/docker-compose.yml \
  --profile gitea \
  --profile ollama \
  up -d
```

---

## 目录结构

```
deploy/
├── docker-compose.yml    # 主部署文件
├── .env.example          # 环境变量模板
├── .env                  # 实际配置（不提交到 git）
├── Dockerfile.server     # 后端 + rdev/ 扩展镜像
├── Dockerfile.gateway    # 模型网关镜像
├── init.sh               # 容器启动脚本（迁移 + 启动）
└── nginx.conf            # Nginx 反向代理配置
```

根目录已有：
- `Dockerfile.web` — 前端 Next.js 镜像

---

## 端口说明

| 服务 | 内部端口 | 默认对外绑定 |
|------|---------|------------|
| `rdev-web`（nginx）| 80 | `0.0.0.0:80` |
| `rdev-server` | 8080 | 不对外暴露（通过 nginx 代理）|
| `rdev-gateway` | 8090 | `127.0.0.1:8090` |
| `gitea` HTTP | 3000 | `127.0.0.1:3000` |
| `gitea` SSH | 2222 | `127.0.0.1:2222` |
| `ollama` | 11434 | `127.0.0.1:11434` |
| `vllm` | 8000 | `127.0.0.1:8000` |
| `postgres` | 5432 | 不对外暴露 |

> **安全提示**：`rdev-web` 默认绑定 `0.0.0.0:80`（公网可访问）。如需仅本机访问，在 `.env` 中设置 `WEB_BIND=127.0.0.1`，再用 Nginx/Caddy 做 TLS 终止。

---

## 数据持久化

所有重要数据均通过 Docker volume 持久化：

| Volume | 内容 |
|--------|------|
| `rdev_pgdata` | PostgreSQL 数据库 |
| `rdev_server_uploads` | 用户上传文件 |
| `rdev_gitea_data` | Gitea 仓库数据 |
| `rdev_gitea_config` | Gitea 配置 |
| `rdev_ollama_data` | Ollama 模型缓存 |
| `rdev_vllm_cache` | vLLM/HuggingFace 缓存 |

**备份建议：**
```bash
# 备份 postgres
docker exec rdev-postgres-1 pg_dump -U rdev rdev > backup-$(date +%Y%m%d).sql

# 备份所有 volume
docker run --rm \
  -v rdev_pgdata:/data \
  -v $(pwd)/backup:/backup \
  alpine tar czf /backup/pgdata-$(date +%Y%m%d).tar.gz -C /data .
```

---

## 升级

```bash
# 拉取最新代码
git pull

# 重新构建并重启
docker compose -f deploy/docker-compose.yml build --no-cache
docker compose -f deploy/docker-compose.yml up -d
```

迁移在容器启动时自动执行（`init.sh` 调用 `./migrate up`）。

---

## 常见问题排查

### 服务无法启动

```bash
# 查看所有服务日志
docker compose -f deploy/docker-compose.yml logs

# 查看特定服务
docker compose -f deploy/docker-compose.yml logs rdev-server
```

### 数据库连接失败

确认 `POSTGRES_PASSWORD` 在 `.env` 中已设置，且 postgres 服务健康：
```bash
docker compose -f deploy/docker-compose.yml ps postgres
```

### 前端访问空白

检查 rdev-frontend 是否正常：
```bash
docker compose -f deploy/docker-compose.yml logs rdev-frontend
```

Next.js 首次构建需要时间，等待日志出现 `Ready in` 后再访问。

### 重置所有数据（危险）

```bash
docker compose -f deploy/docker-compose.yml down -v
```

`-v` 标志会删除所有 volume，包括数据库数据，**不可恢复**。

---

## 与现有 docker-compose 的关系

| 文件 | 用途 |
|------|------|
| `docker-compose.yml` | 本地开发（仅 postgres）|
| `docker-compose.selfhost.yml` | multica 官方镜像自托管 |
| `docker-compose.selfhost.build.yml` | 开发时本地构建覆盖 |
| `deploy/docker-compose.yml` | **RDev 生产单机部署**（本文档）|

`deploy/docker-compose.yml` 基于 `docker-compose.selfhost.yml` 扩展，增加了 rdev/ 扩展集成、nginx 代理层、以及 Gitea/Ollama/vLLM 可选服务。
