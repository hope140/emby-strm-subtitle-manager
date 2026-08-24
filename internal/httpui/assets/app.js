(() => {
  "use strict";

  let authToken = "";
  let selectedLibrary = "";
  let startIndex = 0;
  let currentLimit = 50;
  let listRequestID = 0;
  let detailRequestID = 0;

  const elements = {
    loginPanel: document.getElementById("login-panel"),
    loginForm: document.getElementById("login-form"),
    token: document.getElementById("token"),
    loginError: document.getElementById("login-error"),
    logout: document.getElementById("logout"),
    appPanel: document.getElementById("app-panel"),
    library: document.getElementById("library"),
    loadItems: document.getElementById("load-items"),
    appStatus: document.getElementById("app-status"),
    items: document.getElementById("items"),
    detail: document.getElementById("detail"),
    previousPage: document.getElementById("previous-page"),
    nextPage: document.getElementById("next-page"),
    pageStatus: document.getElementById("page-status")
  };

  function setText(element, value) {
    element.textContent = value == null || value === "" ? "—" : String(value);
  }

  function clearText(element) {
    element.textContent = "";
  }

  function addText(parent, value, className) {
    const element = document.createElement("span");
    if (className) element.className = className;
    setText(element, value);
    parent.appendChild(element);
    return element;
  }

  function clear(element) {
    while (element.firstChild) element.removeChild(element.firstChild);
  }

  function setError(element, error) {
    setText(element, error && error.message ? error.message : "请求失败");
  }

  async function apiGet(resource) {
    if (!authToken) throw new Error("请先登录");
    const response = await fetch(resource, {
      method: "GET",
      headers: { Authorization: "Bearer " + authToken },
      cache: "no-store"
    });
    let payload = null;
    try { payload = await response.json(); } catch (_) {}
    if (!response.ok) {
      const message = payload && payload.error && payload.error.message;
      if (response.status === 409 && payload && Array.isArray(payload.media_sources)) {
        const sourceError = new Error(message || "需要选择媒体源");
        sourceError.mediaSources = payload.media_sources;
        throw sourceError;
      }
      throw new Error(message || "请求失败（" + response.status + "）");
    }
    return payload;
  }

  function resetPage() {
    startIndex = 0;
    currentLimit = 50;
    elements.previousPage.disabled = true;
    elements.nextPage.disabled = true;
    setText(elements.pageStatus, "未加载");
  }

  function fillLibraries(libraries) {
    clear(elements.library);
    libraries.forEach((library) => {
      const option = document.createElement("option");
      option.value = library.id || "";
      setText(option, library.name || "未命名媒体库");
      elements.library.appendChild(option);
    });
    elements.library.disabled = libraries.length === 0;
    elements.loadItems.disabled = libraries.length === 0;
    selectedLibrary = libraries.length ? libraries[0].id : "";
    if (selectedLibrary) elements.library.value = selectedLibrary;
  }

  function renderItems(page) {
    clear(elements.items);
    if (!page.items || page.items.length === 0) {
      elements.items.className = "item-list empty-state";
      setText(elements.items, "当前页没有 Movie 或 Episode。");
    } else {
      elements.items.className = "item-list";
      page.items.forEach((item) => {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "item";
        button.addEventListener("click", () => loadDetail(item.id));
        addText(button, item.type || "未知类型", "type-badge");
        addText(button, item.name || "未命名媒体", "item-title");
        const facts = document.createElement("span");
        facts.className = "muted";
        const episode = item.type === "Episode" && item.parent_index_number != null && item.index_number != null
          ? " · S" + item.parent_index_number + "E" + item.index_number
          : "";
        setText(facts, (item.series_name || "") + episode + (item.production_year ? " · " + item.production_year : ""));
        button.appendChild(facts);
        elements.items.appendChild(button);
      });
    }
    const total = Number(page.total_record_count || 0);
    const current = total ? Math.floor(Number(page.start_index || 0) / Number(page.limit || currentLimit)) + 1 : 0;
    const pages = total ? Math.ceil(total / Number(page.limit || currentLimit)) : 0;
    setText(elements.pageStatus, pages ? "第 " + current + " / " + pages + " 页" : "无结果");
    elements.previousPage.disabled = Number(page.start_index || 0) <= 0;
    elements.nextPage.disabled = !page.has_more;
    currentLimit = Number(page.limit || currentLimit);
  }

  async function loadItems() {
    if (!selectedLibrary) return;
    const requestID = ++listRequestID;
    elements.loadItems.disabled = true;
    elements.previousPage.disabled = true;
    elements.nextPage.disabled = true;
    setText(elements.appStatus, "加载媒体列表…");
    try {
      const params = new URLSearchParams({ library_id: selectedLibrary, start_index: String(startIndex), limit: String(currentLimit) });
      const page = await apiGet("/v1/emby/items?" + params.toString());
      if (requestID !== listRequestID) return;
      renderItems(page);
      setText(elements.appStatus, "列表已加载");
    } catch (error) {
      if (requestID !== listRequestID) return;
      clear(elements.items);
      elements.items.className = "item-list empty-state";
      setError(elements.items, error);
      clearText(elements.appStatus);
    } finally {
      if (requestID === listRequestID) elements.loadItems.disabled = false;
    }
  }

  function addDetailRow(parent, label, value) {
    const labelElement = document.createElement("dt");
    setText(labelElement, label);
    const valueElement = document.createElement("dd");
    setText(valueElement, value);
    parent.appendChild(labelElement);
    parent.appendChild(valueElement);
  }

  function addStatus(parent, label, value, success) {
    const row = document.createElement("p");
    addText(row, label + "：");
    const status = addText(row, value, success ? "success" : "");
    parent.appendChild(row);
    return status;
  }

  function addMessages(parent, heading, values, className) {
    if (!Array.isArray(values) || values.length === 0) return;
    const title = document.createElement("strong");
    setText(title, heading);
    parent.appendChild(title);
    const list = document.createElement("ul");
    list.className = className;
    values.forEach((value) => {
      const item = document.createElement("li");
      setText(item, typeof value === "string" ? value : (value.reason || value.code || "未知问题"));
      list.appendChild(item);
    });
    parent.appendChild(list);
  }

  function renderSubtitles(detail, inventory) {
    const section = document.createElement("section");
    section.className = "subtitles";
    const heading = document.createElement("h3");
    setText(heading, "字幕清单");
    section.appendChild(heading);
    addStatus(section, "清单完整性", inventory.inventory_complete ? "完整" : "不完整", inventory.inventory_complete === true);
    addStatus(section, "Presence", inventory.presence || "unknown", inventory.presence === "present");
    addMessages(section, "Issues", inventory.issues, "issue-list");
    addMessages(section, "Warnings", inventory.warnings, "warning-list");
    if (!Array.isArray(inventory.subtitles) || inventory.subtitles.length === 0) {
      const empty = document.createElement("p");
      empty.className = "muted";
      setText(empty, "没有可展示的字幕记录。");
      section.appendChild(empty);
    } else {
      inventory.subtitles.forEach((subtitle) => {
        const card = document.createElement("article");
        card.className = "subtitle";
        addText(card, subtitle.kind || "unknown", "type-badge");
        addText(card, subtitle.manageable ? "manageable" : "unmanaged", "status-badge");
        addText(card, subtitle.file_name || "无文件名", "item-title");
        const facts = document.createElement("p");
        facts.className = "muted";
        const flags = [];
        if (subtitle.language) flags.push(subtitle.language);
        if (subtitle.format) flags.push(subtitle.format);
        if (subtitle.is_default) flags.push("default");
        if (subtitle.is_forced) flags.push("forced");
        flags.push(Array.isArray(subtitle.discovered_by) ? subtitle.discovered_by.join(", ") : "unknown");
        setText(facts, flags.join(" · "));
        card.appendChild(facts);
        if (subtitle.unmanageable_reason) addMessages(card, "状态说明", [subtitle.unmanageable_reason], "warning-list");
        section.appendChild(card);
      });
    }
    detail.appendChild(section);
  }

  function sourceButton(source, onSelect) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "item";
    button.addEventListener("click", () => onSelect(source.media_source_id));
    addText(button, source.name || "未命名媒体源", "item-title");
    const facts = document.createElement("span");
    facts.className = "muted";
    setText(facts, (source.container || "未知容器") + (source.is_default ? " · 默认源" : ""));
    button.appendChild(facts);
    return button;
  }

  async function loadDetail(itemID, mediaSourceID) {
    const requestID = ++detailRequestID;
    clear(elements.detail);
    elements.detail.className = "empty-state";
    setText(elements.detail, "加载详情…");
    try {
      const query = mediaSourceID ? "?media_source_id=" + encodeURIComponent(mediaSourceID) : "";
      const media = await apiGet("/v1/media/" + encodeURIComponent(itemID) + query);
      if (requestID !== detailRequestID) return;
      renderDetail(media, itemID, mediaSourceID);
      const subtitles = await apiGet("/v1/media/" + encodeURIComponent(itemID) + "/subtitles" + query);
      if (requestID !== detailRequestID) return;
      renderSubtitles(elements.detail, subtitles.inventory || {});
    } catch (error) {
      if (requestID !== detailRequestID) return;
      clear(elements.detail);
      elements.detail.className = "";
      if (Array.isArray(error.mediaSources)) {
        const heading = document.createElement("p");
        setText(heading, "此媒体有多个媒体源，请明确选择后查看详情。");
        elements.detail.appendChild(heading);
        error.mediaSources.forEach((source) => elements.detail.appendChild(sourceButton(source, (sourceID) => loadDetail(itemID, sourceID))));
      } else {
        setError(elements.detail, error);
      }
    }
  }

  function renderDetail(media, itemID, mediaSourceID) {
    clear(elements.detail);
    elements.detail.className = "";
    const title = document.createElement("h3");
    setText(title, media.title || "未命名媒体");
    elements.detail.appendChild(title);
    const grid = document.createElement("dl");
    grid.className = "detail-grid";
    addDetailRow(grid, "类型", media.type);
    addDetailRow(grid, "系列", media.series_name);
    addDetailRow(grid, "季 / 集", media.season != null && media.episode != null ? "S" + media.season + "E" + media.episode : "—");
    addDetailRow(grid, "年份", media.year);
    addDetailRow(grid, "STRM", media.is_strm ? "是" : "否");
    addDetailRow(grid, "媒体源", mediaSourceID || media.media_source_id ? "已选择" : "未选择");
    addDetailRow(grid, "映射状态", media.mapping_status);
    elements.detail.appendChild(grid);
    addStatus(elements.detail, "媒体上下文完整性", media.inventory_complete ? "完整" : "不完整", media.inventory_complete === true);
    addMessages(elements.detail, "Warnings", media.warnings, "warning-list");
  }

  async function login(event) {
    event.preventDefault();
    authToken = elements.token.value;
    elements.token.value = "";
    clearText(elements.loginError);
    if (!authToken) {
      setText(elements.loginError, "请输入 Token");
      return;
    }
    try {
      await apiGet("/v1/health");
      const libraries = await apiGet("/v1/emby/libraries");
      fillLibraries(Array.isArray(libraries) ? libraries : []);
      resetPage();
      elements.loginPanel.classList.add("hidden");
      elements.appPanel.classList.remove("hidden");
      elements.logout.classList.remove("hidden");
      if (selectedLibrary) await loadItems();
    } catch (error) {
      authToken = "";
      setError(elements.loginError, error);
    }
  }

  function logout() {
    authToken = "";
    listRequestID += 1;
    detailRequestID += 1;
    selectedLibrary = "";
    clear(elements.items);
    clear(elements.detail);
    setText(elements.items, "请选择媒体库。");
    setText(elements.detail, "选择一个 Movie 或 Episode 查看详情。");
    elements.items.className = "item-list empty-state";
    elements.detail.className = "empty-state";
    elements.loginPanel.classList.remove("hidden");
    elements.appPanel.classList.add("hidden");
    elements.logout.classList.add("hidden");
    clearText(elements.loginError);
    clearText(elements.appStatus);
  }

  elements.loginForm.addEventListener("submit", login);
  elements.logout.addEventListener("click", logout);
  elements.library.addEventListener("change", () => {
    selectedLibrary = elements.library.value;
    resetPage();
    loadItems();
  });
  elements.loadItems.addEventListener("click", () => loadItems());
  elements.previousPage.addEventListener("click", () => {
    startIndex = Math.max(0, startIndex - currentLimit);
    loadItems();
  });
  elements.nextPage.addEventListener("click", () => {
    startIndex += currentLimit;
    loadItems();
  });
})();
