# 本地账号服务 API

默认地址：`http://127.0.0.1:8000`。本项目仅包含二维码、本地账号和本地小程序调用；聚合及租约接口位于独立聚合项目。

## 一、通用约定

### 1.1 认证

配置 `admin_user` 与 `admin_password`（`config/service.json`）后启用管理员登录，除下列公开路由外，所有接口与页面均需登录：

- 公开：`GET/POST /login`、`POST /logout`、`GET /health`、`GET /ready`、`/docs/*`、`/openapi.json`、`/static/*`
- 未登录访问 API（`/accounts`、`/wxapp/*`、`/scripts/*`、`/qr/*`、`/activity/*`、`/features`、`/auth/me`）返回 HTTP `401` `{"msg":"请先登录"}`
- 未登录访问页面（`/`、`/scan`、`/apps`）重定向到 `/login?next=<原路径>`

登录成功后返回会话 Cookie（`yyb_session`，HttpOnly，默认有效期 `session_duration_minutes`=1440 分钟）：

```bash
curl -k -c cookies.txt -X POST https://127.0.0.1:8000/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123456","next":"/apps"}'
curl -k -b cookies.txt https://127.0.0.1:8000/accounts
```

- `GET /auth/me`：返回当前登录用户（`{user, session_id}`）；未启用鉴权时返回 `auth_enabled:false`
- `POST /logout`：销毁会话并清除 Cookie
- 连续 8 次登录失败后同一 IP 锁定 15 分钟

账号密码直接配置在 `service.json` 的 `admin_user`/`admin_password` 中，修改后重启生效；登录会话保存在服务内存中，重启服务后所有会话失效需重新登录。

### 1.2 自动化调用（integration_token）

配置 `integration_token` 后，自动化脚本（Python SDK/青龙等）可免登录调用 API——请求携带请求头 `X-Integration-Token: <token>` 即视为管理员。本服务运行用户脚本时会自动注入环境变量 `YYB_INTEGRATION_TOKEN`，SDK 会自动附加该头。

### 1.3 成功响应

除二维码图片、Swagger 静态内容和原始 OpenAPI JSON 外，成功响应为：

```json
{
  "code": 0,
  "msg": "success",
  "data": {}
}
```

### 1.4 失败响应

```json
{
  "code": 400,
  "msg": "错误说明",
  "data": null
}
```

常见状态：`400` 参数错误，`401` 未登录，`404` 资源不存在，`409` 本地账号状态冲突，`502` 上游协议失败。

### 1.5 本地账号 ref

需要 `ref` 的接口接受以下任一值：

- 本地数据库数字 ID；
- 数字 UIN；
- openid。

## 二、状态与文档

### 2.1 `GET /health`

