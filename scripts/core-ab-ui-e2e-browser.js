async (page) => {
  "use strict";

  const phase = await page.evaluate(() => window.name);
  if (!phase.startsWith("core-ab-")) throw new Error("Core A/B UI E2E phase marker is missing");

  async function waitAppStatus(expression) {
    await page.locator("#app-status").getByText(expression, { exact: false }).waitFor({ state: "visible" });
  }

  if (phase === "core-ab-before-upload-1") {
    await page.locator("#app-panel").waitFor({ state: "visible" });
    if (await page.locator("#password").inputValue()) throw new Error("login input retained the administrator password");
    await page.getByRole("button", { name: /Core A\/B UI Fixture Movie/ }).click();
    await page.getByRole("button", { name: /Version A/ }).waitFor({ state: "visible" });
    await page.getByRole("button", { name: /Version A/ }).click();
    await page.locator("#d2-actions").waitFor({ state: "visible" });
    await page.locator("#d2-search").click();
    const candidate = page.locator("#d2-candidates .candidate-card").first();
    await candidate.waitFor({ state: "visible" });
    await candidate.getByRole("button", { name: "获取预览" }).click();
    await page.locator("#d2-cues").getByText("远程预览字幕", { exact: true }).waitFor({ state: "visible" });
    await page.locator("#d3-add").waitFor({ state: "visible" });
    await page.locator("#d3-add-button").click();
    await waitAppStatus("字幕已添加");
    await page.locator(".subtitle").filter({ hasText: "Version-A.subbridge.zh-CN.srt" }).waitFor({ state: "visible" });
    return;
  }

  if (phase === "core-ab-upload-1-delete") {
    await page.locator("#d2-upload-file").setInputFiles("scripts/testdata/core-ab-upload-1.srt");
    await page.locator("#d2-upload").click();
    await page.locator("#d2-cues").getByText("第一次本地上传预览", { exact: true }).waitFor({ state: "visible" });
    const originalSubtitle = page.locator(".subtitle").filter({ hasText: "Fixture.zh-CN.srt" });
    page.once("dialog", dialog => { void dialog.accept().catch(() => {}); });
    await originalSubtitle.getByRole("button", { name: "移入回收区" }).click();
    return;
  }

  if (phase === "core-ab-after-delete-1") {
    await waitAppStatus("移入回收区");
    const deleteHistory = page.locator(".history-item").filter({ hasText: "移入回收区" }).first();
    await deleteHistory.waitFor({ state: "visible" });
    page.once("dialog", dialog => { void dialog.accept().catch(() => {}); });
    await deleteHistory.getByRole("button", { name: "恢复旧字幕" }).click();
    return;
  }

  if (phase === "core-ab-after-restore-1-upload-2-replace") {
    await waitAppStatus("旧字幕已恢复");
    await page.locator(".subtitle").filter({ hasText: "Fixture.zh-CN.srt" }).waitFor({ state: "visible" });
    await page.locator("#d2-upload-file").setInputFiles("scripts/testdata/core-ab-upload-2.srt");
    await page.locator("#d2-upload").click();
    await page.locator("#d2-cues").getByText("第二次本地上传预览", { exact: true }).waitFor({ state: "visible" });
    const restoredSubtitle = page.locator(".subtitle").filter({ hasText: "Fixture.zh-CN.srt" });
    page.once("dialog", dialog => { void dialog.accept().catch(() => {}); });
    await restoredSubtitle.getByRole("button", { name: "用当前预览替换" }).click();
    return;
  }

  if (phase === "core-ab-after-replace-2") {
    await waitAppStatus("字幕已替换");
    const replaceHistory = page.locator(".history-item").filter({ hasText: "替换" }).first();
    await replaceHistory.waitFor({ state: "visible" });
    page.once("dialog", dialog => { void dialog.accept().catch(() => {}); });
    await replaceHistory.getByRole("button", { name: "恢复旧字幕" }).click();
    return;
  }

  if (phase === "core-ab-final") {
    await waitAppStatus("旧字幕已恢复");
    await page.locator(".subtitle").filter({ hasText: "Fixture.zh-CN.srt" }).waitFor({ state: "visible" });

    const bodyText = await page.locator("body").innerText();
    for (const value of ["/fixture/media", "core-ab-upload-1.srt", "core-ab-upload-2.srt"]) {
      if (bodyText.includes(value)) throw new Error("sensitive value appeared in rendered DOM");
    }
    const hasDataAttribute = await page.evaluate(() => Array.from(document.querySelectorAll("*"), element => Array.from(element.attributes).some(attribute => attribute.name.startsWith("data-"))).some(Boolean));
    if (hasDataAttribute) throw new Error("Core A/B UI used a data attribute");
    const storage = await page.evaluate(() => ({ local: Object.keys(localStorage), session: Object.keys(sessionStorage), cookie: document.cookie }));
    if (storage.local.length || storage.session.length || storage.cookie) throw new Error("browser storage retained an operation credential");
    return;
  }

  throw new Error("unknown Core A/B UI E2E phase");
}
