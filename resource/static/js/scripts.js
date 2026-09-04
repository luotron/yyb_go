// 用户脚本面板：列表、运行/停止、cron 定时、上传删除、运行时日志。
(function () {
  const $ = id => document.getElementById(id);
  const state = { scripts: [], meta: null, selected: null, follow: true, ws: null, listTimer: null };

  function escapeHTML(value) {
    return String(value ?? "")
      .replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;").replaceAll("'", "&#039;");
  }

  function formatSize(bytes) {
    if (bytes < 1024) return bytes + " B";
    return (bytes / 1024).toFixed(1) + " KB";
  }

  function formatTime(unix) {
    if (!unix) return "—";
    const date = new Date(unix * 1000);
    const pad = value => String(value).padStart(2, "0");
    return date.getFullYear() + "-" + pad(date.getMonth() + 1) + "-" + pad(date.getDate()) +
      " " + pad(date.getHours()) + ":" + pad(date.getMinutes());
  }

  function argsKey(name) {
    return "yyb.scriptArgs." + name;
  }

  function loadArgs(name) {
    const input = $("scriptArgsInput");
    if (!input) return "";
    try {
      return localStorage.getItem(argsKey(name)) || "";
    } catch {
      return input.value || "";
    }
  }

  function saveArgs(name, value) {
    try {
      if (value) {
        localStorage.setItem(argsKey(name), value);
      } else {
        localStorage.removeItem(argsKey(name));
      }
    } catch {
      /* 隐私模式等场景忽略 */
    }
  }

  async function api(method, path, body) {
    const options = { method };
    if (body !== undefined) {
      options.headers = { "Content-Type": "application/json" };
      options.body = JSON.stringify(body);
    }
    const response = await fetch(path, options);
    const payload = await response.json().catch(() => ({}));
    if (!response.ok || payload.code) {
      throw new Error(payload.msg || ("HTTP " + response.status));
    }
    return payload.data;
  }

  /* ---------------- 列表 ---------------- */

  async function loadScripts() {
    try {
      const data = await api("GET", "/scripts");
      state.meta = data;
      state.scripts = data.scripts || [];
      renderMeta();
      renderList();
    } catch (error) {
      state.scripts = [];
      renderMeta(error.message);
      renderList();
    }
  }

  function renderMeta(errorMessage) {
    const meta = $("scriptsMeta");
    if (errorMessage) {
      meta.textContent = "脚本服务不可用：" + errorMessage;
      return;
    }
    const parts = ["目录 " + state.meta.dir];
    if (state.meta.python_ok) {
      parts.push("Python " + state.meta.python);
    } else {
      parts.push("未找到 Python，运行功能不可用");
    }
    parts.push("脚本运行时自动注入 YYB_SERVER=" + state.meta.server_url);
    meta.textContent = parts.join(" · ");
  }

  function renderList() {
    $("scriptCount").textContent = state.scripts.length + " 个脚本";
    const list = $("scriptList");
    list.innerHTML = "";
    if (!state.scripts.length) {
      list.innerHTML = '<div class="route-note">暂无脚本。上传 .py 或放入 ' +
        escapeHTML(state.meta ? state.meta.dir : "resource/scripts") + " 目录。</div>";
      return;
    }
    for (const script of state.scripts) {
      const row = document.createElement("div");
      row.className = "script-row" + (state.selected === script.name ? " selected" : "");
      const statusChip = script.running
        ? '<span class="chip ok">运行中</span>'
        : (script.exit_code !== undefined && script.exit_code !== null
          ? '<span class="chip ' + (script.exit_code === 0 ? "ok" : "bad") + '">上次退出 ' + script.exit_code + "</span>"
          : '<span class="chip">空闲</span>');
      const scheduleText = script.schedule
        ? escapeHTML(script.schedule) + " · 下次 " + formatTime(script.next_run_at)
        : "未定时";
      row.innerHTML =
        '<div class="script-row-head">' +
        '<span class="script-name">' + escapeHTML(script.name) + "</span>" + statusChip +
        "</div>" +
        '<div class="script-info">' +
        '<div>大小 ' + formatSize(script.size) + " · 更新 " + formatTime(script.updated_at) + "</div>" +
        "<div>定时 " + scheduleText + "</div>" +
        "</div>" +
        '<div class="script-actions">' +
        '<button class="mini-btn primary" data-act="run">运行</button>' +
        '<button class="mini-btn" data-act="stop"' + (script.running ? "" : " disabled") + ">停止</button>" +
        '<button class="mini-btn" data-act="logs">日志</button>' +
        '<button class="mini-btn" data-act="schedule">定时</button>' +
        (script.schedule ? '<button class="mini-btn" data-act="unschedule">取消定时</button>' : "") +
        '<button class="mini-btn" data-act="delete">删除</button>' +
        "</div>";
      row.addEventListener("click", event => {
        if (event.target.closest("[data-act]")) return;
        selectScript(script.name);
      });
      row.querySelectorAll("[data-act]").forEach(button => {
        button.addEventListener("click", () => actScript(button.dataset.act, script.name));
      });
      list.appendChild(row);
    }
  }

  async function actScript(action, name) {
    try {
      switch (action) {
        case "run": {
          const args = ($("scriptArgsInput").value || "").trim();
          await api("POST", "/scripts/" + encodeURIComponent(name) + "/run",
            args ? { args: args } : {});
          saveArgs(name, args);
          selectScript(name);
          break;
        }
        case "stop":
          await api("POST", "/scripts/" + encodeURIComponent(name) + "/stop");
          break;
        case "logs":
          selectScript(name);
          break;
        case "schedule": {
          const current = state.scripts.find(item => item.name === name);
          const cron = prompt("cron 表达式（分 时 日 月 周）", (current && current.schedule) || "32 16 * * *");
          if (cron === null) return;
          await api("PUT", "/scripts/" + encodeURIComponent(name) + "/schedule", { cron: cron.trim() });
          break;
        }
        case "unschedule":
          await api("DELETE", "/scripts/" + encodeURIComponent(name) + "/schedule");
          break;
        case "delete":
          if (!confirm("确认删除脚本 " + name + " ？")) return;
          await api("DELETE", "/scripts/" + encodeURIComponent(name));
          if (state.selected === name) clearLogView();
          break;
      }
    } catch (error) {
      alert(name + "：" + error.message);
    }
    loadScripts();
  }

  /* ---------------- 日志（WebSocket 实时流） ---------------- */

  function selectScript(name) {
    state.selected = name;
    state.follow = true;
    $("scriptArgsInput").value = loadArgs(name);
    $("logFollowBtn").classList.add("active");
    $("logTitle").textContent = name;
    $("logMeta").textContent = "";
    connectLogStream(name);
    renderList();
  }

  function clearLogView() {
    state.selected = null;
    $("logTitle").textContent = "运行日志";
    $("logMeta").textContent = "选择一个脚本查看运行日志";
    $("scriptLogBox").textContent = "选择左侧脚本查看日志。";
    closeLogStream();
  }

  function wsURL(name) {
    const protocol = location.protocol === "https:" ? "wss://" : "ws://";
    return protocol + location.host + "/scripts/" + encodeURIComponent(name) + "/logs/ws";
  }

  function connectLogStream(name) {
    closeLogStream();
    let socket;
    try {
      socket = new WebSocket(wsURL(name));
    } catch (error) {
      $("logMeta").textContent = "WebSocket 不可用：" + error.message;
      return;
    }
    state.ws = socket;

    socket.onopen = () => {
      if (!state.follow) closeLogStream();
    };
    socket.onmessage = event => {
      let message;
      try {
        message = JSON.parse(event.data);
      } catch {
        return;
      }
      if (message.type === "init") {
        $("scriptLogBox").textContent = message.content || "";
      } else if (message.type === "log") {
        $("scriptLogBox").textContent += message.data;
      } else if (message.type === "status") {
        renderLogMeta(message);
      } else if (message.type === "error") {
        $("logMeta").textContent = message.message;
      }
      $("scriptLogBox").scrollTop = $("scriptLogBox").scrollHeight;
    };
    socket.onclose = () => {
      state.ws = null;
      if (state.selected) loadScripts();
    };
    socket.onerror = () => {
      if (state.ws) state.ws.close();
    };
  }

  function closeLogStream() {
    if (state.ws) {
      const socket = state.ws;
      state.ws = null;
      socket.close();
    }
  }

  function renderLogMeta(status) {
    if (status.running) {
      $("logMeta").textContent = "运行中";
      return;
    }
    if (status.exit_code !== undefined && status.exit_code !== null) {
      $("logMeta").textContent = "退出码 " + status.exit_code +
        (status.last_error ? " · " + status.last_error : "");
      return;
    }
    $("logMeta").textContent = status.last_error || "";
  }

  /* ---------------- 上传 ---------------- */

  async function uploadScript(file, overwrite) {
    const form = new FormData();
    form.append("file", file);
    const query = overwrite ? "?overwrite=1" : "";
    const response = await fetch("/scripts/upload" + query, { method: "POST", body: form });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok || payload.code) {
      if (response.status === 409 && !overwrite && confirm("脚本已存在，是否覆盖？")) {
        return uploadScript(file, true);
      }
      throw new Error(payload.msg || ("HTTP " + response.status));
    }
    return payload.data;
  }

  /* ---------------- 事件 ---------------- */

  $("scriptReloadBtn").addEventListener("click", loadScripts);
  $("scriptUploadBtn").addEventListener("click", () => $("scriptFileInput").click());
  $("scriptFileInput").addEventListener("change", async event => {
    const file = event.target.files[0];
    event.target.value = "";
    if (!file) return;
    try {
      await uploadScript(file, false);
      await loadScripts();
    } catch (error) {
      alert("上传失败：" + error.message);
    }
  });
  $("logRefreshBtn").addEventListener("click", () => {
    if (state.selected) connectLogStream(state.selected);
  });
  $("logFollowBtn").addEventListener("click", () => {
    state.follow = !state.follow;
    $("logFollowBtn").classList.toggle("active", state.follow);
    if (state.follow && state.selected) {
      connectLogStream(state.selected);
    } else {
      closeLogStream();
    }
  });

  loadScripts();
  state.listTimer = setInterval(loadScripts, 5000);
})();