返回进程状态。该接口不连接上游。

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "ok": true
  }
}
```

### 2.2 `GET /ready`

进程可服务时返回 HTTP `200`：

```json
{
  "code": 0,
  "msg": "success",
  "data": {
    "ready": true
  }
}
```

响应中不再包含聚合同步字段。

### 2.3 `GET /openapi.json`

返回原始 OpenAPI 3.0.3 对象，不使用统一响应外壳。

### 2.4 `GET /docs/index.html`

返回 Swagger UI；`GET /docs` 会永久重定向到该路径。

## 三、二维码登录

### 3.1 `POST /qr`

创建扫码会话。可选查询参数 `as_base64=true` 用于同时返回二维码 data URI。

响应 `data` 包含 `session_id`、`status`、`image_url`，按需包含 `image_base64`。

### 3.2 `GET /qr/{session_id}/image`

返回 `image/png` 二维码图片，不使用 JSON 外壳。

### 3.3 `GET /qr/{session_id}/poll`

轮询状态。常见值：`pending`、`scanned`、`authorized`、`confirmed`、`expired`、`cancelled`、`unknown`。

### 3.4 `POST /qr/{session_id}/confirm`

确认已授权会话、写入本地账号数据库并尝试同步资料。

## 四、本地账号

### 4.1 `GET /accounts`

- 不带 `ref`：列出本地账号。
- 带 `?ref=...`：返回单个本地账号。

公开响应不返回登录缓冲、凭据或协议会话。

### 4.2 `DELETE /accounts?ref=...`

删除本地账号及其关联会话。

### 4.3 `GET /accounts/avatar?ref=...`

返回本地或远程头像内容。无法读取时返回对应错误状态。

### 4.4 `POST /accounts/refresh`

请求体：

```json
{"ref":"ACCOUNT_REF"}
```

检查账号状态并更新本地记录。

### 4.5 `POST /accounts/resync`

请求体同上，用当前凭据重新同步昵称、头像和用户资料。

## 五、本地小程序调用

四个 `/wxapp/*` 接口都从本地数据库解析账号，复用本地协议会话。

### 5.1 `POST /wxapp/getCode`

```json
{
  "ref": "ACCOUNT_REF",
  "app_id": "APP_ID"
}
```

### 5.2 `POST /wxapp/getPhoneNumber`

```json
{
  "ref": "ACCOUNT_REF",
  "app_id": "APP_ID",
  "payload": {}
}
```

### 5.3 `POST /wxapp/operateWxData`

```json
{
  "ref": "ACCOUNT_REF",
  "app_id": "APP_ID",
  "payload": {}
}
```

`payload` 按目标小程序业务原样传入协议层。上游响应放在成功响应的 `data.result` 中。

### 5.4 `POST /wxapp/getHostSign`

获取微信 `verifyPlugin` HostSign。目标 CGI 为 `/cgi-bin/mmbiz-bin/wxaapp/verifyplugin`，命令号为 `1714`。

```json
{
  "ref": "ACCOUNT_REF",
  "app_id": "HOST_APP_ID",
  "payload": {
    "provider": "PLUGIN_APP_ID",
    "inner_version": 20
  }
}
```

`payload` 还支持：

```json
{"plugins":[{"provider":"PLUGIN_APP_ID","inner_version":20}]}
```

或原始 `data`：

```json
{"data":"{\"plugins\":[{\"provider\":\"PLUGIN_APP_ID\",\"inner_version\":20}]}"}
```

返回 `host_sign`、`noncestr`、`timestamp` 和插件域名配置；前三项对应小程序请求头 `X-WECHAT-HOSTSIGN` 中的 `signature`、`noncestr`、`timestamp`。

## 六、统一功能调用与活动签名

### 6.1 `GET /features`

返回固定功能定义：`getCode`、`getPhoneNumber`、`operateWxData`、`getHostSign`。功能表不写入 SQLite。

### 6.2 `POST /accounts/{ref}/call`

兼容统一调用形式，`feature` 支持功能名或编号：

```json
{
  "feature": "getHostSign",
  "app_id": "HOST_APP_ID",
  "payload": {
    "provider": "PLUGIN_APP_ID",
    "inner_version": 20
  }
}
```

`getHostSign` 的编号为 `1004`。

### 6.3 `POST /activity/sign`

生成活动业务请求体中的 `signTimestamp`、`signNonce` 和 `sign`。它与 `getHostSign` 是两套独立机制。

生成新签名：

```json
{
  "sign_key": "ACTIVITY_SIGN_KEY",
  "token": "DT_USER_TOKEN"
}
```

固定时间戳与 nonce 复算：

```json
{
  "sign_key": "ACTIVITY_SIGN_KEY",
  "sign_timestamp": "TIMESTAMP_MS",
  "sign_nonce": "NONCE"
}
```

算法：

```text
seed       = 16 位 base36 随机串
signNonce  = HMAC-SHA256(key=signTimestamp, message=seed + token).hex[0:16]
raw        = "timestamp=" + signTimestamp + "&nonce=" + signNonce
sign       = HMAC-SHA256(key=signKey, message=raw).hex
```

其中 `signKey` 来自活动配置；`encrypt_baseinfo.txt` 使用 `ENC_V1` 格式，正文为 `IV(16) + AES-CBC/PKCS7 密文`。

## 七、不提供的路由

本服务不注册以下路径，访问时返回 HTTP `404`：

- `/aggregator/*`
- `/client/*`

号池聚合、账号租约和黑名单不属于本项目范围。

## 八、用户脚本

用户开发的 `.py` 脚本放在 `data_root/scripts/` 目录（网页 `/apps` 的「用户脚本」面板可上传/运行/定时/查看日志）。

### 8.1 运行环境

脚本运行时自动注入环境变量：

- `YYB_SERVER` / `YYB_BASE_URL`：本地服务地址（`listen_address` 映射到 127.0.0.1）
- `PYTHONPATH`：`asset_root/scripts/sdk`（内含 `yyb_sdk.py`，脚本内 `import yyb_sdk` 即可使用）
- `YYB_SCRIPT_NAME`、`YYB_SCRIPTS_DIR`：脚本名与脚本目录
- `PYTHONUTF8=1`、`PYTHONIOENCODING=utf-8`、`PYTHONUNBUFFERED=1`：输出按 UTF-8 编码且无缓冲，保证日志实时推送（脚本内无需手动 flush）

Python 解释器由 `python_command` 配置（默认 `python`），需自行安装并确保在 PATH 中。

### 8.2 `GET /scripts`

返回脚本列表与运行器信息：

```json
{
  "scripts": [
    {
      "name": "demo.py", "size": 2048, "updated_at": 1787983442,
      "running": false, "exit_code": 0,
      "schedule": "32 16 * * *", "next_run_at": 1788019320
    }
  ],
  "dir": "resource/scripts",
  "sdk_dir": "resource/scripts/sdk",
  "server_url": "http://127.0.0.1:8000",
  "python": "C:\\...\\python.exe",
  "python_ok": true
}
```

### 8.3 `POST /scripts/upload`

multipart 上传，字段 `file`，仅接受合法 `*.py` 文件名（≤1 MiB）。同名冲突返回 `409`，加 `?overwrite=1` 覆盖。

### 8.4 运行与停止

- `POST /scripts/{name}/run`：立即运行；已在运行返回 `409`。
- `POST /scripts/{name}/stop`：终止运行。

### 8.5 日志

- `GET /scripts/{name}/logs?limit=262144` 返回日志尾部（最大 256 KiB）与运行状态：

```json
{"name":"demo.py","content":"...","running":false,"exit_code":0,"last_error":""}
```

- `GET /scripts/{name}/logs/ws`（WebSocket）实时推送日志，网页日志窗口即走该通道。服务端发送 JSON 文本帧：

```json
{"type":"init","content":"当前日志全文（客户端整体替换）"}
{"type":"log","data":"增量日志（追加）"}
{"type":"status","running":false,"started_at":1787984419,"finished_at":1787984419,"exit_code":0,"last_error":""}
```

连接后先收到 `init` + 当前 `status`，运行中持续收到 `log` 增量；脚本结束后发送最终 `status` 并由服务端关闭连接。经 nginx 反代时需配置 `Upgrade`/`Connection` 头转发。

### 8.6 定时

- `PUT /scripts/{name}/schedule`，请求体 `{"cron": "32 16 * * *"}`（分 时 日 月 周，支持 `*` `,` `-` `/`）
- `DELETE /scripts/{name}/schedule` 取消

调度器按最近触发时刻精确计时，到点立即执行（毫秒级误差，无轮询延迟）。定时任务持久化在 `data_root/scripts/schedules.json`。日与周字段同时受限时按“或”匹配。

### 8.7 删除

`DELETE /scripts/{name}` 删除脚本、日志与定时任务。