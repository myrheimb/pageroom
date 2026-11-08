import { createRoot } from "react-dom/client";
import { act } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { UpdateBanner } from "./Dashboard";
import { api } from "../api";
import { t } from "../i18n";

vi.mock("../api", () => ({
  api: { checkForUpdate: vi.fn(), openExternal: vi.fn(() => Promise.resolve()) },
}));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

async function render() {
  const host = document.createElement("div");
  await act(async () => createRoot(host).render(<UpdateBanner />));
  return host;
}

describe("update banner", () => {
  beforeEach(() => vi.clearAllMocks());

  it("shows a newer release and opens its page", async () => {
    const url = "https://github.com/myrheimb/pageroom/releases/tag/v0.2.0";
    vi.mocked(api.checkForUpdate).mockResolvedValue({ version: "0.2.0", url });
    const host = await render();
    expect(host.textContent).toContain(t.dashboard.updateAvailable("0.2.0"));
    await act(async () => host.querySelector("button")!.click());
    expect(api.openExternal).toHaveBeenCalledWith(url);
  });

  it("shows nothing without a newer release", async () => {
    vi.mocked(api.checkForUpdate).mockResolvedValue(null);
    expect((await render()).textContent).toBe("");
  });

  it("shows nothing when the check fails", async () => {
    vi.mocked(api.checkForUpdate).mockRejectedValue(new Error("offline"));
    expect((await render()).textContent).toBe("");
  });
});
