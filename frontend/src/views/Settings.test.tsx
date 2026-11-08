import { createRoot } from "react-dom/client";
import { act } from "react";
import { describe, expect, it, vi } from "vitest";
import { Account } from "./Settings";
import { t } from "../i18n";

vi.mock("../api", () => ({
  api: { getWelcome: () => Promise.resolve({ recent: [], name: "Ada", email: "", sitesFolder: "", githubSignIn: false, platform: "darwin", language: "", version: "9.8.7" }) },
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
