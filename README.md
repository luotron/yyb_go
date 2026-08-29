# 本地账号服务

本项目只负责二维码录入、本地账号管理、本地协议会话和小程序接口调用，所有路由不做鉴权。

## 快速启动

双击：

```text
.\bin\yyb-go.exe
```

默认地址：`https://127.0.0.1:8000`（配置了 `certs/` 证书；留空 `tls_cert/tls_key` 则回退 `http://127.0.0.1:8000`）  
接口文档：`https://127.0.0.1:8000/docs/index.html`

程序固定读取 `config/service.json`，不读取命令行参数、环境变量或 `.env`。启动文件不再切换模式或复制配置。

## 职责边界

| 能力 | 本项目 |
|---|---|
| 二维码创建、轮询和确认 | 是 |
| 本地账号 SQLite 存储 | 是 |
| 本地账号资料刷新与头像 | 是 |
| 本地协议池和 `/wxapp/*` 调用 | 是 |
| 启动存活刷新与 expired 账号自动删除 | 是 |
| 卡密／计费网关校验 | 否（已移除） |
| 来源同步和 `/aggregator/*` | 否 |
| 租约、个人黑名单和 `/client/*` | 否 |

聚合项目默认监听 `127.0.0.1:8001`，两个进程可同时运行，数据库和配置互不共用。

## 主要接口

### 状态与文档

- `GET /health`
- `GET /ready`
- `GET /openapi.json`
- `GET /docs/index.html`

### 二维码

- `POST /qr`
- `GET /qr/{session_id}/image`
- `GET /qr/{session_id}/poll`
- `POST /qr/{session_id}/confirm`

### 本地账号

- `GET|DELETE /accounts`
- `GET /accounts/avatar`
- `POST /accounts/refresh`
- `POST /accounts/resync`

服务启动时会自动执行一次存活刷新，刷新结果为 `expired` 的账号会被自动删除（结果见启动日志）；控制台账号队列亦提供「删除」按钮，对应 `DELETE /accounts?ref=...`。

### 本地小程序调用

- `POST /wxapp/getCode`
- `POST /wxapp/getPhoneNumber`
- `POST /wxapp/operateWxData`
- `POST /wxapp/getHostSign`
- `GET /features`
- `POST /accounts/{ref}/call`
- `POST /activity/sign`

所有接口均无需鉴权请求头，直接调用即可。完整字段和示例见 `API文档.md` 与运行时 OpenAPI。请勿把服务暴露到不受信任的网络。

## 两类签名

- `getHostSign` 通过本地 WMPF 会话调用微信 `verifyPlugin`，生成 `X-WECHAT-HOSTSIGN` 所需的 `host_sign/noncestr/timestamp`。
- `/activity/sign` 在本地生成业务请求体的 `signTimestamp/signNonce/sign`，输入为活动 `signKey` 与登录态 token。

两者输入、算法与落点均不同。

## 文件与数据

```text
cmd/yyb-go/main.go              服务入口
internal/httpapi/               本地 HTTP、OpenAPI 与脚本接口
internal/protocol/              本地协议池与 verifyPlugin HostSign
internal/crypto/                活动 HMAC 签名
internal/qr/                    扫码客户端
internal/scriptrunner/          用户脚本运行器与 cron 调度
internal/store/                 本地账号与协议会话
config/service.json             Windows 配置
config/service.docker.json      Docker 配置
certs/                          HTTPS 证书（cert.pem/key.pem，gitignored）
deploy/systemd/yyb-go.service   systemd 后台运行单元
resource/db/yyb.db              本地 SQLite 数据
resource/avatars/               本地头像
resource/qr/                    临时二维码图片
resource/templates/             页面模板
resource/static/                静态资源
resource/scripts/               用户 Python 脚本（sdk/ 为共享 SDK，其余 gitignored）
```

本地数据库现在只包含账号与协议会话表。旧 `features`、`account_leases`、`account_user_blacklist` 表及已过期会话已在拆分清理中移除。

## 服务配置

| 字段 | 说明 |
|---|---|
| `listen_address` | 明确的 `host:port`，默认 `127.0.0.1:8000`。 |
| `data_root` | SQLite、头像、二维码与用户脚本的数据根目录。 |
| `asset_root` | 模板和静态资源根目录（含脚本 SDK）。 |
| `database_filename` | 本地 SQLite 文件名。 |
| `tcp_proxy` | 本地协议连接使用的可选 TCP 代理。 |
| `keepalive_interval_minutes` | 账号保活检查间隔（分钟），默认 1。 |
| `keepalive_ahead_minutes` | 凭证过期前提前多久刷新（分钟），默认 45。 |
| `python_command` | 用户脚本解释器，默认自动探测 `python`/`python3`。 |
| `tls_cert` / `tls_key` | PEM 证书与私钥路径；同时配置即启用 HTTPS/WSS。 |
| `admin_user` / `admin_password` | 同时配置即启用管理员登录，首次启动时创建初始管理员。 |
| `session_duration_minutes` | 登录会话有效期（分钟），默认 1440。 |
| `cookie_secure` | 会话 Cookie 是否加 Secure 标记（纯 HTTPS 部署可开 true）。 |
| `integration_token` | 可选，自动化调用免登录令牌（请求头 `X-Integration-Token`）。 |

