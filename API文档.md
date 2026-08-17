# 本地账号服务 API

默认地址：`http://127.0.0.1:8000`。本项目仅包含二维码、本地账号和本地小程序调用；聚合及租约接口位于独立聚合项目。

## 一、通用约定

### 1.1 认证

本服务不做鉴权，所有路由均可直接调用（请自行限制监听地址或在前置网关上做访问控制）。

### 1.2 成功响应

除二维码图片、Swagger 静态内容和原始 OpenAPI JSON 外，成功响应为：

```json
{
  "code": 0,
  "msg": "success",
  "data": {}
}
```

### 1.3 失败响应

```json
{
  "code": 400,
  "msg": "错误说明",
  "data": null
}
```

常见状态：`400` 参数错误，`404` 资源不存在，`409` 本地账号状态冲突，`502` 上游协议失败。

### 1.4 本地账号 ref

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