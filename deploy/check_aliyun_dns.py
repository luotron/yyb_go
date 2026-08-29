#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
校验 config/service.json 中的阿里云 DNS AccessKey 是否有效。

在项目根目录运行（或传 service.json 路径作为参数）：
    python3 check_aliyun_dns.py
    python3 check_aliyun_dns.py /opt/yyb_go/config/service.json

依赖：仅 Python 标准库。
"""

import base64
import hashlib
import hmac
import json
import random
import sys
import urllib.parse
import urllib.request
from datetime import datetime, timezone


def percent_encode(value: str) -> str:
    return urllib.parse.quote(str(value), safe="~-._")


def call_alidns(action: str, params: dict, ak: str, sk: str) -> dict:
    params = dict(params)
    params.update({
        "Action": action,
        "Format": "JSON",
        "Version": "2015-01-09",
        "AccessKeyId": ak,
        "SignatureMethod": "HMAC-SHA1",
        "SignatureVersion": "1.0",
        "SignatureNonce": str(random.randint(1000000000, 9999999999)),
        "Timestamp": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    })
    canonical = "&".join(
        f"{percent_encode(key)}={percent_encode(params[key])}" for key in sorted(params)
    )
    string_to_sign = "GET&%2F&" + percent_encode(canonical)
    signature = base64.b64encode(
        hmac.new((sk + "&").encode(), string_to_sign.encode(), hashlib.sha1).digest()
    ).decode()
    url = "https://alidns.aliyuncs.com/?" + canonical + "&Signature=" + percent_encode(signature)
    with urllib.request.urlopen(url, timeout=15) as resp:
        return json.loads(resp.read().decode())


def main() -> int:
    path = sys.argv[1] if len(sys.argv) > 1 else "config/service.json"
    with open(path, encoding="utf-8") as f:
        cfg = json.load(f)
    ak = (cfg.get("tls_dns_access_key_id") or "").strip()
    sk = (cfg.get("tls_dns_access_key_secret") or "").strip()
    print("配置文件:", path)
    if not ak or not sk:
        print("!! 配置中 AccessKey 为空")
        return 1
    masked = ak[:6] + "***" + ak[-4:] if len(ak) > 12 else "***"
    print("使用 AccessKeyId:", masked, f"(长度 {len(ak)})")
    try:
        result = call_alidns("DescribeDomainRecords", {"DomainName": "luotronserver.xyz"}, ak, sk)
        print(json.dumps(result, ensure_ascii=False, indent=2)[:800])
        if "DomainRecords" in result:
            print("==> AccessKey 有效，可以正常调用阿里云 DNS API")
            return 0
        print("==> 调用失败，见上方返回的 Code/Message")
        return 1
    except Exception as exc:
        print("请求异常:", exc)
        return 1


if __name__ == "__main__":
    sys.exit(main())
