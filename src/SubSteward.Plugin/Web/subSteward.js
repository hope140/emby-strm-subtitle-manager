define([], function () {
    "use strict";

    return function (view, params) {
        var pageRoot = view && view.getAttribute && view.getAttribute("data-role") === "page"
            ? view
            : document.querySelector('[data-role="page"][data-controller="__plugin/SubStewardUI3.js"]:not([data-substeward-initialized="true"])')
                || document.querySelector('[data-role="page"][data-controller="__plugin/SubStewardUI3.js"]');
        if (!pageRoot || pageRoot.getAttribute("data-substeward-initialized") === "true") {
            return;
        }

        pageRoot.setAttribute("data-substeward-initialized", "true");

        var PLUGIN_ID = "20b47482-cb89-42d2-a6e0-5b87fd9b7858";
        var DEFAULT_CONFIGURATION = {
            TargetLanguage: "zh-Hans",
            SecondaryLanguage: "eng",
            PreferBilingual: false,
            FormatOrder: "ass,ssa,srt",
            LibraryOverrides: []
        };
        var FORMAT_ORDER_CHOICES = [
            "ass,ssa,srt",
            "ass,srt,ssa",
            "ssa,ass,srt",
            "ssa,srt,ass",
            "srt,ass,ssa",
            "srt,ssa,ass"
        ];
        var LANGUAGE_ALIASES = {
            "zho": "zho",
            "zh": "zho",
            "chi": "zho",
            "zh-hans": "zh-Hans",
            "hans": "zh-Hans",
            "chs": "zh-Hans",
            "gb": "zh-Hans",
            "gbk": "zh-Hans",
            "sc": "zh-Hans",
            "simplified": "zh-Hans",
            "simplified-chinese": "zh-Hans",
            "zh-cn": "zh-Hans",
            "zh-sg": "zh-Hans",
            "zh-my": "zh-Hans",
            "简体": "zh-Hans",
            "简": "zh-Hans",
            "zh-hant": "zh-Hant",
            "hant": "zh-Hant",
            "cht": "zh-Hant",
            "big5": "zh-Hant",
            "tc": "zh-Hant",
            "traditional": "zh-Hant",
            "traditional-chinese": "zh-Hant",
            "zh-tw": "zh-Hant",
            "zh-hk": "zh-Hant",
            "zh-mo": "zh-Hant",
            "繁体": "zh-Hant",
            "繁體": "zh-Hant",
            "繁": "zh-Hant",
            "eng": "eng",
            "en": "eng",
            "english": "eng",
            "英语": "eng",
            "jpn": "jpn",
            "ja": "jpn",
            "japanese": "jpn",
            "日语": "jpn"
        };
        var state = {
            configuration: Object.assign({}, DEFAULT_CONFIGURATION),
            libraries: [],
            items: [],
            selectedLibrary: null,
            selectedItem: null,
            selectedSourceId: null,
            candidates: [],
            artifact: null,
            alignmentHistory: [],
            activeTab: "status"
        };

        function getElement(id) {
            return pageRoot.querySelector("#" + id);
        }

        function getHostApi() {
            try {
                if (window.ApiClient) return window.ApiClient;
                if (window.parent && window.parent.ApiClient) return window.parent.ApiClient;
            } catch (error) {
                return null;
            }
            return null;
        }

        function getRequestHeaders() {
            var api = getHostApi();
            var headers = { Accept: "application/json" };
            if (api && typeof api.getRequestHeaders === "function") {
                Object.assign(headers, api.getRequestHeaders());
            } else if (api && typeof api.accessToken === "function") {
                headers["X-Emby-Token"] = api.accessToken();
            }
            return headers;
        }

        function buildUrl(path, query) {
            var api = getHostApi();
            var cleanQuery = {};
            Object.keys(query || {}).forEach(function (key) {
                if (query[key] !== undefined && query[key] !== null && query[key] !== "") {
                    cleanQuery[key] = query[key];
                }
            });
            if (api && typeof api.getUrl === "function") {
                return api.getUrl(path, cleanQuery);
            }
            var url = new URL(path, window.location.origin);
            Object.keys(cleanQuery).forEach(function (key) {
                url.searchParams.set(key, cleanQuery[key]);
            });
            return url.toString();
        }

        async function request(path, options) {
            var requestOptions = options || {};
            var headers = getRequestHeaders();
            var body = requestOptions.body;
            if (body !== undefined && body !== null && typeof body !== "string") {
                body = JSON.stringify(body);
                headers["Content-Type"] = "application/json";
            }
            var controller = null;
            var timer = null;
            if (typeof AbortController !== "undefined") {
                controller = new AbortController();
                timer = setTimeout(function () {
                    controller.abort();
                }, requestOptions.timeoutMs || 60000);
            }
            try {
                var response = await fetch(buildUrl(path, requestOptions.query), {
                    method: requestOptions.method || "GET",
                    headers: headers,
                    body: body,
                    credentials: "same-origin",
                    signal: controller ? controller.signal : undefined
                });
                var text = await response.text();
                var payload = null;
                if (text) {
                    try {
                        payload = JSON.parse(text);
                    } catch (error) {
                        payload = text;
                    }
                }
                if (!response.ok) {
                    throw new Error(payload && payload.Message
                        ? payload.Message
                        : "Emby 请求失败（HTTP " + response.status + "）。");
                }
                return payload;
            } catch (error) {
                if (controller && controller.signal.aborted) {
                    throw new Error("Emby 请求超时，请稍后重试。");
                }
                throw error;
            } finally {
                if (timer) clearTimeout(timer);
            }
        }

        function escapeHtml(value) {
            return String(value === undefined || value === null ? "" : value)
                .replace(/&/g, "&amp;")
                .replace(/</g, "&lt;")
                .replace(/>/g, "&gt;")
                .replace(/"/g, "&quot;")
                .replace(/'/g, "&#039;");
        }

        function formatCount(value) {
            return Number(value || 0).toLocaleString("zh-CN");
        }

        function formatPercent(value) {
            return Math.round(Number(value || 0) * 100) + "%";
        }

        function formatMilliseconds(value) {
            var total = Math.max(0, Math.round(Number(value || 0) / 1000));
            var hours = Math.floor(total / 3600);
            var minutes = Math.floor((total % 3600) / 60);
            var seconds = total % 60;
            return (hours ? String(hours).padStart(2, "0") + ":" : "")
                + String(minutes).padStart(2, "0") + ":"
                + String(seconds).padStart(2, "0");
        }

        function actionName(item) {
            return item && item.Action && item.Action.Action ? item.Action.Action : "MANUAL";
        }

        function actionClass(action) {
            if (action === "KEEP") return "ss-chip-good";
            if (action === "SEARCH" || action === "REPAIR" || action === "UPGRADE") return "ss-chip-warning";
            if (action === "IGNORE") return "ss-chip-danger";
            return "";
        }

        function healthClass(health) {
            if (health === "PASS") return "ss-chip-good";
            if (health === "FAIL") return "ss-chip-danger";
            return "ss-chip-warning";
        }

        function itemPresence(item) {
            var sources = item && Array.isArray(item.MediaSources) ? item.MediaSources : [];
            if (!sources.length) return null;
            return sources.some(function (source) {
                return source.Presence && source.Presence.TargetLanguagePresent;
            });
        }

        function setFeedback(id, message, kind) {
            var element = getElement(id);
            if (!element) return;
            element.textContent = message || "";
            if (kind) element.setAttribute("data-kind", kind);
            else element.removeAttribute("data-kind");
        }

        function setButtonBusy(button, busy, busyText) {
            if (!button) return;
            if (busy) {
                button.disabled = true;
                button.dataset.originalText = button.textContent;
                button.textContent = busyText || "处理中…";
            } else {
                button.disabled = false;
                if (button.dataset.originalText) {
                    button.textContent = button.dataset.originalText;
                    delete button.dataset.originalText;
                }
            }
        }

        function switchTab(name, focusPanel) {
            state.activeTab = name;
            pageRoot.querySelectorAll("[data-tab]").forEach(function (button) {
                var selected = button.getAttribute("data-tab") === name;
                button.setAttribute("aria-selected", selected ? "true" : "false");
                button.tabIndex = selected ? 0 : -1;
            });
            ["status", "manual", "settings"].forEach(function (tabName) {
                var panel = getElement("panel" + tabName.charAt(0).toUpperCase() + tabName.slice(1));
                panel.hidden = tabName !== name;
            });
            if (focusPanel) {
                getElement("panel" + name.charAt(0).toUpperCase() + name.slice(1)).focus();
            }
        }

        function switchSettingsPane(name) {
            getElement("globalSettingsPane").hidden = name !== "global";
            getElement("librarySettingsPane").hidden = name !== "library";
            pageRoot.querySelectorAll("[data-settings-pane]").forEach(function (button) {
                button.setAttribute("aria-pressed", button.getAttribute("data-settings-pane") === name ? "true" : "false");
            });
        }

        function normalizeLanguageChoice(value) {
            var normalized = String(value || "").trim().toLowerCase().replace(/_/g, "-");
            return LANGUAGE_ALIASES[normalized] || "";
        }

        function normalizeFormatOrderChoice(value) {
            var normalized = String(value || "").trim().toLowerCase();
            var parts = normalized.split(/[;,]/).map(function (part) {
                return part.trim().replace(/^\./, "");
            }).filter(function (part) { return Boolean(part); });
            if (parts.length !== 3) return "";
            if (parts.indexOf("subrip") >= 0) {
                parts[parts.indexOf("subrip")] = "srt";
            }
            var canonical = parts.join(",");
            return FORMAT_ORDER_CHOICES.indexOf(canonical) >= 0 ? canonical : "";
        }

        function removeLegacyChoice(select) {
            if (!select) return;
            Array.prototype.slice.call(select.querySelectorAll("option[data-legacy-choice=\"true\"]")).forEach(function (option) {
                option.parentNode.removeChild(option);
            });
        }

        function setChoiceValue(id, rawValue, normalize, fallbackValue) {
            var select = getElement(id);
            if (!select) return;
            removeLegacyChoice(select);
            var raw = String(rawValue || "").trim();
            var value = normalize(raw) || (raw ? "" : fallbackValue);
            var hasValue = Array.prototype.some.call(select.options, function (option) {
                return option.value === value;
            });
            if (hasValue) {
                select.value = value;
                return;
            }
            if (raw) {
                var legacyOption = document.createElement("option");
                legacyOption.value = raw;
                legacyOption.textContent = "当前配置：" + raw + "（旧值，请改选支持项）";
                legacyOption.setAttribute("data-legacy-choice", "true");
                select.insertBefore(legacyOption, select.firstChild);
                select.value = raw;
                return;
            }
            select.value = fallbackValue;
        }

        function renderStatus() {
            var total = state.items.length;
            var present = state.items.filter(function (item) { return itemPresence(item) === true; }).length;
            var missing = state.items.filter(function (item) { return itemPresence(item) === false; }).length;
            var multiSource = state.items.filter(function (item) {
                return Array.isArray(item.MediaSources) && item.MediaSources.length !== 1;
            }).length;
            var requiresManual = state.items.filter(function (item) {
                return actionName(item) === "MANUAL" || !Array.isArray(item.MediaSources) || item.MediaSources.length !== 1;
            }).length;
            getElement("statusScope").textContent = total ? "当前载入前 " + formatCount(total) + " 项" : "没有数据";
            getElement("statusMetrics").innerHTML = [
                metricCard("已载入条目", total, "按当前名称筛选，最多 100 项"),
                metricCard("目标字幕已存在", present, total ? formatPercent(present / total) + " 的当前样本" : "暂无样本"),
                metricCard("目标字幕缺失", missing, "可进入手动工作台搜索"),
                metricCard("需要人工判断", requiresManual, multiSource ? multiSource + " 项不是单 Source" : "包含 Health 未知状态")
            ].join("");

            var groups = ["KEEP", "SEARCH", "MANUAL", "OTHER"].map(function (name) {
                var count = state.items.filter(function (item) {
                    var action = actionName(item);
                    return name === "OTHER"
                        ? ["KEEP", "SEARCH", "MANUAL"].indexOf(action) < 0
                        : action === name;
                }).length;
                return { name: name, count: count };
            });
            getElement("actionBreakdown").innerHTML = groups.map(function (group) {
                var width = total ? Math.round(group.count / total * 100) : 0;
                var fillClass = group.name === "SEARCH" ? "is-warning" : group.name === "OTHER" ? "is-danger" : "";
                return '<div><div class="ss-action-bar-head"><span>' + group.name + '</span><strong>'
                    + formatCount(group.count) + '</strong></div><div class="ss-bar-track"><div class="ss-bar-fill '
                    + fillClass + '" style="width:' + width + '%"></div></div></div>';
            }).join("");

            var attention = state.items.filter(function (item) {
                return actionName(item) !== "KEEP" || (item.MediaSources || []).length !== 1;
            }).slice(0, 8);
            getElement("attentionList").innerHTML = attention.length
                ? attention.map(function (item) {
                    var sourceCount = (item.MediaSources || []).length;
                    return '<button class="ss-attention-row" type="button" data-open-item="' + escapeHtml(item.Id) + '">'
                        + '<span><span class="ss-row-title">' + escapeHtml(item.Name || "未命名条目") + '</span>'
                        + '<span class="ss-row-meta"><span>' + escapeHtml(item.LibraryName || "未归属媒体库") + '</span><span>·</span>'
                        + '<span>' + (sourceCount === 1 ? (itemPresence(item) ? "目标字幕已存在" : "目标字幕缺失") : sourceCount + " 个 Source") + '</span></span></span>'
                        + '<span class="ss-chip ' + actionClass(actionName(item)) + '">' + escapeHtml(actionName(item)) + '</span></button>';
                }).join("")
                : '<div class="ss-muted">当前载入范围内没有待处理条目。</div>';
        }

        function metricCard(label, value, note) {
            return '<div class="ss-card ss-metric-card"><span class="ss-metric-label">' + escapeHtml(label)
                + '</span><div class="ss-metric-value">' + formatCount(value)
                + '</div><div class="ss-metric-note">' + escapeHtml(note) + '</div></div>';
        }

        function renderItems() {
            getElement("itemCount").textContent = formatCount(state.items.length) + " 项";
            getElement("itemList").innerHTML = state.items.length
                ? state.items.map(function (item) {
                    var selected = state.selectedItem && state.selectedItem.Id === item.Id;
                    return '<button class="ss-item-row" type="button" aria-current="' + (selected ? "true" : "false")
                        + '" data-item-id="' + escapeHtml(item.Id) + '"><span><span class="ss-row-title" title="'
                        + escapeHtml(item.Name || "未命名条目") + '">' + escapeHtml(item.Name || "未命名条目") + '</span>'
                        + '<span class="ss-row-meta"><span>' + escapeHtml(item.Type || "媒体") + '</span><span>·</span><span>'
                        + escapeHtml(item.LibraryName || "未归属媒体库") + '</span></span></span><span class="ss-chip '
                        + actionClass(actionName(item)) + '">' + escapeHtml(actionName(item)) + '</span></button>';
                }).join("")
                : '<div class="ss-empty"><div><strong>没有匹配条目</strong><span>修改筛选条件后重试。</span></div></div>';
        }

        function renderExistingSubtitleStreams(source) {
            var streams = source && Array.isArray(source.SubtitleStreams) ? source.SubtitleStreams : [];
            if (!streams.length) {
                return '<div class="ss-muted">详情读取后显示已有字幕深检结果；内封字幕不会提取正文。</div>';
            }

            var targetHealth = source.ExistingTargetHealth || "UNKNOWN";
            var rows = streams.map(function (stream) {
                var role = stream.IsTargetLanguage ? "目标语言" : stream.IsSecondaryLanguage ? "第二语言" : "其他字幕";
                var facts = [];
                if (stream.Format) facts.push(String(stream.Format).toUpperCase());
                if (stream.Encoding) facts.push(stream.Encoding);
                if (stream.Quality && stream.Quality.TargetLanguagePresent) {
                    facts.push("目标覆盖 " + formatPercent(stream.Quality.TargetLanguageConfidence));
                }
                if (!facts.length && Array.isArray(stream.Reasons)) facts = stream.Reasons.slice(0, 1);
                return '<div class="ss-candidate"><div><div class="ss-row-title">' + escapeHtml(stream.Title || role)
                    + '</div><div class="ss-row-meta"><span>' + escapeHtml(role) + '</span><span>·</span><span>'
                    + escapeHtml(stream.LanguageLabel || stream.Language || "未知语言") + '</span><span>·</span><span>'
                    + escapeHtml(stream.IsExternal ? "外置" : "内封") + '</span></div>'
                    + (facts.length ? '<div class="ss-muted">' + escapeHtml(facts.join(" · ")) + '</div>' : '')
                    + '</div><span class="ss-chip ' + healthClass(stream.Health) + '">' + escapeHtml(stream.Health || "UNKNOWN") + '</span></div>';
            }).join("");

            return '<div class="ss-existing-subtitles"><div class="ss-inline-actions" style="justify-content:space-between;margin-top:14px">'
                + '<span class="ss-row-title">已有字幕深检</span><span class="ss-chip ' + healthClass(targetHealth) + '">目标 ' + escapeHtml(targetHealth) + '</span></div>'
                + '<div class="ss-candidate-list">' + rows + '</div></div>';
        }

        function renderWorkbench() {
            var body = getElement("workbenchBody");
            var item = state.selectedItem;
            var workbench = pageRoot.querySelector(".ss-workbench");
            if (workbench) workbench.classList.toggle("ss-has-selection", Boolean(item));
            if (!item) {
                getElement("workbenchState").textContent = "未选择";
                body.innerHTML = '<div class="ss-empty"><div><strong>先选择一个媒体条目</strong><span>条目状态、候选和字幕预览会显示在这里。</span></div></div>';
                return;
            }

            var sources = Array.isArray(item.MediaSources) ? item.MediaSources : [];
            var source = sources.filter(function (entry) { return entry.Id === state.selectedSourceId; })[0] || sources[0] || null;
            var singleSource = sources.length === 1;
            var presence = source && source.Presence;
            getElement("workbenchState").textContent = actionName(item);

            var sourceControl = sources.length
                ? '<select class="ss-select" id="sourceSelect" aria-label="选择媒体来源">'
                    + sources.map(function (entry) {
                        return '<option value="' + escapeHtml(entry.Id) + '"'
                            + (entry.Id === (source && source.Id) ? " selected" : "") + '>'
                            + escapeHtml(entry.Name || entry.Container || "MediaSource") + '</option>';
                    }).join("") + '</select>'
                : '<span class="ss-chip ss-chip-danger">没有 MediaSource</span>';

            body.innerHTML = '<div class="ss-item-summary"><div><h3>' + escapeHtml(item.Name || "未命名条目")
                + '</h3><div class="ss-row-meta"><span>' + escapeHtml(item.LibraryName || "未归属媒体库") + '</span><span>·</span><span>'
                + escapeHtml(item.Type || "媒体") + '</span>' + (item.IsStrm ? '<span>·</span><span>STRM</span>' : "")
                + '</div></div><div class="ss-inline-actions"><span class="ss-chip ' + actionClass(actionName(item)) + '">' + escapeHtml(actionName(item))
                + '</span><button class="ss-button ss-mobile-only" id="changeItem" type="button">更换媒体</button></div></div>'
                + renderStepper()
                + '<div class="ss-work-grid"><section class="ss-subcard"><div class="ss-card-header"><h3>条目与来源</h3></div><div class="ss-subcard-body">'
                + '<label class="ss-field-label" for="sourceSelect">媒体来源</label>' + sourceControl
                 + '<div class="ss-presence"><span class="ss-status-dot ' + (presence && presence.TargetLanguagePresent ? "is-good" : "")
                 + '"></span><div><div class="ss-row-title">' + (presence && presence.TargetLanguagePresent ? "目标字幕已存在" : "目标字幕缺失或未知")
                 + '</div><div class="ss-muted">' + escapeHtml(presence && presence.TargetLanguageLabel ? presence.TargetLanguageLabel : effectiveConfiguration(item).TargetLanguage)
                 + ' · ' + formatCount(source ? source.SubtitleStreamCount : 0) + ' 条字幕流</div></div></div>'
                 + renderExistingSubtitleStreams(source)
                 + '<button class="ss-button ss-button-primary" id="searchCandidates" type="button" ' + (singleSource ? "" : "disabled")
                + '>寻找字幕</button><div class="ss-feedback" id="workFeedback">'
                + (singleSource ? "搜索会使用当前媒体库的有效目标语言。" : "多 Source 保持只读，不开放写入。") + '</div></div></section>'
                + '<section class="ss-subcard"><div class="ss-card-header"><h3>' + (state.artifact ? "校验与预览" : "字幕候选")
                + '</h3><span class="ss-chip">' + (state.artifact ? "已下载并校验" : formatCount(state.candidates.length) + " 个") + '</span></div>'
                + '<div class="ss-subcard-body">' + renderStage() + '</div></section></div>';

            var sourceSelect = getElement("sourceSelect");
            if (sourceSelect) {
                sourceSelect.addEventListener("change", function (event) {
                    state.selectedSourceId = event.target.value;
                    state.candidates = [];
                    state.artifact = null;
                    state.alignmentHistory = [];
                    renderWorkbench();
                });
            }
            var searchButton = getElement("searchCandidates");
            if (searchButton) searchButton.addEventListener("click", searchCandidates);
            pageRoot.querySelectorAll("[data-fetch-index]").forEach(function (button) {
                button.addEventListener("click", fetchCandidate);
            });
            var installButton = getElement("installArtifact");
            if (installButton) installButton.addEventListener("click", installArtifact);
            var changeItemButton = getElement("changeItem");
            if (changeItemButton) changeItemButton.addEventListener("click", clearSelectedItem);
            pageRoot.querySelectorAll("[data-align-sign]").forEach(function (button) {
                button.addEventListener("click", alignArtifact);
            });
            var undoAlignmentButton = getElement("undoAlignment");
            if (undoAlignmentButton) undoAlignmentButton.addEventListener("click", undoAlignment);
        }

        function clearSelectedItem() {
            state.selectedItem = null;
            state.selectedSourceId = null;
            state.candidates = [];
            state.artifact = null;
            state.alignmentHistory = [];
            renderItems();
            renderWorkbench();
            getElement("itemSearch").focus();
        }

        function renderStepper() {
            var hasCandidates = state.candidates.length > 0 || Boolean(state.artifact);
            var hasArtifact = Boolean(state.artifact);
            var aligned = hasArtifact && Number(state.artifact.TimelineOffsetMilliseconds || 0) !== 0;
            var labels = ["寻找", "下载", "校验", "对轴（可选）", "安装"];
            return '<div class="ss-stepper" aria-label="字幕处理进度">' + labels.map(function (label, index) {
                var className = "";
                if (index === 0) className = hasCandidates ? "is-done" : "is-active";
                if (index === 1) className = hasArtifact ? "is-done" : hasCandidates ? "is-active" : "";
                if (index === 2) className = hasArtifact ? "is-done" : "";
                if (index === 3) className = aligned ? "is-done" : hasArtifact ? "is-optional" : "";
                if (index === 4) className = hasArtifact ? "is-active" : "";
                return '<div class="ss-step ' + className + '">' + (index + 1) + ' ' + label + '</div>';
            }).join("") + '</div>';
        }

        function renderStage() {
            if (state.artifact) return renderArtifact(state.artifact);
            if (!state.candidates.length) {
                return '<div class="ss-muted">尚未搜索。候选会按 Hash 或标题绑定状态排序，未绑定候选不可下载。</div>';
            }
            return '<div class="ss-candidate-list">' + state.candidates.map(function (candidate, index) {
                var matched = candidate.IsHashMatch || candidate.TitleMatch;
                var badges = candidate.IsHashMatch
                    ? '<span class="ss-chip ss-chip-good">Hash 匹配</span>'
                    : candidate.TitleMatch
                        ? '<span class="ss-chip ss-chip-warning">标题匹配</span>'
                        : '<span class="ss-chip ss-chip-danger">未绑定</span>';
                var mismatchBadges = (candidate.LanguageMismatch ? '<span class="ss-chip ss-chip-warning">语言标注不符</span>' : "")
                    + (candidate.VariantMismatch ? '<span class="ss-chip ss-chip-warning">简繁变体不符</span>' : "");
                return '<div class="ss-candidate"><div><div class="ss-row-title" title="' + escapeHtml(candidate.Name || "未命名候选")
                    + '">' + escapeHtml(candidate.Name || "未命名候选") + '</div><div class="ss-row-meta"><span>'
                    + escapeHtml(candidate.Provider || "未知 Provider") + '</span><span>·</span><span>'
                    + escapeHtml(candidate.LanguageLabel || candidate.Language || "未知语言") + '</span><span>·</span><span>'
                    + escapeHtml(candidate.Format || "未知格式") + '</span></div><div class="ss-chip-row">' + badges + mismatchBadges
                    + '</div></div><button class="ss-button" type="button" data-fetch-index="' + index + '" '
                    + (matched ? "" : "disabled") + '>下载并校验</button></div>';
            }).join("") + '</div>';
        }

        function renderArtifact(artifact) {
            var quality = artifact.Quality || {};
            var preference = artifact.Preference || {};
            var action = artifact.Action || {};
            var reasons = artifact.Reasons || [];
            var cues = artifact.Cues || [];
            var canInstall = artifact.Health === "PASS" || artifact.Health === "WARNING";
            return '<div><div class="ss-inline-actions" style="justify-content:space-between"><div class="ss-row-meta"><span>'
                + escapeHtml(artifact.LanguageLabel || artifact.Language || "未知语言") + '</span><span>·</span><span>'
                + escapeHtml(artifact.Format || "未知格式") + '</span><span>·</span><span>'
                + escapeHtml(artifact.Encoding || "未知编码") + '</span></div><span class="ss-chip '
                + healthClass(artifact.Health) + '">' + escapeHtml(artifact.Health || "UNKNOWN") + '</span></div>'
                + '<div class="ss-quality-grid"><div class="ss-quality-item"><span>目标语言</span><strong>'
                + (quality.TargetLanguagePresent ? formatPercent(quality.TargetLanguageConfidence) : "未确认")
                + '</strong></div><div class="ss-quality-item"><span>第二语言 cue</span><strong>'
                + formatCount(quality.SecondaryLanguageCueCount) + '</strong></div><div class="ss-quality-item"><span>双语判断</span><strong>'
                + (quality.BilingualDetected ? formatPercent(quality.BilingualConfidence) : "未发现")
                + '</strong></div><div class="ss-quality-item"><span>偏好评估</span><strong>'
                + escapeHtml(preference.Suitability || "未评估") + '</strong></div></div>'
                + (reasons.length ? '<ul class="ss-reason-list">' + reasons.map(function (reason) {
                    return '<li>' + escapeHtml(reason) + '</li>';
                }).join("") + '</ul>' : "")
                + renderAlignment(artifact)
                + '<div class="ss-inline-actions" style="justify-content:space-between;margin-top:12px"><span class="ss-muted">'
                + escapeHtml(action.Action ? "建议动作 " + action.Action + "。" + ((action.Reasons || []).join(" ")) : "请确认校验和偏好结果。")
                + '</span><button class="ss-button ss-button-primary" id="installArtifact" type="button" '
                + (canInstall ? "" : "disabled") + '>安装字幕</button></div>'
                + '<details class="ss-cue-details"><summary>展开 cue 预览（前 200 条）</summary><div class="ss-cue-list">'
                + (cues.length ? cues.map(function (cue) {
                    return '<div class="ss-cue"><span>' + formatMilliseconds(cue.StartMilliseconds) + ' → '
                        + formatMilliseconds(cue.EndMilliseconds) + '</span><span class="ss-cue-text">'
                        + escapeHtml(cue.Text) + '</span></div>';
                }).join("") : '<div class="ss-muted">没有可展示的 cue。</div>')
                + '</div></details></div>';
        }

        function renderAlignment(artifact) {
            var cumulativeOffset = Number(artifact.TimelineOffsetMilliseconds || 0);
            var offsetText = cumulativeOffset === 0
                ? "当前未偏移"
                : "当前累计 " + (cumulativeOffset > 0 ? "+" : "") + formatCount(cumulativeOffset) + " ms";
            return '<div class="ss-align-note" data-fd-id="alignment-controls"><div><span class="ss-row-title">人工固定偏移</span>'
                + '<div class="ss-muted">' + offsetText + '。字幕晚于画面选“字幕提前”，字幕早于画面选“字幕延后”；不会直接修改媒体文件。</div></div>'
                + '<div class="ss-align-controls"><input class="ss-input ss-align-input" id="alignmentOffset" type="number" min="10" max="600000" step="10" value="500" aria-label="对轴偏移毫秒数" />'
                + '<button class="ss-button" type="button" data-align-sign="-1">字幕提前</button>'
                + '<button class="ss-button" type="button" data-align-sign="1">字幕延后</button>'
                + (state.alignmentHistory.length ? '<button class="ss-button" id="undoAlignment" type="button">撤销上次</button>' : '')
                + '</div></div>';
        }

        function normalizeConfiguration(configuration) {
            var normalized = Object.assign({}, DEFAULT_CONFIGURATION, configuration || {});
            normalized.TargetLanguage = normalized.TargetLanguage || DEFAULT_CONFIGURATION.TargetLanguage;
            normalized.SecondaryLanguage = normalized.SecondaryLanguage || DEFAULT_CONFIGURATION.SecondaryLanguage;
            normalized.FormatOrder = normalized.FormatOrder || DEFAULT_CONFIGURATION.FormatOrder;
            normalized.LibraryOverrides = Array.isArray(normalized.LibraryOverrides) ? normalized.LibraryOverrides : [];
            return normalized;
        }

        function normalizeId(value) {
            return String(value || "").replace(/-/g, "").toLowerCase();
        }

        function effectiveConfiguration(item) {
            var globalConfig = state.configuration;
            var libraryOverride = item && globalConfig.LibraryOverrides.filter(function (entry) {
                return entry && entry.Enabled && normalizeId(entry.LibraryId) === normalizeId(item.LibraryId);
            })[0];
            if (!libraryOverride) return globalConfig;
            return {
                TargetLanguage: libraryOverride.TargetLanguage || globalConfig.TargetLanguage,
                SecondaryLanguage: libraryOverride.SecondaryLanguage || globalConfig.SecondaryLanguage,
                PreferBilingual: Boolean(libraryOverride.PreferBilingual),
                FormatOrder: libraryOverride.FormatOrder || globalConfig.FormatOrder
            };
        }

        async function loadConfiguration() {
            var api = getHostApi();
            var configuration = api && typeof api.getPluginConfiguration === "function"
                ? await api.getPluginConfiguration(PLUGIN_ID)
                : await request("/Plugins/" + PLUGIN_ID + "/Configuration");
            state.configuration = normalizeConfiguration(configuration);
            setChoiceValue("globalTargetLanguage", state.configuration.TargetLanguage, normalizeLanguageChoice, DEFAULT_CONFIGURATION.TargetLanguage);
            setChoiceValue("globalSecondaryLanguage", state.configuration.SecondaryLanguage, normalizeLanguageChoice, DEFAULT_CONFIGURATION.SecondaryLanguage);
            setChoiceValue("globalFormatOrder", state.configuration.FormatOrder, normalizeFormatOrderChoice, DEFAULT_CONFIGURATION.FormatOrder);
            getElement("globalPreferBilingual").checked = Boolean(state.configuration.PreferBilingual);
        }

        async function persistConfiguration(configuration) {
            var api = getHostApi();
            if (api && typeof api.updatePluginConfiguration === "function") {
                await api.updatePluginConfiguration(PLUGIN_ID, configuration);
            } else {
                await request("/Plugins/" + PLUGIN_ID + "/Configuration", { method: "POST", body: configuration });
            }
            state.configuration = normalizeConfiguration(configuration);
        }

        async function saveGlobalConfiguration() {
            var button = getElement("saveGlobalSettings");
            var target = getElement("globalTargetLanguage").value.trim();
            var secondary = getElement("globalSecondaryLanguage").value.trim();
            var order = getElement("globalFormatOrder").value.trim();
            if (!target || !secondary || !order) {
                setFeedback("globalSettingsFeedback", "目标语言、第二语言和格式优先级都必须选择。", "error");
                return;
            }
            setButtonBusy(button, true, "保存中…");
            try {
                await persistConfiguration(Object.assign({}, state.configuration, {
                    TargetLanguage: target,
                    SecondaryLanguage: secondary,
                    FormatOrder: order,
                    PreferBilingual: getElement("globalPreferBilingual").checked
                }));
                setFeedback("globalSettingsFeedback", "全局设置已保存。", "success");
                renderLibraries();
                await loadItems(true);
            } catch (error) {
                setFeedback("globalSettingsFeedback", error.message || "保存失败。", "error");
            } finally {
                setButtonBusy(button, false);
            }
        }

        async function loadLibraries() {
            try {
                var libraries = await request("/SubSteward/Libraries");
                state.libraries = Array.isArray(libraries) ? libraries : [];
            } catch (error) {
                state.libraries = [];
                setFeedback("librarySettingsFeedback", "媒体库列表读取失败：" + (error.message || "未知错误"), "error");
            }
            renderLibraries();
        }

        function renderLibraries() {
            getElement("libraryCount").textContent = formatCount(state.libraries.length) + " 个";
            getElement("libraryList").innerHTML = state.libraries.length
                ? state.libraries.map(function (library) {
                    var entry = state.configuration.LibraryOverrides.filter(function (override) {
                        return override && normalizeId(override.LibraryId) === normalizeId(library.Id);
                    })[0];
                    var enabled = entry && entry.Enabled;
                    return '<button class="ss-library-row" type="button" aria-current="'
                        + (state.selectedLibrary && state.selectedLibrary.Id === library.Id ? "true" : "false")
                        + '" data-library-id="' + escapeHtml(library.Id) + '"><span><span class="ss-row-title">'
                        + escapeHtml(library.Name || "未命名媒体库") + '</span><span class="ss-row-meta">'
                        + (enabled ? "使用独立偏好" : "继承全局默认") + '</span></span><span class="ss-chip '
                        + (enabled ? "ss-chip-good" : "") + '">' + (enabled ? "独立" : "继承") + '</span></button>';
                }).join("")
                : '<div class="ss-empty"><div><strong>没有读取到媒体库</strong><span>请确认管理员会话和插件 API。</span></div></div>';
            if (state.selectedLibrary) renderSelectedLibrary();
        }

        function selectLibrary(libraryId) {
            state.selectedLibrary = state.libraries.filter(function (library) { return library.Id === libraryId; })[0] || null;
            renderLibraries();
        }

        function renderSelectedLibrary() {
            var library = state.selectedLibrary;
            if (!library) return;
            var entry = state.configuration.LibraryOverrides.filter(function (override) {
                return override && normalizeId(override.LibraryId) === normalizeId(library.Id);
            })[0];
            var enabled = Boolean(entry && entry.Enabled);
            var settingsLayout = pageRoot.querySelector(".ss-settings-layout");
            if (settingsLayout) settingsLayout.classList.add("ss-library-selected");
            getElement("librarySettingsEmpty").hidden = true;
            getElement("librarySettingsForm").hidden = false;
            getElement("selectedLibraryName").textContent = library.Name || "媒体库设置";
            getElement("libraryOverrideEnabled").checked = enabled;
            getElement("libraryFieldset").disabled = !enabled;
            getElement("librarySettingsCard").setAttribute("aria-disabled", enabled ? "false" : "true");
            getElement("libraryInheritanceSummary").textContent = enabled
                ? "当前覆盖全局默认，仅影响此媒体库。"
                : "当前继承全局默认。开启后可编辑独立值。";
            setChoiceValue("libraryTargetLanguage", entry && entry.TargetLanguage || state.configuration.TargetLanguage, normalizeLanguageChoice, state.configuration.TargetLanguage);
            setChoiceValue("librarySecondaryLanguage", entry && entry.SecondaryLanguage || state.configuration.SecondaryLanguage, normalizeLanguageChoice, state.configuration.SecondaryLanguage);
            setChoiceValue("libraryFormatOrder", entry && entry.FormatOrder || state.configuration.FormatOrder, normalizeFormatOrderChoice, state.configuration.FormatOrder);
            getElement("libraryPreferBilingual").checked = entry
                ? Boolean(entry.PreferBilingual)
                : Boolean(state.configuration.PreferBilingual);
        }

        function toggleLibraryOverride() {
            var enabled = getElement("libraryOverrideEnabled").checked;
            getElement("libraryFieldset").disabled = !enabled;
            getElement("librarySettingsCard").setAttribute("aria-disabled", enabled ? "false" : "true");
            getElement("libraryInheritanceSummary").textContent = enabled
                ? "将使用此媒体库的独立设置。保存后生效。"
                : "将恢复继承全局默认。保存后生效。";
        }

        function clearSelectedLibrary() {
            state.selectedLibrary = null;
            var settingsLayout = pageRoot.querySelector(".ss-settings-layout");
            if (settingsLayout) settingsLayout.classList.remove("ss-library-selected");
            getElement("librarySettingsForm").hidden = true;
            getElement("librarySettingsEmpty").hidden = false;
            getElement("librarySettingsCard").setAttribute("aria-disabled", "true");
            renderLibraries();
            var firstLibrary = getElement("libraryList").querySelector("[data-library-id]");
            if (firstLibrary) firstLibrary.focus();
        }

        async function saveLibraryConfiguration() {
            if (!state.selectedLibrary) return;
            var button = getElement("saveLibrarySettings");
            var enabled = getElement("libraryOverrideEnabled").checked;
            var target = getElement("libraryTargetLanguage").value.trim();
            var secondary = getElement("librarySecondaryLanguage").value.trim();
            var order = getElement("libraryFormatOrder").value.trim();
            if (enabled && (!target || !secondary || !order)) {
                setFeedback("librarySettingsFeedback", "启用独立设置时，请选择目标语言、第二语言和格式优先级。", "error");
                return;
            }
            setButtonBusy(button, true, "保存中…");
            try {
                var entries = state.configuration.LibraryOverrides.filter(function (entry) {
                    return entry && normalizeId(entry.LibraryId) !== normalizeId(state.selectedLibrary.Id);
                });
                entries.push({
                    LibraryId: state.selectedLibrary.Id,
                    LibraryName: state.selectedLibrary.Name,
                    Enabled: enabled,
                    TargetLanguage: target || state.configuration.TargetLanguage,
                    SecondaryLanguage: secondary || state.configuration.SecondaryLanguage,
                    PreferBilingual: getElement("libraryPreferBilingual").checked,
                    FormatOrder: order || state.configuration.FormatOrder
                });
                await persistConfiguration(Object.assign({}, state.configuration, { LibraryOverrides: entries }));
                setFeedback("librarySettingsFeedback", enabled ? "媒体库独立设置已保存。" : "已恢复继承全局默认。", "success");
                renderLibraries();
                await loadItems(true);
            } catch (error) {
                setFeedback("librarySettingsFeedback", error.message || "保存失败。", "error");
            } finally {
                setButtonBusy(button, false);
            }
        }

        async function loadItems(preserveSelection) {
            var search = getElement("itemSearch").value.trim();
            setFeedback("itemFeedback", "正在读取媒体条目…");
            try {
                var items = await request("/SubSteward/Items", { query: { SearchTerm: search, Limit: 100 } });
                state.items = Array.isArray(items) ? items : [];
                if (preserveSelection && state.selectedItem) {
                    state.selectedItem = state.items.filter(function (item) {
                        return item.Id === state.selectedItem.Id;
                    })[0] || null;
                } else if (state.selectedItem) {
                    state.selectedItem = null;
                    state.selectedSourceId = null;
                    state.candidates = [];
                    state.artifact = null;
                }
                renderStatus();
                renderItems();
                renderWorkbench();
                setFeedback("itemFeedback", state.items.length
                    ? "已读取前 " + formatCount(state.items.length) + " 项。"
                    : "当前筛选没有结果。");
                getElement("lastUpdated").textContent = "更新于 " + new Date().toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" });
            } catch (error) {
                state.items = [];
                renderStatus();
                renderItems();
                renderWorkbench();
                setFeedback("itemFeedback", error.message || "条目读取失败。", "error");
            }
        }

        async function selectItem(itemId) {
            var cached = state.items.filter(function (item) { return item.Id === itemId; })[0];
            if (!cached) return;
            state.selectedItem = cached;
            state.selectedSourceId = cached.MediaSources && cached.MediaSources.length ? cached.MediaSources[0].Id : null;
            state.candidates = [];
            state.artifact = null;
            state.alignmentHistory = [];
            renderItems();
            renderWorkbench();
            try {
                var fresh = await request("/SubSteward/Items/" + encodeURIComponent(itemId));
                if (fresh && state.selectedItem && fresh.Id === state.selectedItem.Id) {
                    state.selectedItem = fresh;
                    state.selectedSourceId = fresh.MediaSources && fresh.MediaSources.length ? fresh.MediaSources[0].Id : null;
                    renderItems();
                    renderWorkbench();
                }
            } catch (error) {
                var feedback = getElement("workFeedback");
                if (feedback) feedback.textContent = "详情读取失败：" + (error.message || "未知错误");
            }
        }

        async function searchCandidates() {
            if (!state.selectedItem || !state.selectedSourceId) return;
            var button = getElement("searchCandidates");
            setButtonBusy(button, true, "搜索中…");
            try {
                var response = await request("/SubSteward/Subtitles/Search", {
                    query: {
                        ItemId: state.selectedItem.Id,
                        MediaSourceId: state.selectedSourceId,
                        Language: effectiveConfiguration(state.selectedItem).TargetLanguage
                    }
                });
                state.candidates = response && Array.isArray(response.Candidates) ? response.Candidates : [];
                state.artifact = null;
                state.alignmentHistory = [];
                renderWorkbench();
                var feedback = getElement("workFeedback");
                if (feedback) feedback.textContent = state.candidates.length
                    ? "已找到候选。先下载并校验正文，再决定是否安装。"
                    : "没有返回可用候选。";
            } catch (error) {
                var workFeedback = getElement("workFeedback");
                if (workFeedback) workFeedback.textContent = "搜索失败：" + (error.message || "未知错误");
            } finally {
                setButtonBusy(getElement("searchCandidates"), false);
            }
        }

        async function fetchCandidate(event) {
            var index = Number(event.currentTarget.getAttribute("data-fetch-index"));
            var candidate = state.candidates[index];
            if (!candidate) return;
            setButtonBusy(event.currentTarget, true, "下载校验中…");
            try {
                state.artifact = await request("/SubSteward/Subtitles/Fetch", {
                    method: "POST",
                    body: { CandidateToken: candidate.Token }
                });
                state.alignmentHistory = [];
                renderWorkbench();
            } catch (error) {
                event.currentTarget.disabled = false;
                event.currentTarget.textContent = "重试下载";
                var workFeedback = getElement("workFeedback");
                if (workFeedback) workFeedback.textContent = "候选下载失败：" + (error.message || "未知错误");
            }
        }

        async function alignArtifact(event) {
            if (!state.artifact) return;
            var input = getElement("alignmentOffset");
            var amount = Number(input && input.value);
            if (!Number.isFinite(amount) || amount !== Math.floor(amount) || amount < 10 || amount > 600000) {
                var invalidFeedback = getElement("workFeedback");
                if (invalidFeedback) invalidFeedback.textContent = "对轴偏移必须是 10 到 600000 之间的整数毫秒。";
                if (input) input.focus();
                return;
            }

            var sign = Number(event.currentTarget.getAttribute("data-align-sign"));
            var delta = amount * (sign < 0 ? -1 : 1);
            var previousArtifact = state.artifact;
            var button = event.currentTarget;
            setButtonBusy(button, true, "对轴中…");
            try {
                var alignedArtifact = await request("/SubSteward/Subtitles/Align", {
                    method: "POST",
                    body: {
                        ArtifactToken: previousArtifact.ArtifactToken,
                        OffsetMilliseconds: delta
                    }
                });
                state.alignmentHistory.push(previousArtifact);
                state.artifact = alignedArtifact;
                renderWorkbench();
                var feedback = getElement("workFeedback");
                if (feedback) feedback.textContent = "已生成偏移后的临时字幕并重新校验；确认预览后再安装。";
            } catch (error) {
                setButtonBusy(button, false);
                var workFeedback = getElement("workFeedback");
                if (workFeedback) workFeedback.textContent = "对轴失败：" + (error.message || "未知错误");
            }
        }

        function undoAlignment() {
            if (!state.alignmentHistory.length) return;
            state.artifact = state.alignmentHistory.pop();
            renderWorkbench();
            var feedback = getElement("workFeedback");
            if (feedback) feedback.textContent = "已撤销上一次时间偏移。";
        }

        async function installArtifact() {
            if (!state.artifact || !state.selectedItem) return;
            var message = state.artifact.Health === "WARNING"
                ? "这份字幕存在 WARNING。仍要写入新的版本化 sidecar 并刷新 Emby 吗？"
                : "确认把这份字幕写入当前单一 MediaSource，并刷新 Emby 吗？";
            if (!window.confirm(message)) return;
            var button = getElement("installArtifact");
            setButtonBusy(button, true, "安装中…");
            try {
                var response = await request("/SubSteward/Subtitles/Install", {
                    method: "POST",
                    body: { ArtifactToken: state.artifact.ArtifactToken }
                });
                state.artifact = null;
                state.candidates = [];
                state.alignmentHistory = [];
                var refreshed = await request("/SubSteward/Items/" + encodeURIComponent(state.selectedItem.Id));
                state.selectedItem = refreshed;
                state.selectedSourceId = refreshed.MediaSources && refreshed.MediaSources.length ? refreshed.MediaSources[0].Id : null;
                await loadItems(true);
                renderWorkbench();
                var feedback = getElement("workFeedback");
                if (feedback) feedback.textContent = "安装完成：" + (response && response.FileName ? response.FileName : "Emby 已刷新字幕流");
            } catch (error) {
                setButtonBusy(getElement("installArtifact"), false);
                var workFeedback = getElement("workFeedback");
                if (workFeedback) workFeedback.textContent = "安装失败：" + (error.message || "未知错误");
            }
        }

        pageRoot.querySelectorAll("[data-tab]").forEach(function (button, index, buttons) {
            button.addEventListener("click", function () { switchTab(button.getAttribute("data-tab"), true); });
            button.addEventListener("keydown", function (event) {
                if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
                event.preventDefault();
                var nextIndex = event.key === "ArrowRight"
                    ? (index + 1) % buttons.length
                    : (index - 1 + buttons.length) % buttons.length;
                buttons[nextIndex].focus();
                switchTab(buttons[nextIndex].getAttribute("data-tab"), false);
            });
        });

        pageRoot.querySelectorAll("[data-settings-pane]").forEach(function (button) {
            button.addEventListener("click", function () {
                switchSettingsPane(button.getAttribute("data-settings-pane"));
            });
        });

        getElement("refreshAll").addEventListener("click", async function () {
            var button = getElement("refreshAll");
            setButtonBusy(button, true, "刷新中…");
            try {
                await Promise.all([loadConfiguration(), loadLibraries()]);
                renderLibraries();
                await loadItems(true);
            } finally {
                setButtonBusy(button, false);
            }
        });
        getElement("openManual").addEventListener("click", function () { switchTab("manual", true); });
        getElement("itemFilterForm").addEventListener("submit", function (event) {
            event.preventDefault();
            loadItems(false);
        });
        getElement("itemList").addEventListener("click", function (event) {
            var row = event.target.closest("[data-item-id]");
            if (row) selectItem(row.getAttribute("data-item-id"));
        });
        getElement("attentionList").addEventListener("click", function (event) {
            var row = event.target.closest("[data-open-item]");
            if (!row) return;
            switchTab("manual", false);
            selectItem(row.getAttribute("data-open-item"));
        });
        getElement("libraryList").addEventListener("click", function (event) {
            var row = event.target.closest("[data-library-id]");
            if (row) selectLibrary(row.getAttribute("data-library-id"));
        });
        getElement("libraryOverrideEnabled").addEventListener("change", toggleLibraryOverride);
        getElement("changeLibrary").addEventListener("click", clearSelectedLibrary);
        getElement("saveGlobalSettings").addEventListener("click", saveGlobalConfiguration);
        getElement("saveLibrarySettings").addEventListener("click", saveLibraryConfiguration);

        (async function bootstrap() {
            try {
                await Promise.all([loadConfiguration(), loadLibraries()]);
                renderLibraries();
                await loadItems(false);
            } catch (error) {
                setFeedback("itemFeedback", "SubSteward 初始化失败：" + (error.message || "未知错误"), "error");
                getElement("statusMetrics").innerHTML = metricCard("页面初始化失败", 0, "请检查管理员会话和插件日志");
            }
        }());
    };
});
