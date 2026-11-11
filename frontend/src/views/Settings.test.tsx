import { createRoot } from "react-dom/client";
import { act } from "react";
import { describe, expect, it, vi } from "vitest";
import { Account, SettingsView, webAddress } from "./Settings";
import { useStore } from "../store";
import { api } from "../api";
import { t } from "../i18n";

vi.mock("../api", () => ({
  api: {
    getWelcome: () => Promise.resolve({ recent: [], name: "Ada", email: "", sitesFolder: "", githubSignIn: false, platform: "darwin", language: "", version: "9.8.7" }),
    saveSettings: vi.fn(() => Promise.resolve()),
    getSite: vi.fn(),
    getChanges: () => Promise.resolve([]),
    getSiteIcon: () => Promise.resolve({ image: "", letter: true }),
    getAuthStatus: () => Promise.resolve({}),
    getLanguages: vi.fn(() => Promise.resolve({ default: "en", fixed: false, languages: [{ code: "en", name: "English" }] })),
    saveLanguages: vi.fn(() => Promise.resolve()),
  },
}));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

describe("You tab", () => {
  it("shows the app version", async () => {
    const host = document.createElement("div");
    await act(async () => createRoot(host).render(<Account />));
    expect(host.textContent).toContain(t.settings.version("9.8.7"));
    expect(host.textContent).toContain("Pageroom 9.8.7");
  });
});

describe("webAddress", () => {
  it.each(["example.com", "example.com/", "https://example.com", "https://example.com/", "  example.com  "])("saves %j as https://example.com/", (typed) => {
    expect(webAddress(typed)).toBe("https://example.com/");
  });

  it("keeps an http scheme", () => {
    expect(webAddress("http://example.com")).toBe("http://example.com/");
  });

  it("leaves an empty address empty", () => {
    expect(webAddress("  ")).toBe("");
  });
});

describe("saving a site address", () => {
  const settings = { title: "Site", description: "", baseURL: "", formAddress: "" };
  const git = { isRepo: true, remote: "git@github.com:a/b.git", host: "", branch: "main", signedIn: true, published: false, cloudflare: { name: "mysite", setUp: true } };

  async function open(tab: "general" | "publishing") {
    const site = { settings, git, info: {}, tree: {}, styleHook: {} };
    useStore.setState({ site: site as never });
    vi.mocked(api.getSite).mockResolvedValue(site as never);
    vi.mocked(api.saveSettings).mockReset();
    vi.mocked(api.saveSettings).mockImplementation(async (saved) => {
      vi.mocked(api.getSite).mockResolvedValue({ ...site, settings: saved } as never);
    });
    const host = document.createElement("div");
    document.body.append(host);
    await act(async () => createRoot(host).render(<SettingsView tab={tab} />));
    return host;
  }

  async function type(input: HTMLInputElement, value: string) {
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, value);
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });
  }

  it("saves the General address in Hugo's form and shows it", async () => {
    const host = await open("general");
    const input = host.querySelector<HTMLInputElement>("#st-url")!;
    await type(input, "example.com");
    const save = [...host.querySelectorAll("button")].find((button) => button.textContent?.includes(t.settings.saveSettings))!;
    await act(async () => save.click());
    expect(api.saveSettings).toHaveBeenCalledWith({ ...settings, baseURL: "https://example.com/" });
    expect(input.value).toBe("https://example.com/");
    expect(save.disabled).toBe(true);
  });

  it("saves the Cloudflare address in Hugo's form", async () => {
    const host = await open("publishing");
    const input = host.querySelector<HTMLInputElement>('input[placeholder*="workers.dev"]')!;
    await type(input, "mysite.you.workers.dev");
    const save = input.nextElementSibling as HTMLButtonElement;
    await act(async () => save.click());
    expect(api.saveSettings).toHaveBeenCalledWith({ ...settings, baseURL: "https://mysite.you.workers.dev/" });
  });
});

describe("site languages", () => {
  it("adds a language from the list and saves it", async () => {
    const site = { settings: {}, git: {}, info: {}, tree: {}, styleHook: {} };
    useStore.setState({ site: site as never });
    vi.mocked(api.getSite).mockResolvedValue(site as never);
    const host = document.createElement("div");
    document.body.append(host);
    await act(async () => createRoot(host).render(<SettingsView tab="general" />));

    const select = host.querySelector<HTMLSelectElement>("#st-add-lang")!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!.call(select, "ja");
      select.dispatchEvent(new Event("change", { bubbles: true }));
    });
    const button = (text: string) => [...host.querySelectorAll("button")].find((candidate) => candidate.textContent?.includes(text))!;
    await act(async () => button(t.settings.addLanguageButton).click());
    expect(host.textContent).toContain("日本語");
    await act(async () => button(t.settings.saveLanguages).click());

    expect(api.saveLanguages).toHaveBeenCalledWith("en", [{ code: "en", name: "English" }, { code: "ja", name: "日本語" }]);
  });

  const render = async (settings: object) => {
    vi.mocked(api.getLanguages).mockResolvedValueOnce(settings as never);
    const site = { settings: {}, git: {}, info: {}, tree: {}, styleHook: {} };
    useStore.setState({ site: site as never });
    vi.mocked(api.getSite).mockResolvedValue(site as never);
    const host = document.createElement("div");
    document.body.append(host);
    await act(async () => createRoot(host).render(<SettingsView tab="general" />));
    return host;
  };
  const labelled = (host: HTMLElement, text: string) => host.querySelector<HTMLButtonElement>(`button[aria-label="${text}"]`);

  it("keeps the default first with no move buttons", async () => {
    const host = await render({ default: "en", fixed: true, languages: [{ code: "ja", name: "日本語" }, { code: "en", name: "English" }, { code: "fr", name: "Français" }] });
    expect(labelled(host, t.settings.moveLanguageUp("English"))).toBeNull();
    expect(labelled(host, t.settings.moveLanguageDown("English"))).toBeNull();
    expect(labelled(host, t.settings.moveLanguageUp("日本語"))!.disabled).toBe(true);

    await act(async () => labelled(host, t.settings.moveLanguageDown("日本語"))!.click());
    await act(async () => [...host.querySelectorAll("button")].find((candidate) => candidate.textContent?.includes(t.settings.saveLanguages))!.click());
    expect(api.saveLanguages).toHaveBeenLastCalledWith("en", [{ code: "en", name: "English" }, { code: "fr", name: "Français" }, { code: "ja", name: "日本語" }]);
  });

  it("saves the own name of a language without one", async () => {
    const host = await render({ default: "en", fixed: true, languages: [{ code: "en", name: "en" }] });
    expect(host.textContent).toContain("English");
    const select = host.querySelector<HTMLSelectElement>("#st-add-lang")!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value")!.set!.call(select, "ja");
      select.dispatchEvent(new Event("change", { bubbles: true }));
    });
    const button = (text: string) => [...host.querySelectorAll("button")].find((candidate) => candidate.textContent?.includes(text))!;
    await act(async () => button(t.settings.addLanguageButton).click());
    await act(async () => button(t.settings.saveLanguages).click());
    expect(api.saveLanguages).toHaveBeenLastCalledWith("en", [{ code: "en", name: "English" }, { code: "ja", name: "日本語" }]);
  });
});
