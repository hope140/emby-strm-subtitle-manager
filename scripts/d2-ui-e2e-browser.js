async (page) => {
  "use strict";

  const phase = await page.evaluate(() => window.name);
  if (!phase) throw new Error("D2 UI E2E phase marker is missing");

  async function login() {
    await page.locator("#app-panel").waitFor({ state: "visible" });
    if (await page.locator("#token").inputValue()) throw new Error("login input retained the Bearer Token");
    await page.getByRole("button", { name: /D2 UI Fixture Movie/ }).click();
  }

  async function storageState() {
    return page.evaluate(() => ({
      local: Object.keys(localStorage),
      session: Object.keys(sessionStorage),
      cookie: document.cookie
    }));
  }

  if (phase === "disabled") {
    await login();
    await page.locator("#d2-panel").waitFor({ state: "visible" });
    if (await page.locator("#d2-actions").isVisible()) throw new Error("disabled D2 controls are visible");
    if (!(await page.locator("#d2-status").textContent()).includes("未启用")) throw new Error("disabled D2 status is missing");
    if ((await page.locator("#d2-results").isVisible()) || (await page.locator("#d2-preview").isVisible())) throw new Error("disabled D2 result or preview is visible");
    const state = await storageState();
    if (state.local.length || state.session.length || state.cookie) throw new Error("disabled flow wrote browser storage");
    await page.reload();
    await page.locator("#login-panel").waitFor({ state: "visible" });
    return;
  }

  if (phase === "enabled") {
    const consoleMessages = [];
    const pageErrors = [];
    const d2Requests = [];
    const d2Responses = [];
    page.on("console", message => consoleMessages.push(message.text()));
    page.on("pageerror", error => pageErrors.push(String(error)));
    page.on("request", request => {
      if (request.url().includes("/subtitles/")) d2Requests.push({ url: request.url(), method: request.method(), body: request.postData() });
    });
    page.on("response", async response => {
      if (!response.url().includes("/subtitles/")) return;
      try { d2Responses.push({ url: response.url(), status: response.status(), body: await response.json() }); } catch (_) {}
    });

    await login();
    await page.locator("#d2-actions").waitFor({ state: "visible" });
    await page.locator("#d2-search").click();
    await page.locator("#d2-candidates .candidate-card").nth(1).waitFor({ state: "visible" });
    const cards = page.locator("#d2-candidates .candidate-card");
    if (await cards.count() !== 2) throw new Error("expected two candidates");
    const searchRequest = d2Requests.find(request => request.url.endsWith("/subtitles/search"));
    if (!searchRequest) throw new Error("search request was not observed");
    const searchBody = JSON.parse(searchRequest.body);
    if (searchBody.media_source_id !== "fixture-source" || searchBody.language !== "zh-CN" || searchBody.forced !== false || Object.keys(searchBody).length !== 3) throw new Error("search request was not fixed to the selected source and language");

    await cards.nth(0).getByRole("button", { name: "获取预览" }).click();
    await cards.nth(0).locator(".candidate-header .status-badge").getByText("获取失败", { exact: true }).waitFor({ state: "visible" });
    if (!(await cards.nth(1).getByRole("button", { name: "获取预览" }).isEnabled())) throw new Error("candidate B was not retained after candidate A failed");
    if (!(await cards.nth(0).textContent()).includes("其他候选仍可用")) throw new Error("candidate failure message is not actionable");

    await cards.nth(1).getByRole("button", { name: "获取预览" }).click();
    await page.locator("#d2-preview").waitFor({ state: "visible" });
    await page.locator("#d2-cues").getByText("第 1 条预览文本", { exact: true }).waitFor({ state: "visible" });
    if (!(await page.locator("#d2-preview-next").isEnabled())) throw new Error("truncated preview did not enable next page");
    await page.locator("#d2-preview-next").click();
    await page.locator("#d2-cues").getByText("第 201 条预览文本", { exact: true }).waitFor({ state: "visible" });
    if (!(await page.locator("#d2-preview-status").textContent()).includes("201-205")) throw new Error("next preview page did not advance by returned cue count");
    await page.locator("#d2-preview-reset").click();
    await page.locator("#d2-cues").getByText("第 1 条预览文本", { exact: true }).waitFor({ state: "visible" });

    await page.waitForTimeout(100);
    const bodyText = await page.locator("body").innerText();
    const responseTokens = d2Responses.flatMap(response => {
      const body = response.body || {};
      const values = [];
      if (body.candidates) {
        for (const candidate of body.candidates) {
          if (["id", "candidate_id", "remote_id"].some(field => Object.prototype.hasOwnProperty.call(candidate, field))) {
            throw new Error("candidate response exposed a raw candidate ID field");
          }
          values.push(candidate.token);
        }
      }
      if (body.artifact_token) values.push(body.artifact_token);
      return values;
    }).filter(Boolean);
    for (const value of responseTokens.concat(["C:\\d2-ui\\media"])) {
      if (bodyText.includes(value)) throw new Error("sensitive value appeared in rendered DOM");
    }
    const hasDataAttribute = await page.evaluate(() => Array.from(document.querySelectorAll("*"), element => Array.from(element.attributes).some(attribute => attribute.name.startsWith("data-"))).some(Boolean));
    if (hasDataAttribute) throw new Error("D2 UI used a data attribute");
    const state = await storageState();
    if (state.local.length || state.session.length || state.cookie) throw new Error("enabled flow wrote browser storage");
    if (pageErrors.length) throw new Error("page error: " + pageErrors.join(" | "));
    for (const message of consoleMessages) {
      if (responseTokens.some(value => message.includes(value))) throw new Error("sensitive value appeared in console");
    }
    await page.reload();
    await page.locator("#login-panel").waitFor({ state: "visible" });
    if (await page.locator("#app-panel").isVisible()) throw new Error("refresh retained the authenticated UI");
    const refreshed = await storageState();
    if (refreshed.local.length || refreshed.session.length || refreshed.cookie) throw new Error("refresh retained browser storage");
    return;
  }

  if (phase === "expiry") {
    await login();
    await page.locator("#d2-actions").waitFor({ state: "visible" });
    await page.locator("#d2-search").click();
    const cards = page.locator("#d2-candidates .candidate-card");
    await cards.nth(1).getByRole("button", { name: "获取预览" }).click();
    await page.locator("#d2-cues").getByText("第 1 条预览文本", { exact: true }).waitFor({ state: "visible" });
    await page.waitForTimeout(1600);
    await page.locator("#d2-preview-next").click();
    await page.locator("#d2-preview-status").getByText("预览已过期", { exact: false }).waitFor({ state: "visible" });
    return;
  }

  throw new Error("unknown D2 UI E2E phase");
}
