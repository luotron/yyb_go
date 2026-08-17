// 应用列表 + 接口调试面板。面板始终只对当前应用的 base 地址发请求。
(function () {
  const catalog = window.YYB_CATALOG;
  const $ = id => document.getElementById(id);
  const state = { apps: catalog.apps, current: null, activeRoute: null, refs: [], lastText: "" };

  function escapeHTML(value) {
    return String(value ?? "")
      .replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;").replaceAll("'", "&#039;");
  }

  function displayBase(app) {
    return app.base || location.origin;
  }

  function renderApps() {
    const list = $("appList");
    list.innerHTML = "";
    for (const app of state.apps) {
      const card = document.createElement("div");
      card.className = "app-card";
      card.innerHTML =
        '<div class="app-meta">' +
        '<h2 class="app-name">' + escapeHTML(app.name) +
        '<span class="chip">' + escapeHTML(app.module) + "</span>" +
        '<span class="chip" data-health="' + app.id + '">检测中</span></h2>' +
        '<p class="app-desc">' + escapeHTML(app.desc) + "</p>" +
        '<div class="field">' +
        '<label for="base-' + app.id + '">服务地址</label>' +
        '<div class="inline-actions">' +
        '<input class="input mono" id="base-' + app.id + '" style="flex:1;min-width:0" spellcheck="false" autocomplete="off"' +
        ' value="' + escapeHTML(displayBase(app)) + '" readonly>' +
        "</div></div>" +
        '<div class="note">' + app.routes.length + " 个接口 · 与本页面同源，直接调试" +
        "</div></div>" +
        '<div class="app-actions">' +
        '<button class="btn primary" data-debug="' + app.id + '" type="button">调试接口</button>' +
        app.links.map(link =>
          '<a class="btn" target="_blank" rel="noreferrer" href="' +
          escapeHTML((app.base || "") + link.href) + '">' + escapeHTML(link.label) + "</a>").join("") +
        "</div>";
      list.appendChild(card);
    }

    list.querySelectorAll("[data-debug]").forEach(button => {
      button.addEventListener("click", () => openDrawer(findApp(button.dataset.debug)));
    });
    for (const app of state.apps) checkHealth(app);
  }

  function findApp(id) {
    return state.apps.find(app => app.id === id);
  }

  async function checkHealth(app) {
    const chip = document.querySelector('[data-health="' + app.id + '"]');
    if (!chip) return;
    chip.textContent = "检测中";
    chip.classList.remove("ok", "bad");
    try {
      const response = await fetch((app.base || "") + "/health", { method: "GET" });
      chip.textContent = response.ok ? "在线" : "异常 " + response.status;
      chip.classList.add(response.ok ? "ok" : "bad");
    } catch {
      chip.textContent = "无法访问";
      chip.classList.add("bad");
    }
  }

  /* ---------------- 调试面板 ---------------- */

  function openDrawer(app) {
    state.current = app;
    state.activeRoute = null;
    $("drawerTitle").textContent = app.name + " · 接口调试";
    $("drawerBase").textContent = displayBase(app);
    $("refPickerGroup").classList.toggle("hidden", !app.supportsRefPicker);
    $("routeSearch").value = "";
    $("scrim").classList.remove("hidden");
    $("drawer").classList.remove("hidden");
    renderRoutes();
    applyRoute(app.routes[0]);
    if (app.supportsRefPicker) loadRefs();
  }

  function closeDrawer() {
    $("scrim").classList.add("hidden");
    $("drawer").classList.add("hidden");
    state.current = null;
  }

  function renderRoutes() {
    const app = state.current;
    if (!app) return;
    const keyword = $("routeSearch").value.trim().toLowerCase();
    const matched = app.routes.filter(route =>
      !keyword || (route.path + " " + route.note + " " + route.method).toLowerCase().includes(keyword));
    $("routeCount").textContent = matched.length + " / " + app.routes.length + " 个接口";
    const list = $("routeList");
    list.innerHTML = "";
    let group = "";
    for (const route of matched) {
      if (route.group !== group) {
        group = route.group;
        const title = document.createElement("div");
        title.className = "route-group";
        title.textContent = group;
        list.appendChild(title);
      }
      const button = document.createElement("button");
      button.type = "button";
      button.className = "route" + (state.activeRoute === route ? " active" : "");
      button.innerHTML =
        '<span class="route-line"><span class="verb ' + route.method.toLowerCase() + '">' + route.method + "</span>" +
        '<span class="route-path">' + escapeHTML(route.path) + "</span></span>" +
        '<span class="route-note">' + escapeHTML(route.note) + "</span>";
      button.addEventListener("click", () => applyRoute(route));
      list.appendChild(button);
    }
    if (!matched.length) list.innerHTML = '<div class="route-group">没有匹配的接口</div>';
  }

  function applyRoute(route) {
    if (!route) return;
    state.activeRoute = route;
    $("methodSel").value = route.method;
    $("pathInput").value = route.path;
    $("bodyInput").value = route.body ? JSON.stringify(route.body, null, 2) : "";
    $("routeNote").textContent = route.note;
    renderRoutes();
    renderParams();
  }

  function renderParams() {
    const box = $("paramsBox");
    const tokens = [...new Set(($("pathInput").value + " " + $("bodyInput").value).match(/\{[a-zA-Z0-9_]+\}/g) || [])];
    const previous = {};
    box.querySelectorAll("input").forEach(input => { previous[input.dataset.token] = input.value; });
    box.innerHTML = "";
    box.classList.toggle("hidden", !tokens.length);
    for (const token of tokens) {
      const name = token.slice(1, -1);
      const field = document.createElement("div");
      field.className = "field";
      field.innerHTML =
        '<label for="param-' + name + '">占位符 ' + escapeHTML(token) + "</label>" +
        '<input class="input mono" id="param-' + name + '" data-token="' + escapeHTML(token) + '" spellcheck="false" autocomplete="off">';
      box.appendChild(field);
      const input = field.querySelector("input");
      if (previous[token] !== undefined) input.value = previous[token];
      else if (name === "ref" && state.refs.length) input.value = state.refs[0].openid || "";
    }
  }

  function resolveTokens(text) {
    let output = text;
    $("paramsBox").querySelectorAll("input").forEach(input => {
      if (input.value) output = output.replaceAll(input.dataset.token, input.value);
    });
    return output;
  }

  async function loadRefs() {
    const select = $("refSel");
    try {
      const response = await fetch((state.current.base || "") + "/accounts");
      const payload = await response.json();
      const list = Array.isArray(payload) ? payload : (payload.data?.accounts || payload.data || []);
      state.refs = Array.isArray(list) ? list : [];
      select.innerHTML = state.refs.length
        ? state.refs.map(account => {
          const ref = account.openid || account.ref || "";
          const name = account.nickname || account.nick_name || ref;
          return '<option value="' + escapeHTML(ref) + '">' + escapeHTML(name) + " · " + escapeHTML(ref.slice(0, 10)) + "…</option>";
        }).join("")
        : '<option value="">暂无账号</option>';
    } catch (error) {
      select.innerHTML = '<option value="">加载失败：' + escapeHTML(error.message) + "</option>";
    }
  }

  function fillRef() {
    const ref = $("refSel").value;
    if (!ref) return;
    const param = $("paramsBox").querySelector('input[data-token="{ref}"]');
    if (param) {
      param.value = ref;
      return;
    }
    const body = $("bodyInput");
    if (body.value.includes("{ref}")) {
      body.value = body.value.replaceAll("{ref}", ref);
      return;
    }
    try {
      const parsed = JSON.parse(body.value || "{}");
      parsed.ref = ref;
      body.value = JSON.stringify(parsed, null, 2);
    } catch {
      body.value = JSON.stringify({ ref }, null, 2);
    }
  }

  function plan() {
    return {
      method: $("methodSel").value,
      path: resolveTokens($("pathInput").value.trim()) || "/health",
      rawBody: resolveTokens($("bodyInput").value.trim())
    };
  }

  function formatSize(bytes) {
    if (bytes < 1024) return bytes + " B";
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
    return (bytes / 1024 / 1024).toFixed(2) + " MB";
  }

  function setResult(text, meta) {
    state.lastText = text;
    $("resultImage").classList.add("hidden");
    $("resultBox").classList.remove("hidden");
    $("resultBox").textContent = text;
    $("resultMeta").textContent = meta;
  }

  async function send() {
    if (!state.current) return;
    const { method, path, rawBody } = plan();
    const headers = {};
    let body;
    if (rawBody && method !== "GET") {
      try {
        body = JSON.stringify(JSON.parse(rawBody));
      } catch (error) {
        setResult("请求体不是合法 JSON：" + error.message, "未发送");
        return;
      }
      headers["Content-Type"] = "application/json";
    }

    $("sendBtn").disabled = true;
    $("resultMeta").textContent = "请求中…";
    const started = performance.now();
    try {
      const response = await fetch((state.current.base || "") + path, { method, headers, body });
      const elapsed = Math.round(performance.now() - started);
      const type = response.headers.get("content-type") || "";
      const buffer = await response.arrayBuffer();
      $("statusText").textContent = response.status + " " + response.statusText;
      $("timeText").textContent = elapsed + " ms";
      $("sizeText").textContent = formatSize(buffer.byteLength);
      $("resultHeaders").textContent = [...response.headers.entries()]
        .map(([name, value]) => name + ": " + value).join("\n");
      if (type.startsWith("image/")) {
        $("resultBox").classList.add("hidden");
        const image = $("resultImage");
        image.src = URL.createObjectURL(new Blob([buffer], { type }));
        image.classList.remove("hidden");
        state.lastText = "[" + type + " " + buffer.byteLength + " bytes]";
        $("resultMeta").textContent = "图片响应 " + type;
      } else {
        const text = new TextDecoder().decode(buffer);
        let pretty = text;
        try {
          pretty = JSON.stringify(JSON.parse(text), null, 2);
        } catch { /* 非 JSON 原样展示 */ }
        setResult(pretty || "(空响应)", response.ok ? "完成" : "服务端返回 " + response.status);
      }
    } catch (error) {
      $("statusText").textContent = "网络错误";
      $("timeText").textContent = Math.round(performance.now() - started) + " ms";
      $("sizeText").textContent = "—";
      setResult(String(error), "请求失败");
    } finally {
      $("sendBtn").disabled = false;
    }
  }

  function curlCommand() {
    const { method, path, rawBody } = plan();
    const parts = ["curl -X " + method + " '" + (state.current.base || location.origin) + path + "'"];
    if (rawBody && method !== "GET") {
      parts.push("-H 'Content-Type: application/json'");
      parts.push("-d '" + rawBody.replaceAll("'", "'\\''") + "'");
    }
    return parts.join(" \\\n  ");
  }

  async function copyText(text, button) {
    const label = button.textContent;
    try {
      await navigator.clipboard.writeText(text);
      button.textContent = "已复制";
    } catch {
      button.textContent = "复制失败";
    }
    setTimeout(() => { button.textContent = label; }, 1200);
  }

  $("closeDrawerBtn").addEventListener("click", closeDrawer);
  $("scrim").addEventListener("click", closeDrawer);
  $("routeSearch").addEventListener("input", renderRoutes);
  $("pathInput").addEventListener("input", renderParams);
  $("bodyInput").addEventListener("input", renderParams);
  $("pathInput").addEventListener("keydown", event => { if (event.key === "Enter") send(); });
  $("sendBtn").addEventListener("click", send);
  $("fillRefBtn").addEventListener("click", fillRef);
  $("reloadRefBtn").addEventListener("click", loadRefs);
  $("formatBtn").addEventListener("click", () => {
    const body = $("bodyInput");
    if (!body.value.trim()) return;
    try {
      body.value = JSON.stringify(JSON.parse(body.value), null, 2);
    } catch (error) {
      setResult("格式化失败：" + error.message, "未发送");
    }
  });
  $("clearBodyBtn").addEventListener("click", () => { $("bodyInput").value = ""; renderParams(); });
  $("copyCurlBtn").addEventListener("click", event => copyText(curlCommand(), event.currentTarget));
  $("copyResultBtn").addEventListener("click", event => copyText(state.lastText || "", event.currentTarget));
  document.addEventListener("keydown", event => {
    if (event.key === "Escape" && state.current) closeDrawer();
  });

  renderApps();

  const wanted = new URLSearchParams(location.search).get("app");
  if (wanted && findApp(wanted)) openDrawer(findApp(wanted));
})();
