(() => {
  "use strict";

  let authToken = "";
  let selectedLibrary = "";
  let startIndex = 0;
  let currentLimit = 50;
  let listRequestID = 0;
  let detailRequestID = 0;
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
    previewLimit: 200
  };

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
    pageStatus: document.getElementById("page-status"),
    d2Panel: document.getElementById("d2-panel"),
    d2Status: document.getElementById("d2-status"),
    d2Actions: document.getElementById("d2-actions"),
    d2Forced: document.getElementById("d2-forced"),
    d2Search: document.getElementById("d2-search"),
    d2Results: document.getElementById("d2-results"),
    d2ResultStatus: document.getElementById("d2-result-status"),
    d2Candidates: document.getElementById("d2-candidates"),
    d2Preview: document.getElementById("d2-preview"),
    d2ArtifactMeta: document.getElementById("d2-artifact-meta"),
    d2PreviewLimit: document.getElementById("d2-preview-limit"),
    d2PreviewPrevious: document.getElementById("d2-preview-previous"),
    d2PreviewNext: document.getElementById("d2-preview-next"),
    d2PreviewReset: document.getElementById("d2-preview-reset"),
    d2PreviewStatus: document.getElementById("d2-preview-status"),
    d2Cues: document.getElementById("d2-cues")
  };

  const safeMessages = {
    unauthorized: "登录已失效，请重新登录。",
    remote_search_disabled: "远程搜索未启用，请联系管理员确认 D2 Canary 状态。",
    canary_item_not_allowed: "当前媒体未获准进行 D2 预览，请选择已授权的单源 Movie 或 Episode。",
    media_not_found: "媒体不存在或已不可用。",
    candidate_invalid: "候选已失效，请重新搜索。",
    artifact_invalid: "预览已失效，请重新获取候选。",
    candidate_expired: "候选已过期，请重新搜索。",
    artifact_expired: "预览已过期，请重新获取候选。",
    d2_multisource_unsupported: "当前媒体包含多个媒体源，D2 只支持单源 Movie 或 Episode。",
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
    media_source_required: "请先选择一个媒体源。"
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

  async function apiGet(resource) {
    if (!authToken) throw makeError("unauthorized", 401, 0);
    const response = await fetch(resource, {
      method: "GET",
      headers: { Authorization: "Bearer " + authToken },
      cache: "no-store"
    });
    const payload = await parseResponse(response);
    if (!response.ok) {
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
    if (!authToken) throw makeError("unauthorized", 401, 0);
    const response = await fetch(resource, {
      method: "POST",
      headers: {
        Authorization: "Bearer " + authToken,
        "Content-Type": "application/json"
      },
      body: JSON.stringify(body),
      cache: "no-store"
    });
    const payload = await parseResponse(response);
    if (!response.ok) {
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

  function renderCandidates() {
    clear(elements.d2Candidates);
    d2.candidates.forEach((candidate) => elements.d2Candidates.appendChild(candidateCard(candidate)));
    const count = d2.candidates.length;
    setText(elements.d2ResultStatus, count ? "显示 " + count + " 个候选" : "没有候选");
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
    elements.d2PreviewPrevious.disabled = true;
    elements.d2PreviewNext.disabled = true;
  }

  function resetD2ForItem(itemID, multipleSources) {
    d2.itemID = itemID;
    d2.media = null;
    d2.multipleSources = multipleSources;
    d2.candidates = [];
    d2.searchRequestID += 1;
    clear(elements.d2Candidates);
    clearText(elements.d2ResultStatus);
    setVisible(elements.d2Results, false);
    clearPreview();
    setVisible(elements.d2Panel, false);
    clearError(elements.d2Status);
    setVisible(elements.d2Actions, false);
  }

  function renderD2Gate(media, itemID, multipleSources) {
    d2.itemID = itemID;
    d2.media = media;
    d2.multipleSources = multipleSources;
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
    if (multipleSources) {
      setText(elements.d2Status, safeMessages.d2_multisource_unsupported);
      return;
    }
    if (!media || (media.type !== "Movie" && media.type !== "Episode") || !media.media_source_id) {
      setText(elements.d2Status, "请选择一个有效的单源 Movie 或 Episode 详情。");
      return;
    }
    setText(elements.d2Status, "已选定单源媒体，可以搜索 zh-CN 候选。");
    setVisible(elements.d2Actions, true);
  }

  function markRemoteSearchDisabled(error) {
    if (error && error.code === "remote_search_disabled") {
      d2.remoteSearchEnabled = false;
      renderD2Gate(d2.media, d2.itemID, d2.multipleSources);
    }
  }

  async function searchD2() {
    if (!d2.remoteSearchEnabled || !d2.media || d2.multipleSources || !d2.itemID) return;
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
      renderSubtitles(elements.detail, subtitles.inventory || {});
      renderD2Gate(media, itemID, Boolean(mediaSourceID && knownMultiSourceItems.has(itemID)));
    } catch (error) {
      if (requestID !== detailRequestID) return;
      clear(elements.detail);
      elements.detail.className = "";
      if (Array.isArray(error.mediaSources)) {
        knownMultiSourceItems.add(itemID);
        const heading = document.createElement("p");
        setText(heading, "此媒体有多个媒体源，请明确选择后查看详情。D2 只支持单源媒体。");
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

  function setRemoteSearchFeature(health) {
    const features = health && health.features ? health.features : {};
    d2.remoteSearchEnabled = features.remote_search_enabled === true;
  }

  async function login(event) {
    event.preventDefault();
    authToken = elements.token.value;
    elements.token.value = "";
    clearError(elements.loginError);
    if (!authToken) {
      setText(elements.loginError, "请输入 Token");
      return;
    }
    try {
      const health = await apiGet("/v1/health");
      setRemoteSearchFeature(health);
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
    resetD2ForItem("", false);
    clear(elements.items);
    clear(elements.detail);
    setText(elements.items, "请选择媒体库。");
    setText(elements.detail, "选择一个 Movie 或 Episode 查看详情。");
    elements.items.className = "item-list empty-state";
    elements.detail.className = "empty-state";
    elements.loginPanel.classList.remove("hidden");
    elements.appPanel.classList.add("hidden");
    elements.logout.classList.add("hidden");
    elements.token.value = "";
    clearError(elements.loginError);
    clearText(elements.appStatus);
  }

  elements.loginForm.addEventListener("submit", login);
  elements.logout.addEventListener("click", logout);
  elements.library.addEventListener("change", () => {
    selectedLibrary = elements.library.value;
    detailRequestID += 1;
    resetD2ForItem("", false);
    clear(elements.detail);
    elements.detail.className = "empty-state";
    setText(elements.detail, "选择一个 Movie 或 Episode 查看详情。");
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
  elements.d2Search.addEventListener("click", () => searchD2());
  elements.d2PreviewLimit.addEventListener("change", () => changePreviewLimit());
  elements.d2PreviewPrevious.addEventListener("click", () => previewPrevious());
  elements.d2PreviewNext.addEventListener("click", () => previewNext());
  elements.d2PreviewReset.addEventListener("click", () => previewReset());
})();
