// 左侧图标导航：所有控制台页面引入本脚本即可获得一致的入口。
(function () {
  const items = [
    {
      href: "/",
      label: "控制台",
      match: path => path === "/",
      icon: '<path d="M4 13h6V4H4zM14 20h6v-9h-6zM4 20h6v-4H4zM14 8h6V4h-6z"/>'
    },
    {
      href: "/scan",
      label: "扫码添加",
      match: path => path.startsWith("/scan"),
      icon: '<path d="M4 9V5a1 1 0 0 1 1-1h4M20 9V5a1 1 0 0 0-1-1h-4M4 15v4a1 1 0 0 0 1 1h4M20 15v4a1 1 0 0 1-1 1h-4M8 8h3v3H8zM13 13h3v3h-3z"/>'
    },
    {
      href: "/apps",
      label: "应用",
      match: path => path.startsWith("/apps"),
      icon: '<path d="M4 5h6v6H4zM14 5h6v6h-6zM4 13h6v6H4zM14 13h6v6h-6z"/>'
    },
    {
      href: "/docs/index.html",
      label: "Swagger 文档",
      match: path => path.startsWith("/docs"),
      icon: '<path d="M5 4h9a3 3 0 0 1 3 3v13H8a3 3 0 0 1-3-3zM17 7h2v13H8"/>'
    }
  ];

  function render() {
    if (document.querySelector(".rail")) return;
    const path = location.pathname;
    const rail = document.createElement("nav");
    rail.className = "rail";
    rail.setAttribute("aria-label", "主导航");

    const brand = document.createElement("a");
    brand.className = "rail-brand";
    brand.href = "/";
    brand.textContent = "Y";
    brand.setAttribute("aria-label", "YYB Go 控制台");
    rail.appendChild(brand);

    for (const item of items) {
      const link = document.createElement("a");
      link.className = "rail-item" + (item.match(path) ? " active" : "");
      link.href = item.href;
      link.title = item.label;
      link.setAttribute("aria-label", item.label);
      if (item.match(path)) link.setAttribute("aria-current", "page");
      link.innerHTML =
        '<svg viewBox="0 0 24 24" aria-hidden="true">' + item.icon + "</svg>" +
        '<span class="rail-tip">' + item.label + "</span>";
      rail.appendChild(link);
    }

    const spacer = document.createElement("div");
    spacer.className = "rail-spacer";
    rail.appendChild(spacer);

    // 登录用户显示退出入口
    fetch("/auth/me").then(response => response.json()).then(payload => {
      if (payload.code !== 0) return;
      const data = payload.data || {};
      if (!data.user) return;
      const logout = document.createElement("a");
      logout.className = "rail-item";
      logout.href = "#";
      logout.title = "退出登录（" + (data.user.display_name || data.user.username) + "）";
      logout.setAttribute("aria-label", "退出登录");
      logout.innerHTML =
        '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M15 4h4a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1h-4M10 17l5-5-5-5M15 12H3"/></svg>' +
        '<span class="rail-tip">退出登录</span>';
      logout.addEventListener("click", event => {
        event.preventDefault();
        fetch("/logout", { method: "POST" }).then(() => { location.href = "/login"; });
      });
      rail.appendChild(logout);
    }).catch(() => {});

    document.body.insertBefore(rail, document.body.firstChild);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", render);
  } else {
    render();
  }
})();
