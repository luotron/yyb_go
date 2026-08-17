// 本地服务（yyb_go）的接口目录。面板只按这里的定义发请求。
(function () {
  const LOCAL_ROUTES = [
    { group: "状态", method: "GET", path: "/health", note: "进程状态" },
    { group: "状态", method: "GET", path: "/ready", note: "就绪检查" },
    { group: "状态", method: "GET", path: "/openapi.json", note: "原始 OpenAPI 3.0.3" },
    { group: "二维码登录", method: "POST", path: "/qr?as_base64=true", note: "创建扫码会话" },
    { group: "二维码登录", method: "GET", path: "/qr/{session_id}/image", note: "二维码图片 image/jpeg" },
    { group: "二维码登录", method: "GET", path: "/qr/{session_id}/poll", note: "轮询扫码状态" },
    { group: "二维码登录", method: "POST", path: "/qr/{session_id}/confirm", note: "确认授权并入库" },
    { group: "账号", method: "GET", path: "/accounts", note: "账号列表" },
    { group: "账号", method: "GET", path: "/accounts?ref={ref}", note: "单个账号" },
    { group: "账号", method: "DELETE", path: "/accounts?ref={ref}", note: "删除账号及会话" },
    { group: "账号", method: "GET", path: "/accounts/avatar?ref={ref}", note: "头像内容" },
    { group: "账号", method: "POST", path: "/accounts/refresh", note: "刷新存活，留空刷新全部", body: { ref: "" } },
    { group: "账号", method: "POST", path: "/accounts/resync", note: "同步资料，留空同步全部", body: { ref: "" } },
    { group: "统一调用", method: "GET", path: "/features", note: "功能定义列表（GET/POST 均可）" },
    {
      group: "统一调用", method: "POST", path: "/accounts/{ref}/call", note: "按 feature 名或编号调用",
      body: { feature: "getCode", app_id: "wx141bfb9b73c970a9", payload: {} }
    },
    {
      group: "统一调用", method: "POST", path: "/activity/sign", note: "生成活动 sign / signNonce",
      body: { sign_key: "ACTIVITY_SIGN_KEY", token: "DT_USER_TOKEN" }
    },
    {
      group: "小程序能力", method: "POST", path: "/wxapp/getCode", note: "获取 wx.login code",
      body: { ref: "{ref}", app_id: "wx141bfb9b73c970a9" }
    },
    {
      group: "小程序能力", method: "POST", path: "/wxapp/getPhoneNumber", note: "获取手机号",
      body: { ref: "{ref}", app_id: "wx141bfb9b73c970a9", payload: {} }
    },
    {
      group: "小程序能力", method: "POST", path: "/wxapp/operateWxData", note: "调用云业务",
      body: { ref: "{ref}", app_id: "wx141bfb9b73c970a9", payload: { api_name: "getUserInfo", data: {}, env: 1 } }
    },
    {
      group: "小程序能力", method: "POST", path: "/wxapp/getHostSign", note: "获取插件 HostSign",
      body: { ref: "{ref}", app_id: "wx141bfb9b73c970a9", payload: { provider: "wxc3b909c3d24c5417", inner_version: 20 } }
    }
  ];

  window.YYB_CATALOG = {
    apps: [
      {
        id: "local",
        name: "本地服务",
        module: "yyb_go",
        desc: "扫码入池、本地账号管理、小程序能力直调。默认监听 :8000。",
        base: "",
        supportsRefPicker: true,
        routes: LOCAL_ROUTES,
        links: [
          { label: "控制台", href: "/" },
          { label: "扫码添加", href: "/scan" },
          { label: "Swagger", href: "/docs/index.html" }
        ]
      }
    ]
  };
})();
