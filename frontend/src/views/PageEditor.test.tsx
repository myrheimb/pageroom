import { createRoot } from "react-dom/client";
import { act } from "react";
import { describe, expect, it, vi } from "vitest";
import { PageEditor, CoverPicture, setCoverFields } from "./PageEditor";
import { t } from "../i18n";

const { api } = vi.hoisted(() => {
  const page = { path: "content/a.md", url: "/a/", isSection: false, fields: { title: "A" }, keys: ["title"], body: "", hash: "h1" };
  return { api: { getPage: vi.fn(async () => page), savePage: vi.fn(async () => ({ ...page, hash: "h2" })), getTree: vi.fn(async () => ({})) } };
});

vi.mock("../api", () => ({ api, on: () => () => {}, resolveImageRef: (src: string) => src, siteFileURL: (p: string) => "/files/" + p, translate: (e: unknown) => e }));
vi.mock("../editor/BlockEditor", () => ({ BlockEditor: () => null }));
vi.mock("./PageSettings", () => ({ PageSettings: () => null }));
vi.mock("./History", () => ({ HistoryDialog: () => null }));
vi.mock("./Preview", () => ({ PreviewPane: () => null }));
vi.mock("./MediaPicker", () => ({
  MediaPicker: ({ onPick }: { onPick: (r: { ref: string }) => void }) => <button onClick={() => onPick({ ref: "beach.jpg" })}>pick-beach</button>,
}));
vi.mock("../ui", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../ui")>()),
  ask: { prompt: vi.fn(async () => "A beach"), confirm: vi.fn() },
}));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

async function mount(props: Partial<Parameters<typeof CoverPicture>[0]>) {
  const onChange = vi.fn();
  const host = document.createElement("div");
  await act(async () =>
    createRoot(host).render(
      <CoverPicture image="" alt="" resolve={(src) => "/files/" + src} pick={async () => null} describe={async () => null} onChange={onChange} {...props} />,
    ),
  );
  return { host, onChange };
}

const click = (host: HTMLElement, label: string) =>
  act(async () => {
    Array.from(host.querySelectorAll("button")).find((button) => button.textContent?.includes(label))!.click();
  });

describe("cover picture on the canvas", () => {
  it("saves the picked picture and the typed description", async () => {
    const { host, onChange } = await mount({ pick: async () => ({ ref: "beach.jpg" }), describe: async () => "A beach" });
    expect(host.textContent).toContain(t.editor.coverNote);
    await click(host, t.editor.coverAdd);
    expect(onChange).toHaveBeenCalledWith("beach.jpg", "A beach");
  });

  it("keeps the description the picker already has", async () => {
    const describe = vi.fn();
    const { host, onChange } = await mount({ pick: async () => ({ ref: "a.jpg", alt: "From library" }), describe });
    await click(host, t.editor.coverAdd);
    expect(describe).not.toHaveBeenCalled();
    expect(onChange).toHaveBeenCalledWith("a.jpg", "From library");
  });

  it("shows the picture with its description and removes both", async () => {
    const { host, onChange } = await mount({ image: "beach.jpg", alt: "A beach" });
    const img = host.querySelector("img")!;
    expect(img.getAttribute("src")).toBe("/files/beach.jpg");
    expect(img.getAttribute("alt")).toBe("A beach");
    await click(host, t.editor.coverRemove);
    expect(onChange).toHaveBeenCalledWith(undefined, undefined);
  });
});

describe("saving the cover", () => {
  it("writes the picture to image and its description to imageAlt", () => {
    const setField = vi.fn();
    setCoverFields(setField, "beach.jpg", "A beach");
    expect(setField).toHaveBeenCalledWith("image", "beach.jpg");
    expect(setField).toHaveBeenCalledWith("imageAlt", "A beach");
  });
});

describe("the page editor's cover", () => {
  it("saves the picked picture to image and the typed description to imageAlt", async () => {
    const host = document.createElement("div");
    await act(async () => createRoot(host).render(<PageEditor path="content/a.md" onPathChange={() => {}} onDeleted={() => {}} />));
    await click(host, t.editor.coverAdd);
    await click(host, "pick-beach");
    await act(async () => {
      window.dispatchEvent(new KeyboardEvent("keydown", { key: "s", metaKey: true }));
    });
    expect(api.savePage).toHaveBeenCalledWith(expect.objectContaining({ set: { image: "beach.jpg", imageAlt: "A beach" } }));
  });
});
