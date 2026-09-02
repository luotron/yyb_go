# nginx + ACME 证书自动部署文档

目标：yyb-go 保持纯 HTTP/WS 服务，由 **nginx 终结 HTTPS/WSS**，证书通过 **acme.sh + 阿里云 DNS-01** 自动签发与续期，全程无需开放 80/443 端口。

## 一、架构与前提

```
浏览器 ──https/wss──> 公网网关(218.15.101.28):8000 ──端口映射──> 树莓派(192.168.1.63):8000
                                                                        │ nginx 终结 TLS
                                                                        ▼
                                                              http://127.0.0.1:8001 (yyb-go)
```

前提条件：

- 域名（如 `www.luotronserver.xyz`）DNS 托管在**阿里云**，A 记录指向公网服务器
- 公网网关把 **8000** 端口映射到树莓派的 **8000**（证书验证走 DNS，不需要 80 端口）
- 阿里云 **AccessKey**：RAM 用户 + 授权 `AliyunDNSFullAccess`
- 树莓派已安装 yyb-go（监听本地 8001）

## 二、部署步骤

### 1. 修改 yyb-go 监听地址

`/opt/yyb_go/config/service.json`：

```json
{
  "listen_address": "127.0.0.1:8001",
  "cookie_secure": true
}
```

重启 yyb-go：

```bash
sudo pkill -9 -f yyb-go
cd /opt/yyb_go && nohup ./bin/yyb-go > yyb.log 2>&1 &
```

> 服务端注入给用户脚本的 `YYB_SERVER` 会自动变为 `http://127.0.0.1:8001`，树莓派本地脚本直连后端，不经过公网。

### 2. 安装 nginx 与 acme.sh

```bash
apt-get update && apt-get install -y nginx curl socat
curl https://get.acme.sh | sh -s email=你的邮箱@example.com
```

- 邮箱仅用于接收 Let's Encrypt 到期/紧急通知，不影响签发，建议填真实邮箱
- 已安装后修改邮箱：`~/.acme.sh/acme.sh --update-account --accountemail 新邮箱@example.com`

### 3. 签发证书（阿里云 DNS-01）

```bash
export Ali_Key="LTAI5t********"       # AccessKey ID，LTAI 开头 20/24 位
export Ali_Secret="********"          # AccessKey Secret，约 30 位
~/.acme.sh/acme.sh --set-default-ca --server letsencrypt
~/.acme.sh/acme.sh --issue --dns dns_ali -d www.luotronserver.xyz --days 90
```

执行过程：

1. 自动在阿里云 DNS 添加 `_acme-challenge.www.luotronserver.xyz` TXT 记录
2. 等待传播后由 Let's Encrypt 验证
3. 验证通过即签发证书，并自动删除 TXT 记录

> 阿里云凭证会保存到 acme.sh 账户配置（`~/.acme.sh/account.conf`），续期自动沿用，无需再 export。
> 一键脚本见 `deploy/acme-aliyun.sh`（填好 ALI_KEY/ALI_SECRET 后 `sudo bash deploy/acme-aliyun.sh`）。

### 4. 安装证书并注册续期钩子

```bash
mkdir -p /etc/nginx/certs
~/.acme.sh/acme.sh --install-cert -d www.luotronserver.xyz \
  --key-file /etc/nginx/certs/www.luotronserver.xyz.key \
  --fullchain-file /etc/nginx/certs/www.luotronserver.xyz.cer \
  --reloadcmd "systemctl reload nginx"
```

`--reloadcmd` 会被 acme.sh 记忆：**每次续期成功后自动执行**，nginx 无缝加载新证书。

### 5. 配置 nginx

完整配置见 `deploy/nginx/yyb-go.conf.example`，核心内容：

```nginx
server {
    listen 8000 ssl;
    listen [::]:8000 ssl;
    http2 on;
    server_name www.luotronserver.xyz;

    ssl_certificate     /etc/nginx/certs/www.luotronserver.xyz.cer;
    ssl_certificate_key /etc/nginx/certs/www.luotronserver.xyz.key;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_session_cache   shared:SSL:10m;

    client_max_body_size 2m;

    location / {
        proxy_pass http://127.0.0.1:8001;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 70s;
        proxy_send_timeout 70s;
    }

    # 脚本接口与日志 WebSocket（wss://）
    # 注意：不能用 location /scripts/ 前缀，否则 nginx 会把 /scripts 请求 301 到 /scripts/
    location /scripts {
        proxy_pass http://127.0.0.1:8001;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}
```

> nginx 1.25.1+ 中 `listen ... http2` 已弃用，需用独立的 `http2 on;` 指令。

启用：

```bash
cp deploy/nginx/yyb-go.conf.example /etc/nginx/sites-available/yyb-go
ln -sf /etc/nginx/sites-available/yyb-go /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default
nginx -t && systemctl enable --now nginx
```

### 6. 验证

```bash
curl https://www.luotronserver.xyz:8000/health   # 返回 {"code":0,...}
```

浏览器访问 `https://www.luotronserver.xyz:8000`：证书正常、网页/扫码/脚本功能、wss 实时日志全部可用。

## 三、自动续期机制

acme.sh 安装时自动注册 cron 定时任务：

```
25 0,6,12,18 * * * "/root/.acme.sh"/acme.sh --cron --home "/root/.acme.sh" > /dev/null
```

- 每天 4 次检查它管理的全部证书
- 证书进入续期窗口（Let's Encrypt 90 天证书通常剩 30 天时，ARI 可能给出更精确窗口）即自动重新走 DNS-01 签发
- 签发成功后自动执行安装证书时注册的 `--reloadcmd`（`systemctl reload nginx`）
- 日志：`/root/.acme.sh/acme.sh.log`

维护事项：

- **阿里云 AK 长期有效**即可；AK 泄露/轮换后更新：`export Ali_Key=新AK Ali_Secret=新SK && ~/.acme.sh/acme.sh --save`
- 手动立即检查一次续期：`~/.acme.sh/acme.sh --cron`
- 域名列表增删：`--issue`/`--remove` 管理，`~/.acme.sh/acme.sh --list` 查看已管理域名

## 四、常见问题

| 现象 | 原因与处理 |
|---|---|
| `Error adding TXT record` | RAM 用户未授权 `AliyunDNSFullAccess`，或 AK 复制不完整（ID 应为 `LTAI` 开头 20/24 位）。加 `--debug` 查看原始错误。 |
| `cannot load certificate ... No such file` | 证书尚未签发（`--issue` 未执行或失败）。先成功签发再 `--install-cert`。 |
| `the "listen ... http2" directive is deprecated` | nginx 版本较新，改为 `listen 8000 ssl;` + `http2 on;`（仅告警，不影响运行）。 |
| 局域网内访问域名失败 | 公网网关无回流（hairpin NAT），本机 hosts 把域名指向 `192.168.1.63`，或从外网访问。 |
| 证书续期失败（通知邮件） | 检查阿里云 AK 是否被禁用/删除、RAM 用户权限是否被改；`--cron --debug` 查看日志。 |
| wss 连接失败 | nginx 未加 `/scripts/` 的 `Upgrade`/`Connection` 转发配置。 |

## 五、相关文件

```text
deploy/nginx/yyb-go.conf.example    nginx HTTPS/WSS 反向代理配置模板
deploy/acme-aliyun.sh               一键签发/安装证书脚本（填 AK 后执行）
```