旧 `aggregator`、`billing_config_file` 配置字段会作为未知字段拒绝加载，用于及时发现未迁移的旧配置。

### 管理员登录

配置 `admin_user`/`admin_password` 后，除 `/login`、`/health`、`/ready`、`/docs/*`、`/static/*` 外所有页面与接口均需登录：

- 未登录访问页面重定向到 `/login`，API 返回 401
- 登录成功后发放 `yyb_session` Cookie（HttpOnly，默认 24 小时）
- 自动化脚本通过 `integration_token`（请求头 `X-Integration-Token`）免登录调用；服务运行用户脚本时自动注入 `YYB_INTEGRATION_TOKEN`，SDK 自动携带
- 左侧导航栏登录后出现「退出登录」入口

密码直接配置在 `service.json`（明文），登录会话保存在服务内存中（重启后全部失效）；`integration_token` 免登录头不受重启影响。修改 `admin_user`/`admin_password` 后重启即生效。

## 用户脚本

用户开发的 `.py` 脚本放在 `resource/scripts/`（网页 `/apps` 的「用户脚本」面板可上传、运行、定时、查看实时日志）：

- 脚本运行时注入 `YYB_SERVER`、`PYTHONPATH`（`resource/scripts/sdk`，内含 `yyb_sdk.py`）、`PYTHONUNBUFFERED=1` 等环境变量
- 日志通过 WebSocket（`GET /scripts/{name}/logs/ws`）实时推送到网页
- 定时任务支持 5 段 cron（分 时 日 月 周），调度器精确到秒，持久化于 `resource/scripts/schedules.json`

详细接口见 `API文档.md` 第八节。

## 开发与构建

```powershell
# 运行测试
go test ./...

# 编译 Windows 可执行文件（yyb-go.exe）
go build -trimpath -ldflags="-s -w" -o bin/yyb-go.exe ./cmd/yyb-go

# 交叉编译 Linux amd64（bin/yyb-go-local-linux-amd64）
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="amd64"
go build -trimpath -ldflags="-s -w" -o bin\yyb-go-local-linux-amd64 ./cmd/yyb-go

# 交叉编译树莓派 5（ARM64，bin/yyb-go-linux-arm64）
$env:CGO_ENABLED="0"; $env:GOOS="linux"; $env:GOARCH="arm64"
go build -trimpath -ldflags="-s -w" -o bin\yyb-go-linux-arm64 ./cmd/yyb-go
```

SQLite 驱动为纯 Go 实现（`modernc.org/sqlite`），因此可用 `CGO_ENABLED=0` 做静态编译与交叉编译。运行可执行文件时工作目录需为项目根目录（读取 `config/service.json` 与 `resource/`）。

## 树莓派 5 编译与后台运行

### 1. 安装 Go（64 位系统，需 Go 1.23+）

```bash
# 方式 A：官方二进制（推荐，Raspberry Pi OS 自带 apt 版本通常过旧）
wget https://go.dev/dl/go1.23.12.linux-arm64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.23.12.linux-arm64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc && source ~/.bashrc

# 方式 B：apt（版本满足时）
sudo apt-get update && sudo apt-get install -y golang-go
go version   # 验证
```

### 2. 拉取代码并编译

```bash
sudo mkdir -p /opt/yyb-go && sudo chown pi:pi /opt/yyb-go
git clone <仓库地址> /opt/yyb-go
cd /opt/yyb-go
go build -trimpath -ldflags="-s -w" -o yyb-go ./cmd/yyb-go
```

### 3. 补齐运行资源

```bash
cd /opt/yyb-go
mkdir -p certs          # 放入 cert.pem / key.pem（HTTPS 用；不配置则 HTTP）
sudo apt-get install -y python3 python3-pip
pip3 install requests   # 用户脚本 SDK 依赖
# 按需修改 config/service.json：局域网访问改为 "listen_address": "0.0.0.0:8000"
```

### 4. 后台运行

方式 A：systemd（推荐，开机自启 + 崩溃自动重启）

```bash
sudo cp deploy/systemd/yyb-go.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now yyb-go
journalctl -u yyb-go -f         # 查看日志
sudo systemctl restart yyb-go   # 重启
sudo systemctl stop yyb-go      # 停止
```

方式 B：nohup（简单，不随开机启动）

```bash
cd /opt/yyb-go
nohup ./yyb-go > yyb.log 2>&1 &
echo $! > yyb.pid
kill $(cat yyb.pid)   # 停止
```

`deploy/systemd/yyb-go.service` 中 `WorkingDirectory=/opt/yyb-go`，因此 `service.json` 里的 `resource`、`certs/` 等相对路径均按该目录解析；数据（账号库、脚本、日志）全部写在 `resource/` 下，无需 root。

## Docker

```powershell
docker compose up -d --build
docker compose ps
docker compose logs --tail 100
```

Compose 仅发布 `127.0.0.1:8000`，只挂载本地数据卷，不再挂载计费配置和聚合来源文件。
