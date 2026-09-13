import { test, expect } from "@playwright/test";
test("invited creator completes four-page manual and mock-AI flow", async ({
  page,
}, testInfo) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await page.getByLabel("用户名", { exact: true }).fill(process.env.TEST_ADMIN_USER || "demo-admin");
  await page.getByLabel("密码（12–72 字节）").fill(process.env.TEST_ADMIN_PASSWORD || "owlet-local-demo-only");
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByRole("heading", { name: "我的创作" })).toBeVisible();
  if ((process.env.TEST_WEB_ORIGIN || "").startsWith("https://")) {
    const session = (await page.context().cookies()).find((c) => c.name === "owlet_session");
    expect(session?.secure).toBe(true);
    expect(session?.httpOnly).toBe(true);
    expect(session?.sameSite).toBe("Strict");
  }
  const invitation = await page.request.post("/api/admin/invite", {
    headers: { Origin: process.env.TEST_WEB_ORIGIN || "http://localhost:5173" },
    data: {},
  });
  expect(invitation.status()).toBe(200);
  const { invite } = await invitation.json();
  await page.request.post("/api/auth/logout", {
    headers: { Origin: process.env.TEST_WEB_ORIGIN || "http://localhost:5173" },
    data: {},
  });
  await page.reload();
  await page.getByRole("button", { name: "有邀请码？创建账号" }).click();
  await page
    .getByLabel("用户名", { exact: true })
    .fill("browser-" + Date.now());
  await page.getByLabel("密码（12–72 字节）").fill("browser-test-password");
  await page.getByLabel("一次性邀请码").fill(invite);
  await page.getByRole("button", { name: "注册并登录" }).click();
  await expect(page.getByRole("heading", { name: "我的创作" })).toBeVisible();
  await page.getByRole("button", { name: "商品资料", exact: true }).click();
  await page.getByLabel("商品名称 *").fill("山间柚子茶 · 演示样品");
  await page
    .getByLabel("核心卖点 *")
    .fill("柚子与茶香，清爽口感。测试素材，不对应实际商品。");
  const data = await page.evaluate(() => {
    const c = document.createElement("canvas");
    c.width = 800;
    c.height = 800;
    const g = c.getContext("2d")!;
    g.fillStyle = "#eee8db";
    g.fillRect(0, 0, 800, 800);
    g.fillStyle = "#7f9269";
    g.beginPath();
    g.roundRect(270, 190, 260, 470, 45);
    g.fill();
    g.fillStyle = "#d5c99c";
    g.fillRect(300, 130, 200, 80);
    g.fillStyle = "#f5f0df";
    g.fillRect(270, 340, 260, 200);
    g.fillStyle = "#3b5335";
    g.font = "bold 52px sans-serif";
    g.fillText("柚 香", 310, 425);
    g.font = "22px sans-serif";
    g.fillText("OWLET DEMO", 315, 485);
    return c.toDataURL("image/png").split(",")[1];
  });
  await page.locator("input[type=file]").setInputFiles({
    name: "demo-product.png",
    mimeType: "image/png",
    buffer: Buffer.from(data, "base64"),
  });
  await expect(page.locator(".upload-zone img")).toBeVisible();
  await page.getByRole("button", { name: "保存资料", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "用这个商品创作" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "用这个商品创作" }).click();
  await expect(page.getByLabel("本页标题")).toBeVisible();
  await page.getByLabel("本页标题").fill("一口柚香，慢下来");
  await page.getByLabel("本页文字").fill("柚子与茶香，让日常多一点清爽。");
  await page.getByLabel("标题", { exact: true }).fill("把清爽带进日常");
  await page
    .getByLabel("正文", { exact: true })
    .fill("柚子与茶的搭配，给日常留一点清爽。");
  await page.getByLabel("我已确认文案与分页").check();
  await expect(page.locator(".saved")).toContainText("已保存", {
    timeout: 10000,
  });
  await page.screenshot({
    path: testInfo.outputPath("editor-desktop.png"),
    fullPage: true,
  });
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "导出图文", exact: true }).click();
  const zip = await download;
  expect(zip.suggestedFilename()).toMatch(/\.zip$/);
  await zip.saveAs(testInfo.outputPath("four-pages.zip"));
  page.on("dialog", (d) => d.accept());
  await page.getByRole("button", { name: "生成 / 重写文案" }).click();
  await expect(page.getByRole("button", { name: "应用到草稿" })).toBeVisible({
    timeout: 20000,
  });
  await page.getByRole("button", { name: "应用到草稿" }).click();
  await expect(page.getByLabel("标题", { exact: true })).toHaveValue(
    /让日常多一点喜欢/,
  );
  await page.getByLabel("我已确认文案与分页").check();
  await expect(page.locator(".saved")).toContainText("已保存", {
    timeout: 10000,
  });
  await page.getByRole("button", { name: "生成一张共享背景" }).click();
  await expect(page.getByRole("button", { name: "应用到草稿" })).toBeVisible({
    timeout: 20000,
  });
  await page.getByRole("button", { name: "应用到草稿" }).click();
  await expect(page.locator(".saved")).toContainText("已保存", {
    timeout: 10000,
  });
  await page.getByLabel("本页标题", { exact: true }).fill("局部重写测试");
  await page.getByRole("button", { name: "重写本页文案", exact: true }).click();
  await expect(page.getByRole("button", { name: "应用到草稿" })).toBeVisible({
    timeout: 20000,
  });
  await page.getByRole("button", { name: "应用到草稿" }).click();
  await expect(page.getByLabel("标题", { exact: true })).toHaveValue(
    /让日常多一点喜欢/,
  );
  await expect(page.getByLabel("本页标题", { exact: true })).not.toHaveValue(
    "局部重写测试",
  );
  await expect(page.locator(".saved")).toContainText("已保存", {
    timeout: 10000,
  });
  await page.getByRole("button", { name: "返回", exact: true }).click();
  await page.reload();
  await expect(page.locator(".project-card")).toHaveCount(1);
  await page.screenshot({
    path: testInfo.outputPath("workspace-desktop.png"),
    fullPage: true,
  });
  await page.locator(".cover-button").click();
  await expect(page.getByLabel("标题", { exact: true })).toHaveValue(
    /让日常多一点喜欢/,
  );
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(
    page.getByText("排版编辑请使用电脑，手机可查看、复制和下载。"),
  ).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("editor-mobile.png"),
    fullPage: true,
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  expect(errors).toEqual([]);
  const mobileDownload = page.waitForEvent("download");
  await page.getByRole("button", { name: "下载这一页", exact: true }).click();
  expect((await mobileDownload).suggestedFilename()).toMatch(/\.png$/);
});
