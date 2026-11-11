import { createRoot } from "react-dom/client";
import { act } from "react";
import { describe, expect, it, vi } from "vitest";
import { PageTree } from "./PageTree";
import type { TreeNode } from "../api";
import { t } from "../i18n";

const { createPage } = vi.hoisted(() => ({ createPage: vi.fn(async () => "content/new.ja.md") }));

vi.mock("../api", () => ({ api: { createPage } }));
vi.mock("../ui", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../ui")>()),
  ask: { prompt: vi.fn(async () => "Hello"), confirm: vi.fn() },
  Menu: ({ items }: { items: { label?: string; run?: () => void }[] }) => (
    <>{items.map((item, index) => item.label && <button key={index} onClick={item.run}>{item.label}</button>)}</>
  ),
}));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const page = (path: string, title: string, translations: { language: string; path: string }[]): TreeNode => ({
  path, kind: "page", title, draft: false, weight: 0, bundle: false, url: "/" + title + "/",
  translations: translations.map((x) => ({ ...x, url: "/" })),
});
const tree: TreeNode = {
  path: "content", kind: "section", title: "Home", draft: false, weight: 0, bundle: false, url: "/",
  children: [
    page("content/about.md", "About", [{ language: "en", path: "content/about.md" }, { language: "ja", path: "content/about.ja.md" }]),
    page("content/news.md", "News", [{ language: "en", path: "content/news.md" }]),
  ],
};

async function mount(languages: string[], selected?: string) {
  const onOpen = vi.fn();
  const host = document.createElement("div");
  await act(async () =>
    createRoot(host).render(<PageTree tree={tree} selected={selected} languages={languages} changed={new Set()} onOpen={onOpen} onChanged={() => {}} labelFor={(s) => s.title} />),
  );
  return { host, onOpen };
}

const tagLabels = (host: HTMLElement) => Array.from(host.querySelectorAll(".lang-tags button")).map((b) => b.textContent);

describe("page tree languages", () => {
  it("shows one row per page with a tag for each of its languages", async () => {
    const { host } = await mount(["en", "ja"]);
    expect(host.querySelectorAll('[role="treeitem"]').length).toBe(3);
    expect(tagLabels(host)).toEqual(["en", "ja", "en"]);
  });

  it("opens the file of the tag that was clicked", async () => {
    const { host, onOpen } = await mount(["en", "ja"]);
    await act(async () => host.querySelector<HTMLButtonElement>(`[aria-label="${t.pageTree.openLanguage("ja")}"]`)!.click());
    expect(onOpen).toHaveBeenCalledWith("content/about.ja.md");
  });

  it("creates new pages in the language being edited", async () => {
    const { host } = await mount(["en", "ja"], "content/about.ja.md");
    await act(async () => Array.from(host.querySelectorAll("button")).find((button) => button.textContent === t.pageTree.newPage)!.click());
    expect(createPage).toHaveBeenCalledWith({ section: "content", title: "Hello", bundle: true, language: "ja" });
  });

  it("shows no language controls on a single-language site", async () => {
    const { host } = await mount(["en"]);
    expect(host.querySelector(".lang-tags")).toBeNull();
  });
});
