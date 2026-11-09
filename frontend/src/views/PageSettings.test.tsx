import { createRoot } from "react-dom/client";
import { act } from "react";
import { describe, expect, it, vi } from "vitest";
import { PageSettings } from "./PageSettings";
import { t } from "../i18n";

vi.mock("../api", () => ({ api: {} }));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const page = { path: "/content/posts/a/index.md", url: "/posts/a/", isSection: false, fields: {}, keys: [], body: "", hash: "" };

async function render(fields: Record<string, unknown>, shown = page) {
  const host = document.createElement("div");
  await act(async () =>
    createRoot(host).render(
      <PageSettings page={shown as never} fields={fields} setField={() => {}} resolveImage={() => null} pickImage={async () => null} onRenamed={() => {}} save={async () => {}} />,
    ),
  );
  return host;
}

describe("page settings panel", () => {
  it("has a Page title field on the home page only", async () => {
    const home = await render({ title: "Welcome" }, { ...page, url: "/", isSection: true });
    expect(home.querySelector<HTMLInputElement>("#ps-title")!.value).toBe("Welcome");
    expect(home.textContent).toContain(t.pageEditor.pageTitle);
    expect((await render({ title: "A" })).querySelector("#ps-title")).toBeNull();
  });

  it("has no Featured image control, even when the page has an image", async () => {
    const host = await render({ title: "A", image: "beach.jpg", imageAlt: "A beach" });
    expect(host.textContent).not.toMatch(/featured image/i);
    expect(host.textContent).not.toContain(t.pageSettings.chooseImage);
    expect(host.querySelector("img")).toBeNull();
    expect(host.textContent).not.toContain("imageAlt");
  });

  it("says where each group shows up, and keeps Other settings collapsed", async () => {
    const host = await render({ title: "A", tags: ["x"], extra: "1" });
    for (const note of [t.pageSettings.publishingNote, t.pageSettings.addressNote, t.pageSettings.searchSharingNote, t.pageSettings.organizeNote]) {
      expect(host.textContent).toContain(note);
    }
    expect(host.textContent).toContain(t.pageSettings.otherSettings(1));
    expect(host.textContent).not.toContain(t.pageSettings.otherSettingsHelp);
  });
});
