(() => {
  "use strict";

  let selectedLibrary = "";
  let startIndex = 0;
  let currentLimit = 50;
  let listRequestID = 0;
  let detailRequestID = 0;
  const browse = {
    level: "root",
    parentID: "",
    mode: "nodes",
    crumbs: [{ level: "root", parentID: "", label: "媒体库" }]
  };
  const knownMultiSourceItems = new Set();
  const d2 = {
    remoteSearchEnabled: false,
    itemID: "",
    media: null,
    multipleSources: false,
    candidates: [],
    artifact: null,
    searchRequestID: 0,
    previewRequestID: 0,
    previewBusy: false,
    previewOffset: 0,
    previewCues: [],
    previewHasNext: false,
    previewOffsets: [0],
    previewLimit: 200,
    inventory: null
  };
  const d3 = {
    writeEnabled: false,
    writeCapabilities: { add: false, replace: false, delete: false, restore: false, reason_code: "" },
    csrfToken: "",
    addBusy: false,
    uploadBusy: false,
    operationBusy: false,
    historyRequestID: 0,
    history: []
  };

  const elements = {
    loginPanel: document.getElementById("login-panel"),
    loginForm: document.getElementById("login-form"),
    username: document.getElementById("username"),
    password: document.getElementById("password"),
    loginError: document.getElementById("login-error"),
    appPanel: document.getElementById("app-panel"),
    library: document.getElementById("library"),
    loadItems: document.getElementById("load-items"),
    refreshHealth: document.getElementById("refresh-health"),
    healthSummary: document.getElementById("health-summary"),
    appStatus: document.getElementById("app-status"),
    items: document.getElementById("items"),
    detail: document.getElementById("detail"),
    browseBack: document.getElementById("browse-back"),
    browsePath: document.getElementById("browse-path"),
    previousPage: document.getElementById("previous-page"),
    nextPage: document.getElementById("next-page"),
    pageStatus: document.getElementById("page-status"),
    d2Panel: document.getElementById("d2-panel"),
    d2Status: document.getElementById("d2-status"),
    d2Actions: document.getElementById("d2-actions"),
    d2Forced: document.getElementById("d2-forced"),
    d2Search: document.getElementById("d2-search"),
    d2UploadFile: document.getElementById("d2-upload-file"),
    d2Upload: document.getElementById("d2-upload"),
    d2UploadStatus: document.getElementById("d2-upload-status"),
    d3WriteStatus: document.getElementById("d3-write-status"),
    d2Results: document.getElementById("d2-results"),
    d2ResultStatus: document.getElementById("d2-result-status"),
    d2ProviderSummary: document.getElementById("d2-provider-summary"),
    d2Candidates: document.getElementById("d2-candidates"),
    d2Preview: document.getElementById("d2-preview"),
    d2ArtifactMeta: document.getElementById("d2-artifact-meta"),
    d2PreviewLimit: document.getElementById("d2-preview-limit"),
    d2PreviewPrevious: document.getElementById("d2-preview-previous"),
    d2PreviewNext: document.getElementById("d2-preview-next"),
    d2PreviewReset: document.getElementById("d2-preview-reset"),
    d2PreviewStatus: document.getElementById("d2-preview-status"),
    d2Cues: document.getElementById("d2-cues"),
    d3Add: document.getElementById("d3-add"),
    d3AddButton: document.getElementById("d3-add-button"),
    d3AddStatus: document.getElementById("d3-add-status"),
    d3History: document.getElementById("d3-history"),
    d3HistoryReload: document.getElementById("d3-history-reload"),
    d3HistoryType: document.getElementById("d3-history-type"),
    d3HistoryStatusFilter: document.getElementById("d3-history-status-filter"),
    d3HistoryStatus: document.getElementById("d3-history-status"),
    d3HistoryList: document.getElementById("d3-history-list")
  };

  const safeMessages = {
    unauthorized: "登录已失效，请重新登录。",
    invalid_credentials: "管理员用户名或密码错误。",
    login_rate_limited: "登录尝试过于频繁，请稍后重试。",
    admin_login_unavailable: "管理员登录尚未配置，请检查 APP_ADMIN_USERNAME 和 APP_ADMIN_PASSWORD。",
    session_unavailable: "管理员会话暂时不可用，请稍后重试。",
    remote_search_disabled: "远程搜索未启用，请联系管理员确认功能开关。",
    canary_item_not_allowed: "当前媒体未获准进行字幕操作，请选择已授权媒体。",
    media_not_found: "媒体不存在或已不可用。",
    candidate_invalid: "候选已失效，请重新搜索。",
    artifact_invalid: "预览已失效，请重新获取候选。",
    candidate_expired: "候选已过期，请重新搜索。",
    artifact_expired: "预览已过期，请重新获取候选。",
    media_source_selection_required: "当前媒体有多个版本，请先明确选择一个媒体源。",
    media_source_mismatch: "媒体源已变化，请重新选择媒体详情。",
    subtitle_too_large: "字幕内容超过预览大小限制，请选择其他候选。",
    subtitle_invalid: "字幕内容无法安全解析，请选择其他候选。",
    subtitle_format_unsupported: "字幕格式暂不支持，请选择其他候选。",
    rate_limited: "请求过于频繁，请稍后重试。",
    provider_search_failed: "字幕搜索暂时失败，请稍后重试。",
    emby_timeout: "上游服务响应超时，请稍后重试。",
    candidate_fetch_failed: "该候选获取失败，其他候选仍可用。",
    candidate_fetch_timeout: "该候选获取超时，其他候选仍可用。",
    emby_invalid_response: "上游服务返回内容无效，请稍后重试。",
    emby_unavailable: "上游服务暂时不可用，请稍后重试。",
    preview_store_unavailable: "预览暂时不可用，请稍后重试。",
    invalid_request: "请求参数无效，请重新操作。",
    media_source_required: "请先选择一个媒体源。",
    write_disabled: "写入功能未启用，请联系管理员确认 D3 专用样本状态。",
    csrf_required: "当前会话的安全校验已失效，请重新登录。",
    csrf_origin_invalid: "请求来源未通过安全校验。",
    d3_item_not_allowed: "当前媒体未获准进行字幕写入操作。",
    emby_refresh_failed: "Emby 刷新失败，新文件已隔离。",
    emby_subtitle_not_visible: "Emby 未识别新字幕，新文件已隔离。",
    d3_history_unavailable: "操作记录暂时不可用，请稍后重试。",
    strm_history_location_unsupported: "这条历史记录来自旧的 STRM source 目录语义，当前无法安全恢复。",
    subtitle_unmanageable: "该字幕不是可安全管理的外挂字幕。",
    subtitle_inventory_incomplete: "字幕清单不完整，已拒绝写入操作。",
    subtitle_not_found: "目标字幕已变化，请重新打开详情。",
    operation_conflict: "此操作已被用于不同请求，请重新操作。",
    operation_not_found: "未找到可恢复的操作记录。",
    restore_unavailable: "该操作当前不能恢复。",
    restore_target_conflict: "恢复目标已有同名字幕，未覆盖现有文件。",
    restore_hash_mismatch: "恢复副本校验失败，未执行恢复。",
    subtitle_archive_failed: "旧字幕归档失败，未完成替换。",
    subtitle_trash_failed: "字幕移入回收区失败，未删除原文件。",
    restore_failed: "字幕恢复失败，原文件未被覆盖。",
    write_verification_failed: "字幕写入后校验失败，文件已隔离。"
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

  function setVisible(element, visible) {
    element.classList.toggle("hidden", !visible);
  }

  function emptyWriteCapabilities() {
    return { add: false, replace: false, delete: false, restore: false, reason_code: "" };
  }

  function setWriteCapabilities(media) {
    const capabilities = media && media.write_capabilities ? media.write_capabilities : null;
    d3.writeCapabilities = capabilities ? {
      add: capabilities.add === true,
      replace: capabilities.replace === true,
      delete: capabilities.delete === true,
      restore: capabilities.restore === true,
      reason_code: typeof capabilities.reason_code === "string" ? capabilities.reason_code : ""
    } : emptyWriteCapabilities();
    const reason = d3.writeCapabilities.reason_code;
    if (d3.writeEnabled && reason && safeMessages[reason]) {
      setVisible(elements.d3WriteStatus, true);
      setText(elements.d3WriteStatus, safeMessages[reason]);
    } else {
      setVisible(elements.d3WriteStatus, false);
      clearText(elements.d3WriteStatus);
    }
  }

  function safeErrorMessage(code, status, retryAfter) {
    if (status === 429 || code === "rate_limited") {
      const seconds = Number.isInteger(retryAfter) && retryAfter >= 0 ? retryAfter : 1;
      return seconds > 0 ? "请求过于频繁，请在 " + seconds + " 秒后重试。" : "请求过于频繁，请稍后重试。";
    }
    if (safeMessages[code]) return safeMessages[code];
    if (status === 401) return safeMessages.unauthorized;
    if (status >= 500) return "服务暂时不可用，请稍后重试。";
    return "请求未完成，请检查当前状态后重试。";
  }

  function makeError(code, status, retryAfter) {
    const error = new Error(safeErrorMessage(code, status, retryAfter));
    error.code = code || "request_failed";
    error.status = status || 0;
    error.retryAfter = retryAfter;
    return error;
  }

  function setError(element, error) {
    element.classList.add("error");
    setText(element, error && error.message ? error.message : "请求未完成，请稍后重试。");
  }

  function clearError(element) {
    element.classList.remove("error");
    clearText(element);
  }

  async function parseResponse(response) {
    try {
      return await response.json();
    } catch (_) {
      return null;
    }
  }

  function clearAppState() {
    listRequestID += 1;
    detailRequestID += 1;
    selectedLibrary = "";
    d2.remoteSearchEnabled = false;
    d3.writeEnabled = false;
    d3.csrfToken = "";
    d3.addBusy = false;
    d3.uploadBusy = false;
    d3.operationBusy = false;
    fillLibraries([]);
    resetBrowse();
    resetD2ForItem("", false);
    clear(elements.items);
    setText(elements.items, "请选择媒体库。");
    elements.items.className = "item-list empty-state";
    clear(elements.detail);
    setText(elements.detail, "选择一个 Movie 或 Episode 查看详情。");
    elements.detail.className = "empty-state";
    elements.refreshHealth.disabled = true;
    clearText(elements.healthSummary);
    clearText(elements.appStatus);
  }

  function expireSession() {
    clearAppState();
    elements.appPanel.classList.add("hidden");
    elements.loginPanel.classList.remove("hidden");
    elements.password.value = "";
    setText(elements.loginError, "登录已失效，请重新登录。");
  }

  async function apiGet(resource) {
    const response = await fetch(resource, {
      method: "GET",
      credentials: "same-origin",
      cache: "no-store"
    });
    const payload = await parseResponse(response);
    if (!response.ok) {
      if (response.status === 401) expireSession();
      const errorBody = payload && payload.error ? payload.error : {};
      const error = makeError(errorBody.code, response.status, parseRetryAfter(response));
      if (response.status === 409 && payload && Array.isArray(payload.media_sources)) {
        error.mediaSources = payload.media_sources;
      }
      throw error;
    }
    return payload;
  }

  async function apiPost(resource, body) {
    const headers = {"Content-Type": "application/json"};
    if (d3.csrfToken) headers["X-CSRF-Token"] = d3.csrfToken;
    const response = await fetch(resource, {
      method: "POST",
      headers,
      credentials: "same-origin",
      body: JSON.stringify(body),
      cache: "no-store"
    });
    const payload = await parseResponse(response);
    if (!response.ok) {
      if (response.status === 401) expireSession();
      const errorBody = payload && payload.error ? payload.error : {};
      throw makeError(errorBody.code, response.status, parseRetryAfter(response));
    }
    return payload;
  }

  async function apiMultipart(resource, formData) {
    const headers = {};
    if (d3.csrfToken) headers["X-CSRF-Token"] = d3.csrfToken;
    const response = await fetch(resource, {
      method: "POST",
      headers,
      credentials: "same-origin",
      body: formData,
      cache: "no-store"
    });
    const payload = await parseResponse(response);
    if (!response.ok) {
      if (response.status === 401) expireSession();
      const errorBody = payload && payload.error ? payload.error : {};
      throw makeError(errorBody.code, response.status, parseRetryAfter(response));
    }
    return payload;
  }

  function parseRetryAfter(response) {
    const value = Number.parseInt(response.headers.get("Retry-After") || "", 10);
    return Number.isInteger(value) && value >= 0 && value <= 60 ? value : 0;
  }

  function resetPage() {
    startIndex = 0;
    currentLimit = 50;
    elements.previousPage.disabled = true;
    elements.nextPage.disabled = true;
    setText(elements.pageStatus, "未加载");
  }

  function resetBrowse() {
    browse.level = "root";
    browse.parentID = "";
    browse.mode = "nodes";
    browse.crumbs = [{ level: "root", parentID: "", label: "媒体库" }];
    resetPage();
    renderBrowsePath();
  }

  function renderBrowsePath() {
    clear(elements.browsePath);
    browse.crumbs.forEach((crumb, index) => {
      if (index > 0) addText(elements.browsePath, "›", "muted");
      if (index === browse.crumbs.length - 1) {
        addText(elements.browsePath, crumb.label, "current");
        return;
      }
      const button = document.createElement("button");
      button.type = "button";
      setText(button, crumb.label);
      button.addEventListener("click", () => {
        browse.crumbs = browse.crumbs.slice(0, index + 1);
        applyBrowseCrumb();
      });
      elements.browsePath.appendChild(button);
    });
    elements.browseBack.disabled = browse.mode === "nodes" && browse.crumbs.length <= 1;
  }

  function applyBrowseCrumb() {
    const current = browse.crumbs[browse.crumbs.length - 1];
    browse.level = current.level;
    browse.parentID = current.parentID;
    browse.mode = "nodes";
    resetPage();
    renderBrowsePath();
    loadItems();
  }

  function enterBrowse(level, parentID, label) {
    browse.crumbs.push({ level, parentID, label });
    applyBrowseCrumb();
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

  function browseTypeLabel(type) {
    switch (type) {
      case "Movie": return "电影";
      case "Series": return "剧集";
      case "Season": return "季";
      case "Episode": return "集";
      default: return "媒体";
    }
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
        switch (item.type) {
          case "Series":
            button.addEventListener("click", () => enterBrowse("series", item.id, item.name || "未命名剧集"));
            break;
          case "Season":
            button.addEventListener("click", () => enterBrowse("season", item.id, item.name || "未命名季"));
            break;
          default:
            button.addEventListener("click", () => loadSources(item));
            break;
        }
        addText(button, browseTypeLabel(item.type), "type-badge");
        addText(button, item.name || "未命名媒体", "item-title");
        const facts = document.createElement("span");
        facts.className = "muted";
        const values = [];
        if (item.type === "Episode" && item.parent_index_number != null && item.index_number != null) values.push("S" + item.parent_index_number + "E" + item.index_number);
        else if (item.type === "Season" && item.index_number != null) values.push("第 " + item.index_number + " 季");
        if (item.production_year) values.push(String(item.production_year));
        setText(facts, values.join(" · "));
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
    browse.mode = "nodes";
    renderBrowsePath();
    elements.loadItems.disabled = true;
    elements.previousPage.disabled = true;
    elements.nextPage.disabled = true;
    setText(elements.appStatus, "加载媒体列表…");
    try {
      const params = new URLSearchParams({ library_id: selectedLibrary, level: browse.level, start_index: String(startIndex), limit: String(currentLimit) });
      if (browse.parentID) params.set("parent_id", browse.parentID);
      const page = await apiGet("/v1/emby/browse?" + params.toString());
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

  async function loadSources(item) {
    if (!item || !item.id) return;
    const requestID = ++listRequestID;
    browse.mode = "sources";
    renderBrowsePath();
    clear(elements.items);
    elements.items.className = "item-list empty-state";
    setText(elements.items, "加载版本…");
    elements.previousPage.disabled = true;
    elements.nextPage.disabled = true;
    try {
      const response = await apiGet("/v1/media/" + encodeURIComponent(item.id) + "/sources");
      if (requestID !== listRequestID) return;
      const sources = Array.isArray(response.media_sources) ? response.media_sources : [];
      clear(elements.items);
      if (sources.length === 0) {
        elements.items.className = "item-list empty-state";
        setText(elements.items, "当前媒体没有可选择的版本。");
        return;
      }
      elements.items.className = "item-list";
      const heading = document.createElement("p");
      heading.className = "muted";
      setText(heading, sources.length === 1 ? "当前媒体只有一个版本，已自动打开详情。" : "请选择一个明确版本后再查看字幕和执行操作。");
      elements.items.appendChild(heading);
      sources.forEach((source) => elements.items.appendChild(sourceButton(source, (sourceID) => loadDetail(item.id, sourceID))));
      if (sources.length === 1 && sources[0] && sources[0].media_source_id) loadDetail(item.id, sources[0].media_source_id);
    } catch (error) {
      if (requestID !== listRequestID) return;
      clear(elements.items);
      elements.items.className = "item-list empty-state";
      setError(elements.items, error);
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
    const previous = detail.querySelector(".subtitles");
    if (previous) previous.remove();
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
        if (d3.writeEnabled && d3.writeCapabilities.replace && d3.writeCapabilities.delete && d2.media && d2.media.media_source_id && subtitle.manageable === true) {
          const actions = document.createElement("div");
          actions.className = "subtitle-actions";
          const replace = document.createElement("button");
          replace.type = "button";
          setText(replace, d2.artifact ? "用当前预览替换" : "先获取字幕预览");
          replace.disabled = !d2.artifact || d3.operationBusy;
          replace.addEventListener("click", () => replaceSubtitle(subtitle));
          actions.appendChild(replace);
          const remove = document.createElement("button");
          remove.type = "button";
          remove.className = "secondary";
          setText(remove, "移入回收区");
          remove.disabled = d3.operationBusy;
          remove.addEventListener("click", () => deleteSubtitle(subtitle));
          actions.appendChild(remove);
          card.appendChild(actions);
        }
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

  function formatDate(value) {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? "—" : date.toLocaleString("zh-CN", { hour12: false });
  }

  function formatScore(value) {
    const score = Number(value);
    return Number.isFinite(score) ? score.toFixed(2) : "0.00";
  }

  function candidateStateLabel(state) {
    switch (state) {
      case "ready": return "可用";
      case "fetching": return "获取中";
      case "fetched": return "已获取";
      case "failed": return "获取失败";
      case "expired": return "已过期";
      default: return "未知状态";
    }
  }

  function candidateCard(candidate) {
    const card = document.createElement("article");
    card.className = "candidate-card";
    if (candidate.state === "fetched") card.classList.add("is-fetched");
    if (candidate.state === "failed") card.classList.add("is-failed");
    if (candidate.state === "expired") card.classList.add("is-expired");

    const header = document.createElement("div");
    header.className = "candidate-header";
    addText(header, candidate.provider || "未知 Provider", "type-badge");
    addText(header, candidateStateLabel(candidate.state), "status-badge");
    card.appendChild(header);

    const facts = document.createElement("dl");
    facts.className = "candidate-facts";
    addDetailRow(facts, "名称", candidate.name);
    addDetailRow(facts, "Provider", candidate.provider);
    addDetailRow(facts, "语言", candidate.language);
    addDetailRow(facts, "格式", candidate.format);
    addDetailRow(facts, "Comment", candidate.comment);
    addDetailRow(facts, "Hash match", candidate.is_hash_match ? "是" : "否");
    addDetailRow(facts, "Score", formatScore(candidate.score));
    addDetailRow(facts, "状态", candidateStateLabel(candidate.state));
    addDetailRow(facts, "过期时间", formatDate(candidate.expires_at));
    card.appendChild(facts);

    if (candidate.errorMessage) {
      const error = document.createElement("p");
      error.className = "candidate-error";
      setText(error, candidate.errorMessage);
      card.appendChild(error);
    }

    const footer = document.createElement("div");
    footer.className = "candidate-footer";
    const actionHint = document.createElement("span");
    actionHint.className = "muted";
    setText(actionHint, candidate.state === "fetched" ? "Artifact 已生成，可在下方查看。" : "单个候选操作");
    footer.appendChild(actionHint);
    const button = document.createElement("button");
    button.type = "button";
    setText(button, candidate.state === "fetching" ? "获取中…" : "获取预览");
    button.disabled = candidate.state !== "ready";
    button.addEventListener("click", () => fetchCandidate(candidate));
    footer.appendChild(button);
    card.appendChild(footer);
    return card;
  }

  function renderProviderSummary() {
    clear(elements.d2ProviderSummary);
    if (!d2.candidates.length) return;
    const groups = new Map();
    d2.candidates.forEach((candidate) => {
      const provider = candidate.provider || "未知 Provider";
      const group = groups.get(provider) || { total: 0, ready: 0, fetching: 0, fetched: 0, failed: 0, expired: 0 };
      group.total += 1;
      if (Object.prototype.hasOwnProperty.call(group, candidate.state)) group[candidate.state] += 1;
      groups.set(provider, group);
    });
    const summaries = [];
    groups.forEach((group, provider) => {
      const states = [];
      if (group.ready) states.push("可用 " + group.ready);
      if (group.fetching) states.push("获取中 " + group.fetching);
      if (group.fetched) states.push("已获取 " + group.fetched);
      if (group.failed) states.push("失败 " + group.failed);
      if (group.expired) states.push("过期 " + group.expired);
      summaries.push(provider + "：" + group.total + " 个（" + states.join("，") + "）");
    });
    setText(elements.d2ProviderSummary, "Provider 候选概览：" + summaries.join("；"));
  }

  function renderCandidates() {
    clear(elements.d2Candidates);
    d2.candidates.forEach((candidate) => elements.d2Candidates.appendChild(candidateCard(candidate)));
    const count = d2.candidates.length;
    setText(elements.d2ResultStatus, count ? "显示 " + count + " 个候选" : "没有候选");
    renderProviderSummary();
  }

  function clearPreview() {
    d2.artifact = null;
    d2.previewRequestID += 1;
    d2.previewBusy = false;
    d2.previewOffset = 0;
    d2.previewCues = [];
    d2.previewHasNext = false;
    d2.previewOffsets = [0];
    clearText(elements.d2ArtifactMeta);
    clearText(elements.d2PreviewStatus);
    clear(elements.d2Cues);
    setVisible(elements.d2Preview, false);
    setVisible(elements.d3Add, false);
    clearText(elements.d3AddStatus);
    elements.d3AddButton.disabled = false;
    clearText(elements.d2UploadStatus);
    elements.d2Upload.disabled = false;
    elements.d2PreviewPrevious.disabled = true;
    elements.d2PreviewNext.disabled = true;
  }

  function resetD2ForItem(itemID, multipleSources) {
    d2.itemID = itemID;
    d2.media = null;
    d2.inventory = null;
    d3.writeCapabilities = emptyWriteCapabilities();
    d2.multipleSources = multipleSources;
    d2.candidates = [];
    d2.searchRequestID += 1;
    // A pending search from the previous detail is now stale. Its finally
    // handler will not re-enable this button, so reset it explicitly.
    elements.d2Search.disabled = false;
    clear(elements.d2Candidates);
    clearText(elements.d2ResultStatus);
    setVisible(elements.d2Results, false);
    clearPreview();
    setVisible(elements.d2Panel, false);
    clearError(elements.d2Status);
    setVisible(elements.d2Actions, false);
    d3.historyRequestID += 1;
    d3.history = [];
    clear(elements.d3HistoryList);
    clearText(elements.d3HistoryStatus);
    setVisible(elements.d3History, false);
    setVisible(elements.d3WriteStatus, false);
    clearText(elements.d3WriteStatus);
  }

  function renderD2Gate(media, itemID, multipleSources) {
    d2.itemID = itemID;
    d2.media = media;
    d2.multipleSources = multipleSources;
    setWriteCapabilities(media);
    setVisible(elements.d2Panel, true);
    setVisible(elements.d2Actions, false);
    setVisible(elements.d2Results, false);
    setVisible(elements.d2Preview, false);
    clear(elements.d2Candidates);
    clearText(elements.d2ResultStatus);
    clearError(elements.d2Status);

    if (!d2.remoteSearchEnabled) {
      d2.candidates = [];
      clearPreview();
      setText(elements.d2Status, "远程搜索未启用，搜索、Fetch 和预览控件暂不可用。");
      return;
    }
    if (!media || (media.type !== "Movie" && media.type !== "Episode") || !media.media_source_id) {
      setText(elements.d2Status, multipleSources ? safeMessages.media_source_selection_required : "请选择一个有效的 Movie 或 Episode 详情。");
      return;
    }
    setText(elements.d2Status, "当前媒体源：" + (media.media_source_name || "已选择") + "。可以搜索 zh-CN 候选。");
    setVisible(elements.d2Actions, true);
  }

  function markRemoteSearchDisabled(error) {
    if (error && error.code === "remote_search_disabled") {
      d2.remoteSearchEnabled = false;
      renderD2Gate(d2.media, d2.itemID, d2.multipleSources);
    }
  }

  async function searchD2() {
    if (!d2.remoteSearchEnabled || !d2.media || !d2.media.media_source_id || !d2.itemID) return;
    const requestID = d2.searchRequestID + 1;
    d2.searchRequestID = requestID;
    elements.d2Search.disabled = true;
    clearError(elements.d2Status);
    setText(elements.d2Status, "搜索候选…");
    d2.candidates = [];
    clear(elements.d2Candidates);
    clearText(elements.d2ResultStatus);
    setVisible(elements.d2Results, true);
    clearPreview();
    if (d2.inventory) renderSubtitles(elements.detail, d2.inventory);
    try {
      const response = await apiPost("/v1/media/" + encodeURIComponent(d2.itemID) + "/subtitles/search", {
        media_source_id: d2.media.media_source_id,
        language: "zh-CN",
        forced: elements.d2Forced.checked
      });
      if (requestID !== d2.searchRequestID) return;
      d2.candidates = Array.isArray(response.candidates) ? response.candidates.map((candidate) => ({
        token: candidate.token,
        provider: candidate.provider,
        name: candidate.name,
        language: candidate.language,
        format: candidate.format,
        comment: candidate.comment,
        is_hash_match: candidate.is_hash_match === true,
        score: candidate.score,
        state: candidate.state || "ready",
        expires_at: candidate.expires_at,
        errorMessage: ""
      })) : [];
      renderCandidates();
      setText(elements.d2Status, d2.candidates.length ? "搜索完成，请逐个选择候选获取预览。" : "搜索完成，没有可用候选。");
    } catch (error) {
      if (requestID !== d2.searchRequestID) return;
      markRemoteSearchDisabled(error);
      setError(elements.d2Status, error);
      setVisible(elements.d2Results, false);
    } finally {
      if (requestID === d2.searchRequestID) elements.d2Search.disabled = false;
    }
  }

  async function uploadD2() {
    if (d3.uploadBusy || !d2.remoteSearchEnabled || !d2.itemID || !d2.media || !d2.media.media_source_id) return;
    const file = elements.d2UploadFile.files && elements.d2UploadFile.files[0];
    if (!file) {
      setText(elements.d2UploadStatus, "请选择一个 .srt、.ass 或 .ssa 字幕文件。");
      return;
    }
    clearPreview();
    d3.uploadBusy = true;
    elements.d2Upload.disabled = true;
    clearError(elements.d2UploadStatus);
    setText(elements.d2UploadStatus, "正在校验本地字幕…");
    try {
      const body = new FormData();
      body.append("file", file);
      body.append("media_source_id", d2.media.media_source_id);
      body.append("language", "zh-CN");
      const response = await apiMultipart("/v1/media/" + encodeURIComponent(d2.itemID) + "/subtitles/upload", body);
      d2.artifact = response;
      renderArtifact(response);
      setText(elements.d2UploadStatus, "本地字幕已校验，正在加载纯文本预览。");
      await loadPreview(0, false);
      if (d3.writeEnabled) loadD3History();
    } catch (error) {
      markRemoteSearchDisabled(error);
      setError(elements.d2UploadStatus, error);
    } finally {
      d3.uploadBusy = false;
      elements.d2Upload.disabled = false;
      elements.d2UploadFile.value = "";
    }
  }

  async function fetchCandidate(candidate) {
    if (candidate.state !== "ready" || !d2.itemID) return;
    const itemID = d2.itemID;
    candidate.state = "fetching";
    candidate.errorMessage = "";
    const sequence = (candidate.requestSequence || 0) + 1;
    candidate.requestSequence = sequence;
    renderCandidates();
    clearError(elements.d2Status);
    setText(elements.d2Status, "获取所选候选并校验字幕…");
    try {
      const response = await apiPost("/v1/media/" + encodeURIComponent(itemID) + "/subtitles/fetch", {
        candidate_token: candidate.token
      });
      if (candidate.requestSequence !== sequence || d2.itemID !== itemID) return;
      candidate.state = "fetched";
      candidate.artifactToken = response.artifact_token;
      d2.artifact = response;
      renderCandidates();
      renderArtifact(response);
      setText(elements.d2Status, "字幕已校验，正在加载纯文本预览。");
      await loadPreview(0, false);
    } catch (error) {
      if (candidate.requestSequence !== sequence || d2.itemID !== itemID) return;
      candidate.state = error.code === "candidate_expired" || error.code === "candidate_invalid" ? "expired" : "failed";
      candidate.errorMessage = error.message;
      renderCandidates();
      markRemoteSearchDisabled(error);
      setError(elements.d2Status, error);
    }
  }

  function renderArtifact(artifact) {
    setVisible(elements.d2Preview, true);
    setText(elements.d2ArtifactMeta, "Provider：" + (artifact.provider || "—") + " · 语言：" + (artifact.language || "—") + " · 格式：" + (artifact.format || "—") + " · 字节：" + Number(artifact.byte_length || 0) + " · Cue：" + Number(artifact.cue_count || 0) + " · 过期：" + formatDate(artifact.expires_at));
    clear(elements.d2Cues);
    setText(elements.d2PreviewStatus, "准备预览…");
    setVisible(elements.d3Add, d3.writeEnabled && d3.writeCapabilities.add && Boolean(d2.media && d2.media.media_source_id));
    clearText(elements.d3AddStatus);
    if (d2.inventory) renderSubtitles(elements.detail, d2.inventory);
  }

  function newOperationID() {
    if (window.crypto && typeof window.crypto.randomUUID === "function") return window.crypto.randomUUID();
    return "op-" + Date.now().toString(36) + "-" + Math.random().toString(36).slice(2);
  }

  async function addD3() {
    if (d3.addBusy || !d3.writeEnabled || !d3.writeCapabilities.add || !d2.artifact || !d2.itemID || !d2.media || !d2.media.media_source_id) return;
    d3.addBusy = true;
    elements.d3AddButton.disabled = true;
    clearError(elements.d3AddStatus);
    setText(elements.d3AddStatus, "正在写入、刷新并核验…");
    try {
      await apiPost("/v1/media/" + encodeURIComponent(d2.itemID) + "/subtitles/add", {
        artifact_token: d2.artifact.artifact_token,
        media_source_id: d2.media.media_source_id,
        operation_id: newOperationID()
      });
      setText(elements.appStatus, "字幕已添加，Emby 已刷新并确认可见。");
      await reloadCurrentDetail();
    } catch (error) {
      setError(elements.d3AddStatus, error);
    } finally {
      d3.addBusy = false;
      elements.d3AddButton.disabled = false;
    }
  }

  async function replaceSubtitle(subtitle) {
    if (d3.operationBusy || !d3.writeEnabled || !d3.writeCapabilities.replace || !d2.artifact || !d2.itemID || !d2.media || !d2.media.media_source_id || !subtitle || !subtitle.id) return;
    const itemID = d2.itemID;
    const sourceID = d2.media.media_source_id;
    d3.operationBusy = true;
    setText(elements.appStatus, "正在替换、刷新并核验字幕…");
    if (d2.inventory) renderSubtitles(elements.detail, d2.inventory);
    try {
      await apiPost("/v1/media/" + encodeURIComponent(itemID) + "/subtitles/" + encodeURIComponent(subtitle.id) + "/replace", {
        artifact_token: d2.artifact.artifact_token,
        media_source_id: sourceID,
        operation_id: newOperationID()
      });
      setText(elements.appStatus, "字幕已替换，旧版本已归档，可在操作历史中恢复。");
      await reloadCurrentDetail();
    } catch (error) {
      setError(elements.appStatus, error);
    } finally {
      d3.operationBusy = false;
      if (d2.inventory) renderSubtitles(elements.detail, d2.inventory);
    }
  }

  async function deleteSubtitle(subtitle) {
    if (d3.operationBusy || !d3.writeEnabled || !d3.writeCapabilities.delete || !d2.itemID || !d2.media || !d2.media.media_source_id || !subtitle || !subtitle.id) return;
    if (!window.confirm("此操作会将该字幕移入可恢复回收区，不会永久删除。继续吗？")) return;
    const itemID = d2.itemID;
    const sourceID = d2.media.media_source_id;
    d3.operationBusy = true;
    setText(elements.appStatus, "正在移入回收区、刷新并核验…");
    if (d2.inventory) renderSubtitles(elements.detail, d2.inventory);
    try {
      await apiPost("/v1/media/" + encodeURIComponent(itemID) + "/subtitles/" + encodeURIComponent(subtitle.id) + "/delete", {
        media_source_id: sourceID,
        operation_id: newOperationID()
      });
      setText(elements.appStatus, "字幕已移入回收区，可在操作历史中恢复。");
      await reloadCurrentDetail();
    } catch (error) {
      setError(elements.appStatus, error);
    } finally {
      d3.operationBusy = false;
      if (d2.inventory) renderSubtitles(elements.detail, d2.inventory);
    }
  }

  function operationLabel(value) {
    switch (value) {
      case "add": return "添加";
      case "upload": return "上传校验";
      case "replace": return "替换";
      case "delete": return "移入回收区";
      case "restore": return "恢复";
      default: return "操作";
    }
  }

  function renderD3History() {
    clear(elements.d3HistoryList);
    if (!Array.isArray(d3.history) || d3.history.length === 0) {
      setText(elements.d3HistoryStatus, "当前媒体还没有可展示的操作记录。");
      return;
    }
    const typeFilter = elements.d3HistoryType.value;
    const statusFilter = elements.d3HistoryStatusFilter.value;
    const history = d3.history.filter((operation) => {
      if (typeFilter && operation.type !== typeFilter) return false;
      if (statusFilter && operation.status !== statusFilter) return false;
      return true;
    });
    setText(elements.d3HistoryStatus, "显示 " + history.length + " / " + d3.history.length + " 条操作记录。");
    if (history.length === 0) {
      setText(elements.d3HistoryList, "没有符合筛选条件的操作记录。");
      return;
    }
    history.forEach((operation) => {
      const card = document.createElement("article");
      card.className = "history-item";
      addText(card, operationLabel(operation.type), "type-badge");
      addText(card, operation.status === "verified" ? "已核验" : "已校验", "status-badge");
      if (operation.file_name) addText(card, operation.file_name, "item-title");
      const facts = document.createElement("p");
      facts.className = "muted";
      const values = [];
      if (operation.language) values.push(operation.language);
      if (operation.format) values.push(operation.format);
      if (operation.created_at) values.push(formatDate(operation.created_at));
      setText(facts, values.join(" · ") || "安全操作摘要");
      card.appendChild(facts);
      if ((operation.type === "replace" || operation.type === "delete") && operation.status === "verified") {
        if (operation.restore_supported === false) {
          addMessages(card, "恢复状态", [safeMessages[operation.restore_error_code] || safeMessages.restore_unavailable], "warning-list");
        } else {
          const restore = document.createElement("button");
          restore.type = "button";
          restore.className = "secondary";
          setText(restore, "恢复旧字幕");
          restore.disabled = d3.operationBusy || !d3.writeCapabilities.restore;
          restore.addEventListener("click", () => restoreOperation(operation));
          card.appendChild(restore);
        }
      }
      elements.d3HistoryList.appendChild(card);
    });
  }

  async function loadD3History() {
    if (!d3.writeEnabled || !d2.itemID || !d2.media || !d2.media.media_source_id) {
      setVisible(elements.d3History, false);
      return;
    }
    const requestID = ++d3.historyRequestID;
    setVisible(elements.d3History, true);
    if (!d3.writeCapabilities.restore) {
      d3.history = [];
      clear(elements.d3HistoryList);
      elements.d3HistoryReload.disabled = true;
      setText(elements.d3HistoryStatus, safeMessages[d3.writeCapabilities.reason_code] || "当前媒体暂不支持安全恢复操作。");
      return;
    }
    elements.d3HistoryReload.disabled = false;
    clearError(elements.d3HistoryStatus);
    setText(elements.d3HistoryStatus, "加载操作历史…");
    try {
      const response = await apiGet("/v1/subtitle-operations?item_id=" + encodeURIComponent(d2.itemID) + "&media_source_id=" + encodeURIComponent(d2.media.media_source_id));
      if (requestID !== d3.historyRequestID) return;
      const operations = Array.isArray(response.operations) ? response.operations : [];
      d3.history = operations.filter((operation) => operation && operation.media_source_id === d2.media.media_source_id);
      renderD3History();
    } catch (error) {
      if (requestID !== d3.historyRequestID) return;
      d3.history = [];
      clear(elements.d3HistoryList);
      setError(elements.d3HistoryStatus, error);
    }
  }

  async function restoreOperation(operation) {
    if (d3.operationBusy || !d3.writeCapabilities.restore || !operation || operation.restore_supported === false || !operation.operation_id || !d2.media || !d2.media.media_source_id) return;
    if (!window.confirm("恢复不会覆盖同名现有字幕；发生冲突时会安全拒绝。继续吗？")) return;
    d3.operationBusy = true;
    setText(elements.appStatus, "正在恢复、刷新并核验字幕…");
    renderD3History();
    try {
      await apiPost("/v1/subtitle-operations/" + encodeURIComponent(operation.operation_id) + "/restore", {
        media_source_id: d2.media.media_source_id,
        operation_id: newOperationID()
      });
      setText(elements.appStatus, "旧字幕已恢复，Emby 已刷新并确认可见。");
      await reloadCurrentDetail();
    } catch (error) {
      setError(elements.appStatus, error);
    } finally {
      d3.operationBusy = false;
      renderD3History();
    }
  }

  async function reloadCurrentDetail() {
    if (!d2.itemID || !d2.media || !d2.media.media_source_id) return;
    await loadDetail(d2.itemID, d2.media.media_source_id);
  }

  function formatCueTime(value) {
    const milliseconds = Math.max(0, Number(value) || 0);
    const hours = Math.floor(milliseconds / 3600000);
    const minutes = Math.floor((milliseconds % 3600000) / 60000);
    const seconds = Math.floor((milliseconds % 60000) / 1000);
    const millis = Math.floor(milliseconds % 1000);
    return String(hours).padStart(2, "0") + ":" + String(minutes).padStart(2, "0") + ":" + String(seconds).padStart(2, "0") + "," + String(millis).padStart(3, "0");
  }

  function renderPreviewPage(response) {
    clearError(elements.d2PreviewStatus);
    d2.previewOffset = Number(response.offset || 0);
    d2.previewLimit = Number(response.limit || d2.previewLimit);
    d2.previewCues = Array.isArray(response.cues) ? response.cues : [];
    d2.previewHasNext = response.truncated === true;
    clear(elements.d2Cues);
    d2.previewCues.forEach((cue) => {
      const card = document.createElement("article");
      card.className = "cue";
      const time = document.createElement("span");
      time.className = "cue-time";
      setText(time, formatCueTime(cue.start_ms) + " → " + formatCueTime(cue.end_ms));
      card.appendChild(time);
      const text = document.createElement("p");
      text.className = "cue-text";
      setText(text, cue.text);
      card.appendChild(text);
      elements.d2Cues.appendChild(card);
    });
    const first = d2.previewCues.length ? d2.previewOffset + 1 : d2.previewOffset;
    const last = d2.previewOffset + d2.previewCues.length;
    const total = Number(response.cue_count || 0);
    const range = d2.previewCues.length ? "第 " + first + "-" + last + " 条 / " + total + " 条" : "当前页没有 Cue · 共 " + total + " 条";
    setText(elements.d2PreviewStatus, range + (d2.previewHasNext ? " · 还有下一页" : ""));
    elements.d2PreviewPrevious.disabled = d2.previewBusy || d2.previewOffsets.length <= 1;
    elements.d2PreviewNext.disabled = d2.previewBusy || !d2.previewHasNext || d2.previewCues.length === 0;
    elements.d2PreviewReset.disabled = d2.previewBusy || (d2.previewOffset === 0 && d2.previewOffsets.length <= 1);
  }

  async function loadPreview(offset, addToHistory) {
    if (!d2.artifact || !d2.artifact.artifact_token || !d2.itemID) return;
    const requestID = ++d2.previewRequestID;
    d2.previewBusy = true;
    elements.d2PreviewPrevious.disabled = true;
    elements.d2PreviewNext.disabled = true;
    elements.d2PreviewReset.disabled = true;
    setText(elements.d2PreviewStatus, "加载预览…");
    try {
      const response = await apiPost("/v1/media/" + encodeURIComponent(d2.itemID) + "/subtitles/preview", {
        artifact_token: d2.artifact.artifact_token,
        offset: Math.max(0, Number(offset) || 0),
        limit: d2.previewLimit
      });
      if (requestID !== d2.previewRequestID) return;
      if (addToHistory && d2.previewOffsets[d2.previewOffsets.length - 1] !== response.offset) d2.previewOffsets.push(response.offset);
      d2.previewBusy = false;
      renderPreviewPage(response);
    } catch (error) {
      if (requestID !== d2.previewRequestID) return;
      d2.previewBusy = false;
      clear(elements.d2Cues);
      setError(elements.d2PreviewStatus, error);
      elements.d2PreviewPrevious.disabled = true;
      elements.d2PreviewNext.disabled = true;
      elements.d2PreviewReset.disabled = false;
      if (error.code === "artifact_expired" || error.code === "artifact_invalid") {
        d2.candidates.forEach((candidate) => {
          if (candidate.artifactToken === d2.artifact.artifact_token) {
            candidate.state = "expired";
            candidate.errorMessage = error.message;
          }
        });
        renderCandidates();
      }
      markRemoteSearchDisabled(error);
    }
  }

  function previewNext() {
    const nextOffset = d2.previewOffset + d2.previewCues.length;
    if (!d2.previewHasNext || d2.previewCues.length === 0 || nextOffset <= d2.previewOffset) return;
    loadPreview(nextOffset, true);
  }

  function previewPrevious() {
    if (d2.previewOffsets.length <= 1) return;
    d2.previewOffsets.pop();
    const previousOffset = d2.previewOffsets[d2.previewOffsets.length - 1];
    loadPreview(previousOffset, false);
  }

  function previewReset() {
    d2.previewOffsets = [0];
    loadPreview(0, false);
  }

  function changePreviewLimit() {
    const value = Number(elements.d2PreviewLimit.value);
    d2.previewLimit = value === 500 ? 500 : 200;
    d2.previewOffsets = [0];
    loadPreview(0, false);
  }

  async function loadDetail(itemID, mediaSourceID) {
    const requestID = ++detailRequestID;
    resetD2ForItem(itemID, Boolean(mediaSourceID && knownMultiSourceItems.has(itemID)));
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
      renderD2Gate(media, itemID, Boolean(mediaSourceID && knownMultiSourceItems.has(itemID)));
      d2.inventory = subtitles.inventory || {};
      renderSubtitles(elements.detail, d2.inventory);
      if (d3.writeEnabled) loadD3History();
    } catch (error) {
      if (requestID !== detailRequestID) return;
      clear(elements.detail);
      elements.detail.className = "";
      if (Array.isArray(error.mediaSources)) {
        knownMultiSourceItems.add(itemID);
        const heading = document.createElement("p");
        setText(heading, "此媒体有多个媒体源，请明确选择一个版本后再进行字幕操作。");
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
    addDetailRow(grid, "媒体源", media.media_source_name || (mediaSourceID || media.media_source_id ? "已选择" : "未选择"));
    addDetailRow(grid, "映射状态", media.mapping_status);
    elements.detail.appendChild(grid);
    renderWriteCapabilitySummary(elements.detail, media);
    addStatus(elements.detail, "媒体上下文完整性", media.inventory_complete ? "完整" : "不完整", media.inventory_complete === true);
    addMessages(elements.detail, "Warnings", media.warnings, "warning-list");
  }

  function renderWriteCapabilitySummary(parent, media) {
    const section = document.createElement("section");
    section.className = "capability-summary";
    const heading = document.createElement("h4");
    setText(heading, "字幕操作能力");
    section.appendChild(heading);
    const hint = document.createElement("p");
    hint.className = "muted";
    setText(hint, d3.writeEnabled ? "写入窗口已启用；实际操作仍会由服务端重新核验。" : "写入窗口当前关闭；此处仅显示当前媒体的安全能力。" );
    section.appendChild(hint);
    const list = document.createElement("ul");
    list.className = "capability-list";
    const capabilities = media && media.write_capabilities ? media.write_capabilities : emptyWriteCapabilities();
    for (const entry of [["添加", capabilities.add], ["替换", capabilities.replace], ["移入回收区", capabilities.delete], ["恢复", capabilities.restore]]) {
      const item = document.createElement("li");
      addText(item, entry[0] + "：");
      addText(item, entry[1] === true ? "可用" : "不可用", entry[1] === true ? "success" : "");
      list.appendChild(item);
    }
    section.appendChild(list);
    const reason = typeof capabilities.reason_code === "string" ? capabilities.reason_code : "";
    if (reason && safeMessages[reason]) addMessages(section, "限制说明", [safeMessages[reason]], "warning-list");
    parent.appendChild(section);
  }

  function setRemoteSearchFeature(health) {
    const features = health && health.features ? health.features : {};
    d2.remoteSearchEnabled = features.remote_search_enabled === true;
    d3.writeEnabled = features.write_enabled === true;
    const embyStatus = health && health.emby_status ? health.emby_status : "unknown";
    const embyLabel = embyStatus === "ready" ? "Emby 就绪" : embyStatus === "unknown" ? "Emby 状态未知" : "Emby " + embyStatus;
    const searchLabel = d2.remoteSearchEnabled ? "远程搜索已开启" : "远程搜索已关闭";
    const writeLabel = d3.writeEnabled ? "写入已开启" : "写入已关闭";
    setText(elements.healthSummary, embyLabel + " · " + searchLabel + " · " + writeLabel);
  }

  async function refreshHealth() {
    elements.refreshHealth.disabled = true;
    try {
      const health = await apiGet("/v1/health");
      setRemoteSearchFeature(health);
      setText(elements.appStatus, "运行状态已刷新。");
    } catch (error) {
      setError(elements.appStatus, error);
    } finally {
      elements.refreshHealth.disabled = false;
    }
  }

  async function login(event) {
    event.preventDefault();
    clearError(elements.loginError);
    const username = elements.username.value;
    const password = elements.password.value;
    elements.password.value = "";
    if (!username || !password) {
      setText(elements.loginError, "请输入管理员用户名和密码。");
      return;
    }
    try {
      const loginResponse = await apiPost("/v1/auth/login", { username, password });
      d3.csrfToken = loginResponse && typeof loginResponse.csrf_token === "string" ? loginResponse.csrf_token : "";
      const health = await apiGet("/v1/health");
      setRemoteSearchFeature(health);
      elements.refreshHealth.disabled = false;
      const libraries = await apiGet("/v1/emby/libraries");
      fillLibraries(Array.isArray(libraries) ? libraries : []);
      resetBrowse();
      elements.loginPanel.classList.add("hidden");
      elements.appPanel.classList.remove("hidden");
      if (selectedLibrary) await loadItems();
    } catch (error) {
      setError(elements.loginError, error);
    }
  }

  elements.loginForm.addEventListener("submit", login);
  elements.library.addEventListener("change", () => {
    selectedLibrary = elements.library.value;
    detailRequestID += 1;
    resetD2ForItem("", false);
    clear(elements.detail);
    elements.detail.className = "empty-state";
    setText(elements.detail, "选择一个 Movie 或 Episode 查看详情。");
    resetBrowse();
    loadItems();
  });
  elements.loadItems.addEventListener("click", () => loadItems());
  elements.refreshHealth.addEventListener("click", refreshHealth);
  elements.d3HistoryType.addEventListener("change", renderD3History);
  elements.d3HistoryStatusFilter.addEventListener("change", renderD3History);
  elements.browseBack.addEventListener("click", () => {
    if (browse.mode === "sources") {
      browse.mode = "nodes";
      renderBrowsePath();
      loadItems();
      return;
    }
    if (browse.crumbs.length > 1) {
      browse.crumbs.pop();
      applyBrowseCrumb();
    }
  });
  elements.previousPage.addEventListener("click", () => {
    startIndex = Math.max(0, startIndex - currentLimit);
    loadItems();
  });
  elements.nextPage.addEventListener("click", () => {
    startIndex += currentLimit;
    loadItems();
  });
  elements.d2Search.addEventListener("click", () => searchD2());
  elements.d2Upload.addEventListener("click", () => uploadD2());
  elements.d2PreviewLimit.addEventListener("change", () => changePreviewLimit());
  elements.d2PreviewPrevious.addEventListener("click", () => previewPrevious());
  elements.d2PreviewNext.addEventListener("click", () => previewNext());
  elements.d2PreviewReset.addEventListener("click", () => previewReset());
  elements.d3AddButton.addEventListener("click", () => addD3());
  elements.d3HistoryReload.addEventListener("click", () => loadD3History());
})();
