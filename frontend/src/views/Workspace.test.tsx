import { createRoot } from "react-dom/client";
import { act } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Workspace } from "./Workspace";
import { api, on } from "../api";
import { useStore } from "../store";

vi.mock("../api", () => ({
  api: {
    getSite: vi.fn(),
    getTree: vi.fn(),
    getChanges: vi.fn(),
    sync: vi.fn(),
    confirmClose: vi.fn(),
  },
  on: vi.fn(() => () => {}),
}));
vi.mock("./Dashboard", () => ({ Dashboard: () => null }));
vi.mock("./Pages", () => ({ PagesView: () => null }));
vi.mock("./Media", () => ({ MediaView: () => null }));
vi.mock("./Preview", () => ({ SitePreview: () => null }));
vi.mock("./Menus", () => ({ MenusView: () => null }));
vi.mock("./Appearance", () => ({ AppearanceView: () => null }));
vi.mock("./Settings", () => ({ SettingsView: () => null }));
vi.mock("../dialogs/Publish", () => ({ PublishDialog: () => null, ConflictResolver: () => null }));
vi.mock("../dialogs/WordPress", () => ({ ImportReport: () => null }));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const site = { info: { name: "Bakery", baseURL: "" }, git: {}, tree: { children: [] } };

async function fire(event: string) {
  const call = vi.mocked(on).mock.calls.find(([name]) => name === event)!;
  await act(async () => call[1]());
}

describe("workspace file events", () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    vi.mocked(api.getChanges).mockResolvedValue([]);
    vi.mocked(api.getTree).mockResolvedValue({ children: [] } as never);
    vi.mocked(api.getSite).mockResolvedValue(site as never);
    useStore.setState({ site: site as never, view: "dashboard" as never });
    await act(async () => createRoot(document.createElement("div")).render(<Workspace />));
    vi.mocked(api.getSite).mockClear();
    vi.mocked(api.getTree).mockClear();
  });

  it("re-fetches the site when .pageroom.yaml changes", async () => {
    await fire("site:prefs-changed");
    expect(api.getSite).toHaveBeenCalled();
  });

  it("refreshes only the tree on a plain file change, even when that fails", async () => {
    vi.mocked(api.getTree).mockRejectedValue(new Error("gone"));
    await fire("site:files-changed");
    expect(api.getTree).toHaveBeenCalled();
    expect(api.getSite).not.toHaveBeenCalled();
  });
});
