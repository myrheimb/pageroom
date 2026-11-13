import { createRoot } from "react-dom/client";
import { act, useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { RepeatField } from "./RepeatField";
import { PageSettings } from "./PageSettings";
import { t } from "../i18n";
import { useStore } from "../store";

vi.mock("../api", () => ({ api: {} }));

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const def = {
  key: "steps",
  label: "Steps",
  type: "repeat",
  fields: [
    { key: "title", label: "Title", type: "text" },
    { key: "text", label: "Text", type: "textarea" },
    { key: "slides", label: "Slides", type: "repeat", fields: [{ key: "before", label: "Before", type: "image" }, { key: "after", label: "After", type: "image" }] },
  ],
};

const steps = [
  { title: "One", text: "Alpha", slides: [{ before: "a.jpg", after: "b.jpg" }], legacy: "keep me" },
  { title: "Two", text: "Beta", slides: [] },
];

async function render(initial: unknown, pick = async () => ({ ref: "new.jpg" })) {
  const sent: unknown[] = [];
  function Harness() {
    const [value, setValue] = useState(initial);
    return <RepeatField def={def} value={value} onChange={(v) => { sent.push(v); setValue(v); }} resolveImage={(s) => "/img/" + s} pickImage={pick} />;
  }
  const host = document.createElement("div");
  document.body.append(host);
  await act(async () => createRoot(host).render(<Harness />));
  return { host, sent };
}

function type(input: HTMLInputElement | HTMLTextAreaElement, text: string) {
  const proto = input instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  Object.getOwnPropertyDescriptor(proto, "value")!.set!.call(input, text);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

describe("repeat field", () => {
  it("shows a section with a card per item and its nested cards", async () => {
    const { host } = await render(steps);
    expect(host.querySelector("h3")!.textContent).toBe("Steps");
    expect(host.querySelectorAll(".repeat-card").length).toBe(3);
    expect((host.querySelector("#rf-0-title") as HTMLInputElement).value).toBe("One");
    expect((host.querySelector("#rf-1-text") as HTMLTextAreaElement).value).toBe("Beta");
  });

  it("sends the whole value with one edited title and every other key unchanged", async () => {
    const { host, sent } = await render(steps);
    await act(async () => type(host.querySelector("#rf-1-title")!, "Deux"));
    expect(sent).toEqual([[steps[0], { ...steps[1], title: "Deux" }]]);
  });

  it("sends the whole value with a nested slide image changed", async () => {
    const { host, sent } = await render(steps);
    const replace = host.querySelectorAll<HTMLButtonElement>(".repeat-card .repeat-card button")[0];
    expect(replace.textContent).toBe(t.common.replace);
    await act(async () => replace.click());
    expect(sent).toEqual([[{ ...steps[0], slides: [{ before: "new.jpg", after: "b.jpg" }] }, steps[1]]]);
  });

  it("shows a key the schema doesn't name read-only and keeps it on save", async () => {
    const { host, sent } = await render(steps);
    const legacy = host.querySelector<HTMLInputElement>("#rf-0-raw-legacy")!;
    expect(legacy.readOnly).toBe(true);
    expect(legacy.value).toBe("keep me");
    await act(async () => type(host.querySelector("#rf-0-title")!, "Uno"));
    expect((sent[0] as any[])[0].legacy).toBe("keep me");
  });

  it("renders an empty nested list", async () => {
    const { host } = await render(steps);
    const second = host.querySelectorAll(".repeat-card")[2].parentElement!;
    expect(second.textContent).toContain(t.repeat.empty);
  });
});

describe("repeat field in the side panel", () => {
  it("never shows in the side panel or under Other settings", async () => {
    useStore.setState({ site: { info: { prefs: { fields: [def] } } } } as never);
    const host = document.createElement("div");
    const page = { path: "content/solution.md", url: "/solution/", isSection: false, fields: {}, keys: [], body: "", hash: "" };
    await act(async () =>
      createRoot(host).render(
        <PageSettings page={page as never} fields={{ steps }} setField={() => {}} resolveImage={() => null} pickImage={async () => null} onRenamed={() => {}} save={async () => {}} />,
      ),
    );
    expect(host.textContent).not.toContain("Steps");
    expect(host.textContent).not.toContain(t.pageSettings.otherSettings(1));
  });
});
