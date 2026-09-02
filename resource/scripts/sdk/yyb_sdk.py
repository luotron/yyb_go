#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
YYB Go Python SDK

将本地 YYB Go 服务的 HTTP API（应用宝协议开放能力）封装为 Python 类，
供 scripts 目录下各业务脚本复用。

能力映射：
    /accounts              -> YYBClient.accounts()
    /accounts?ref=...      -> YYBClient.account()
    /accounts (DELETE)     -> YYBClient.delete_account()
    /accounts/avatar       -> YYBClient.avatar_url()
    /accounts/refresh      -> YYBClient.refresh()
    /accounts/resync       -> YYBClient.resync()
    /wxapp/getCode         -> YYBClient.get_code()
    /wxapp/getPhoneNumber  -> YYBClient.get_phone_number()
    /wxapp/operateWxData   -> YYBClient.operate_wx_data()
    /wxapp/getHostSign     -> YYBClient.get_host_sign()
    /activity/sign         -> YYBClient.activity_sign()

环境变量（沿用上游脚本约定）：
    YYB_SERVER  必填，格式 "服务地址@账号标识"，多账号换行分隔，例如：
        192.168.31.36:8088@1
        192.168.31.88:8088@2
    也可用 accounts_from_env() 直接解析成 YYBClient 列表。

