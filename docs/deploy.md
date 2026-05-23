# RDev 单机部署指南

本文档说明如何在单台服务器上使用 Docker Compose 部署完整的 RDev 服务栈。

## 架构概览

```
          ┌─────────────────────────────────────┐
          │          宿主机 127.0.0.1            │
          │                                     │
 :80 ─────┤─► rdev-web (Nginx)                  │
 :3001 ───┤─► gitea (可选)                      │
          └───────────┬─────────────────────────┘
                      │ internal 网络
          ┌───────────▼─────────────────────────┐
          │                                     │
          │  rdev-server  :8080  (Go backend)   │
          │  rdev-frontend:3000  (Next.js)       │
          │  rdev-gateway :8090  (模型网关)      │
          │  postgres     :5432  (pgvector)      │
          │  gitea        :3000  (Gitea, 可选)   │
          └─────────────────────────────────────┘
```

| 服务 | 说明 | 对外端口 |
|------|------|---------|
| `rdev-web` | Nginx 反向代理入口 | `127.0.0.1:80` |
| `rdev-server` | Go 后端 API + WebSocket | 内部 |
| `rdev-frontend` | Next.js SSR 前端 | 内部 |
| `rdev-gateway` | Anthropic↔OpenAI 模型网关 | 内部 |
| `postgres` | PostgreSQL 17 + pgvector | 内部 |
| `gitea` | 本地 Gitea VCS（可选） | `127.0.0.1:3001` |

## 快速开始

### 1. 克隆仓库并进入 deploy 目录

```bash
git clone git@github.com:zinohome/RDev.git
cd RDev/deploy
```

### 2. 配置环境变量

```bash
cp .env.example .env
```

**必须修改的变量**（搜索 `CHANGE_ME`）：

| 变量 | 说明 | 生成方法 |
|------|------|---------|
| `POSTGRES_PASSWORD` | 数据库密码 | 随机强密码 |
| `JWT_SECRET` | JWT 签名密钥 | `openssl rand -base64 32` |
| `RDEV_ENCRYPTION_KEY` | 数据加密密钥（32 字节）| `openssl rand -base64 32` |
| `GITEA_SECRET_KEY` | Gitea 密钥 | `openssl rand -hex 64` |
| `GITEA_INTERNAL_TOKEN` | Gitea 内部 Token | `openssl rand -hex 64` |
| `ANTHROPIC_API_KEY` | Anthropic API Key（可选） | 从 console.anthropic.com 获取 |

### 3. 构建并启动服务

```bash
# 仅启动核心服务（不含 Gitea）
docker compose up -d

# 包含 Gitea VCS 服务
docker compose --profile gitea up -d
```

首次启动会自动：
- 执行数据库迁移（`./migrate up`）
- 生成加密密钥（若 `.env` 中未设置）

### 4. 验证服务状态

```bash
# 查看所有服务状态
docker compose ps

# 查看服务日志
docker compose logs -f rdev-server

# 检查健康状态
curl http://localhost/nginx-health
curl http://localhost/api/health
```

### 5. 登录

访问 http://localhost，使用邮箱注册。

若未配置 `RESEND_API_KEY`，验证码会打印到 `rdev-server` 日志：

```bash
docker compose logs rdev-server | grep "Verification code"
```

## 更新部署

```bash
git pull
docker compose build --no-cache
docker compose up -d
```

## 启用 Gitea 集成

1. 启动 Gitea：

```bash
docker compose --profile gitea up -d gitea
```

2. 访问 http://localhost:3001，完成 Gitea 初始化向导

3. 在 Gitea 创建管理员 Token（Settings → Applications → Generate Token）

4. 更新 `.env`：

```dotenv
GITEA_ADMIN_TOKEN=your_token_here
```

5. 重启 rdev-server：

```bash
docker compose restart rdev-server
```

## 公网暴露（生产环境）

服务默认绑定 `127.0.0.1`，不直接对外暴露。在前面架设 TLS 反向代理：

### Caddy（推荐）

```caddyfile
your-domain.com {
    reverse_proxy localhost:80
}
```

同时更新 `.env`：

```dotenv
FRONTEND_ORIGIN=https://your-domain.com
MULTICA_APP_URL=https://your-domain.com
MULTICA_PUBLIC_URL=https://your-domain.com
MULTICA_TRUSTED_PROXIES=127.0.0.1/32
```

### Nginx（外层）

```nginx
server {
    listen 443 ssl;
    server_name your-domain.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://127.0.0.1:80;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
    }
}
```

## 数据持久化

所有持久化数据存储在 Docker 命名卷中：

| 卷名 | 内容 |
|------|------|
| `rdev_pgdata` | PostgreSQL 数据文件 |
| `rdev_gitea_data` | Gitea 仓库和配置 |
| `rdev_rdev_uploads` | 上传文件 |

**备份数据库**：

```bash
docker compose exec postgres pg_dump -U rdev rdev > backup_$(date +%Y%m%d).sql
```

**恢复数据库**：

```bash
docker compose exec -T postgres psql -U rdev rdev < backup.sql
```

## 常见问题

### 服务启动失败

```bash
# 查看详细错误
docker compose logs rdev-server

# 检查 postgres 是否就绪
docker compose exec postgres pg_isready -U rdev
```

### 端口冲突

修改 `.env` 中的端口变量：

```dotenv
WEB_PORT=8080     # Nginx 端口（默认 80）
GITEA_PORT=3002   # Gitea 端口（默认 3001）
```

### 清理重新部署

```bash
# 停止服务并删除容器（保留数据卷）
docker compose down

# 完全清理（包括数据卷，慎用！）
docker compose down -v
```

## 目录结构

```
deploy/
├── docker-compose.yml    # 主 Compose 文件
├── .env.example          # 环境变量模板
├── .env                  # 实际配置（不提交 git）
├── Dockerfile.server     # 后端多阶段构建
├── Dockerfile.web        # 前端多阶段构建（Next.js）
├── Dockerfile.gateway    # 模型网关构建
├── init.sh               # 服务启动初始化脚本
└── nginx.conf            # Nginx 配置
```
