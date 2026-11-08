import { mkdir } from "node:fs/promises";
import { resolve } from "node:path";
import { chromium, type Page } from "playwright-core";
import { createServer } from "vite";

const outputFolder = resolve(import.meta.dirname, "../../docs/screenshots");
const viewport = { width: 1360, height: 860 };

const server = await createServer({ configFile: resolve(import.meta.dirname, "vite.config.ts"), logLevel: "warn" });
await server.listen();
const serverAddress = server.resolvedUrls?.local[0];
if (!serverAddress) throw new Error("The Vite server has no address.");
const address: string = serverAddress;

const browser = await chromium.launch();

async function openSite(remote = false): Promise<Page> {
  const context = await browser.newContext({ viewport, deviceScaleFactor: 1, locale: "en-US", timezoneId: "UTC" });
  const page = await context.newPage();
  await page.clock.setFixedTime(new Date("2026-01-05T09:30:00Z"));
  await page.addInitScript((remoteSite) => {
    Object.defineProperty(navigator, "platform", { value: "Linux x86_64" });
    window.mockRemote = remoteSite;
  }, remote);
  await page.goto(address);
  await page.getByText("Hannah's Bakery").first().click();
  await page.getByRole("button", { name: "Dashboard" }).waitFor();
  return page;
}

async function openPost(page: Page) {
  await page.getByRole("button", { name: "Pages & posts" }).click();
  await page.getByRole("treeitem", { name: "Hello, world" }).click();
  await page.locator(".ProseMirror").waitFor();
}

async function save(page: Page, name: string) {
  await page.mouse.move(viewport.width, viewport.height);
  await page.waitForTimeout(500);
  await page.screenshot({ path: resolve(outputFolder, `${name}.png`) });
  await page.context().close();
  console.log(`${name}.png`);
}

await mkdir(outputFolder, { recursive: true });

const dashboard = await openSite();
await save(dashboard, "dashboard");

const editor = await openSite();
await openPost(editor);
await editor.evaluate(() => {
  const paragraph = [...document.querySelectorAll(".ProseMirror p")].find((element) => element.textContent?.includes("sourdough"));
  const text = paragraph?.firstChild;
  const start = text?.textContent?.indexOf("baked") ?? -1;
  if (!text || start < 0) throw new Error("The sample paragraph is missing.");
  (document.querySelector(".ProseMirror") as HTMLElement).focus();
  const range = document.createRange();
  range.setStart(text, start);
  range.setEnd(text, start + "baked".length);
  const selection = window.getSelection();
  selection?.removeAllRanges();
  selection?.addRange(range);
});
await editor.locator(".bubble").waitFor();
await save(editor, "editor");

const preview = await openSite();
await openPost(preview);
await preview.getByRole("button", { name: "Toggle page list" }).click();
await preview.getByRole("button", { name: "Preview", exact: true }).click();
await preview.frameLocator("iframe").getByRole("heading", { name: "Hello, world" }).waitFor();
await save(preview, "preview");

const appearance = await openSite();
await appearance.getByRole("button", { name: "Appearance" }).click();
await appearance.frameLocator("iframe").getByRole("heading").first().waitFor();
await save(appearance, "appearance");

const conflict = await openSite(true);
await openPost(conflict);
await conflict.getByRole("button", { name: /^Publish/ }).click();
await conflict.getByRole("button", { name: "Publish now" }).click();
await conflict.getByText("Ben changed the same thing you did").waitFor();
await save(conflict, "conflict");

await browser.close();
await server.close();
