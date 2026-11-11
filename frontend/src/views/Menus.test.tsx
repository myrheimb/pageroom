import { createRoot } from "react-dom/client";
import { act } from "react";
import { describe, expect, it, vi } from "vitest";
import { MenusView } from "./Menus";
import { api } from "../api";
import { useStore } from "../store";
import { t } from "../i18n";

vi.mock("../api", () => ({ api: { getMenus: vi.fn() } }));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

describe("menus screen", () => {
  it("starts on the site's first menu when there is no main menu", async () => {
    vi.mocked(api.getMenus).mockResolvedValue([
      { name: "header", language: "en", items: [{ id: "a", name: "Docs", pageRef: "/docs", weight: 10, source: "config" }] },
      { name: "footer", language: "en", items: [{ id: "b", name: "Legal", pageRef: "/legal", weight: 10, source: "config" }] },
    ] as never);
    useStore.setState({ site: { info: { languages: ["en"] }, tree: { path: "content", children: [] } } as never });
    const host = document.createElement("div");
    await act(async () => createRoot(host).render(<MenusView />));
    expect(host.querySelector(".seg button.on")!.textContent).toBe(t.menus.namedMenu("header"));
    expect(host.textContent).toContain("Docs");
    expect(host.textContent).not.toContain("Legal");
    expect(host.textContent).not.toContain(t.menus.themeDecides("header"));
  });

  it("switches the menus with the language on a multilingual site", async () => {
    vi.mocked(api.getMenus).mockResolvedValue([
      { name: "main", language: "en", items: [{ id: "a", name: "Docs", pageRef: "/docs", weight: 10, source: "config" }] },
      { name: "main", language: "ja", items: [{ id: "a", name: "資料", pageRef: "/docs", weight: 10, source: "config" }] },
    ] as never);
    useStore.setState({ site: { info: { languages: ["en", "ja"] }, tree: { path: "content", children: [] } } as never });
    const host = document.createElement("div");
    await act(async () => createRoot(host).render(<MenusView />));
    expect(host.textContent).toContain("Docs");
    expect(host.textContent).not.toContain("資料");
    const ja = [...host.querySelectorAll<HTMLButtonElement>(`[aria-label="${t.menus.language}"] button`)].find((b) => b.textContent === "ja")!;
    await act(async () => ja.click());
    expect(host.textContent).toContain("資料");
    expect(host.textContent).not.toContain("Docs");
  });
});