依赖：pip install requests
"""

from __future__ import annotations

import os
from typing import Any, Optional

import requests


class YYBError(RuntimeError):
    """服务返回非 0 code 或网络异常。"""


class YYBClient:
    def __init__(
        self,
        base_url: str,
        ref: Optional[str] = None,
        timeout: float = 20,
        session: Optional[requests.Session] = None,
    ) -> None:
        base = base_url.strip().rstrip("/")
        if not base.startswith("http://") and not base.startswith("https://"):
            base = "http://" + base
        self.base_url = base
        self.ref = ref
        self.timeout = timeout
        self.session = session or requests.Session()
        # 服务端配置 secret_key 时，SDK 自动携带请求头免登录调用 API
        self.secret_key = (os.getenv("YYB_SECRET_KEY") or "").strip()

    # ---------- 通用请求 ----------

    def _request(
        self,
        method: str,
        path: str,
        json_body: Any = None,
        params: Optional[dict] = None,
    ) -> dict:
        url = self.base_url + path
        headers = {}
        if self.secret_key:
            headers["X-Secret-Key"] = self.secret_key
        try:
            resp = self.session.request(
                method,
                url,
                json=json_body,
                params=params,
                headers=headers,
                timeout=self.timeout,
            )
        except requests.RequestException as exc:
            raise YYBError(f"请求 {url} 失败: {exc}") from exc
        try:
            body = resp.json()
        except ValueError:
            raise YYBError(f"{url} 返回非 JSON: HTTP {resp.status_code}") from None
        if resp.status_code >= 400:
            raise YYBError(
                f"{url} HTTP {resp.status_code}: "
                f"{body.get('msg', '') if isinstance(body, dict) else body}"
            )
        if isinstance(body, dict) and body.get("code") not in (0, None):
            raise YYBError(f"{url} 业务失败: {body.get('msg', '')}")
        return body.get("data", body) if isinstance(body, dict) else body

    # ---------- 本地账号 ----------

    def accounts(self) -> list:
        """GET /accounts 列出本地账号。"""
        return self._request("GET", "/accounts")

    def account(self) -> list:
        """GET /accounts 列出本地账号（服务端始终返回列表，ref 参数保留仅为兼容）。"""
        return self.accounts()

    def delete_account(self, ref: Optional[str] = None) -> dict:
        """DELETE /accounts?ref=... 删除账号（后端强制要求 ref）。"""
        return self._request("DELETE", "/accounts", params={"ref": self._need_ref(ref)})

    def avatar_url(self, ref: Optional[str] = None) -> str:
        return f"{self.base_url}/accounts/avatar?ref={self._need_ref(ref)}"

    def refresh(self) -> dict:
        return self._request("POST", "/accounts/refresh")

    def resync(self, ref: Optional[str] = None) -> dict:
        return self._request("POST", "/accounts/resync", params={"ref": self._need_ref(ref)})

    # ---------- 小程序调用（应用宝协议） ----------

    def get_code(self, app_id: str, ref: Optional[str] = None) -> dict:
        """POST /wxapp/getCode 获取小程序 wx.login code。"""
        return self._request(
            "POST", "/wxapp/getCode", {"ref": self._need_ref(ref), "app_id": app_id}
        )

    def get_phone_number(self, app_id: str, ref: Optional[str] = None) -> dict:
        """POST /wxapp/getPhoneNumber 获取手机号（encryptedData/iv 等）。"""
        return self._request(
            "POST",
            "/wxapp/getPhoneNumber",
            {"ref": self._need_ref(ref), "app_id": app_id, "payload": {}},
        )

    def operate_wx_data(
        self, app_id: str, payload: dict, ref: Optional[str] = None
    ) -> dict:
        """POST /wxapp/operateWxData 透传小程序业务请求，结果在 data.result。"""
        return self._request(
            "POST",
            "/wxapp/operateWxData",
            {"ref": self._need_ref(ref), "app_id": app_id, "payload": payload},
        )

    def get_host_sign(self, app_id: str, payload: dict, ref: Optional[str] = None) -> dict:
        """POST /wxapp/getHostSign 获取微信 verifyPlugin HostSign。"""
        return self._request(
            "POST",
            "/wxapp/getHostSign",
            {"ref": self._need_ref(ref), "app_id": app_id, "payload": payload},
        )

    # ---------- 活动签名 ----------

    def activity_sign(
        self,
        sign_key: str,
        token: Optional[str] = None,
        sign_timestamp: Optional[str] = None,
        sign_nonce: Optional[str] = None,
    ) -> dict:
        """POST /activity/sign 生成活动业务请求体的签名。

        新签名传 sign_key + token；复算传 sign_key + sign_timestamp + sign_nonce。
        """
        body: dict[str, Any] = {"sign_key": sign_key}
        if token:
            body["token"] = token
        if sign_timestamp:
            body["sign_timestamp"] = sign_timestamp
        if sign_nonce:
            body["sign_nonce"] = sign_nonce
        return self._request("POST", "/activity/sign", body)

    # ---------- 工具 ----------

    def _need_ref(self, ref: Optional[str] = None) -> str:
        ref = (ref or "").strip() or (self.ref or "").strip()
        if not ref:
            raise YYBError("未指定账号 ref，请通过函数参数传入")
        return ref


def parse_yyb_entry(raw: str) -> Optional[dict]:
    """解析 "地址@账号标识" 一行，返回 {"server": ..., "ref": ...}。"""
    value = raw.strip()
    if not value or "@" not in value:
        return None
    server, ref = value.split("@", 1)
    server = server.strip().rstrip("/")
    ref = ref.strip()
    if not server or not ref:
        return None
    return {"server": server, "ref": ref}


def accounts_from_env(env_name: str = "YYB_SERVER", timeout: float = 20) -> list[YYBClient]:
    """从环境变量解析多账号，每行 "服务地址@账号标识"。"""
    raw = os.getenv(env_name, "")
    if not raw:
        raise YYBError(
            f"未配置环境变量 {env_name}，格式：服务地址@账号标识，多账号换行分隔"
        )
    clients: list[YYBClient] = []
    for line in raw.splitlines():
        entry = parse_yyb_entry(line)
        if not entry:
            continue
        clients.append(
            YYBClient(entry["server"], ref=entry["ref"], timeout=timeout)
        )
    if not clients:
        raise YYBError(f"{env_name} 无有效账号，格式：服务地址@账号标识")
    return clients
