import { assetURL, type Body, type Page } from "./api";
async function picture(id: string) {
  const im = new Image();
  im.src = assetURL(id);
  await im.decode();
  return im;
}
function lines(ctx: CanvasRenderingContext2D, text: string, max: number) {
  const out: string[] = [];
  for (const paragraph of text.split("\n")) {
    let line = "";
    for (const c of paragraph) {
      if (ctx.measureText(line + c).width > max && line) {
        out.push(line);
        line = c;
      } else line += c;
    }
    out.push(line);
  }
  return out;
}
export async function draw(
  canvas: HTMLCanvasElement,
  body: Body,
  page: Page,
  index: number,
) {
  await document.fonts.ready;
  const ctx = canvas.getContext("2d")!;
  const W = 900,
    H = 1200;
  canvas.width = W;
  canvas.height = H;
  ctx.fillStyle = body.background || "#f5eee5";
  ctx.fillRect(0, 0, W, H);
  if (page.backgroundId) {
    const im = await picture(page.backgroundId);
    const s = Math.max(W / im.width, H / im.height);
    ctx.drawImage(
      im,
      (W - im.width * s) / 2,
      (H - im.height * s) / 2,
      im.width * s,
      im.height * s,
    );
  }
  const accent = body.color || "#ff2442";
  if (body.template === "promo") {
    ctx.fillStyle = accent;
    ctx.fillRect(0, 0, W, 235);
  } else if (body.template === "life") {
    ctx.fillStyle = "#ffffffbb";
    ctx.fillRect(28, 28, W - 56, H - 56);
  }
  ctx.fillStyle = body.template === "promo" ? "#fff" : "#222";
  ctx.font = '700 64px "Microsoft YaHei", "PingFang SC", sans-serif';
  const title = lines(ctx, page.title, W - 110);
  title.slice(0, 3).forEach((s, i) => ctx.fillText(s, 55, 105 + i * 78));
  if (page.assetId) {
    const im = await picture(page.assetId);
    const width = (W * page.scale) / 100,
      height = width;
    const s = Math.max(width / im.width, height / im.height);
    const sw = width / s,
      sh = height / s;
    const sx = ((im.width - sw) * page.cropX) / 100,
      sy = ((im.height - sh) * page.cropY) / 100;
    ctx.drawImage(
      im,
      sx,
      sy,
      sw,
      sh,
      (W * page.x) / 100 - width / 2,
      (H * page.y) / 100 - height / 2,
      width,
      height,
    );
  }
  ctx.fillStyle = "#ffffffea";
  ctx.fillRect(35, 970, 830, 190);
  ctx.fillStyle = "#333";
  ctx.font = '400 32px "Microsoft YaHei", "PingFang SC", sans-serif';
  const text = lines(ctx, page.text, 770);
  text.slice(0, 3).forEach((s, i) => ctx.fillText(s, 65, 1020 + i * 43));
  if (body.brand?.logoId) {
    const logo = await picture(body.brand.logoId);
    const ratio = Math.min(60 / logo.width, 35 / logo.height);
    ctx.drawImage(logo, W - 90, 1160, logo.width * ratio, logo.height * ratio);
  }
  ctx.fillStyle = accent;
  ctx.font = "600 21px sans-serif";
  ctx.fillText(
    (body.brand?.name || "OWLET") +
      "  /  " +
      String(index + 1).padStart(2, "0"),
    55,
    1180,
  );
  return title.length > 3 || text.length > 3;
}
export async function png(body: Body, page: Page, index: number) {
  const c = document.createElement("canvas");
  const overflow = await draw(c, body, page, index);
  if (overflow)
    throw new Error("第 " + (index + 1) + " 页文字溢出，请缩短标题或正文");
  return new Promise<Blob>((resolve, reject) =>
    c.toBlob(
      (b) => (b ? resolve(b) : reject(new Error("导出失败"))),
      "image/png",
    ),
  );
}
export function download(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
