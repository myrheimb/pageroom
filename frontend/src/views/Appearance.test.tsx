import { createRoot } from "react-dom/client";
import { act } from "react";
import { describe, expect, it, vi } from "vitest";
import { loadSiteFonts, RangeRow } from "./Appearance";
import { t } from "../i18n";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const renderRadius = (value: number) => {
  const host = document.createElement("div");
  act(() => createRoot(host).render(<RangeRow label="Corners" unit="px" min={0} max={24} value={value} unsetValue={-1} onChange={() => {}} />));
  return host;
};

describe("rounded corners slider", () => {
  it("shows 0 as square corners, not as theme default", () => {
    const host = renderRadius(0);
    expect(host.textContent).toContain("0px");
    expect(host.textContent).not.toContain(t.appearance.themeDefault);
    expect(host.querySelector("button")).not.toBeNull();
  });

  it("shows -1 as theme default with nothing to reset", () => {
    const host = renderRadius(-1);
    expect(host.textContent).toContain(t.appearance.themeDefault);
    expect(host.querySelector("button")).toBeNull();
  });
});

describe("site fonts", () => {
  it("registers each font file with its site file URL", () => {
    const sources: unknown[][] = [];
    const added: unknown[] = [];
    class FakeFontFace {
      constructor(...args: unknown[]) { sources.push(args); }
      load() { return Promise.resolve(this); }
    }
    vi.stubGlobal("FontFace", FakeFontFace);
    Object.defineProperty(document, "fonts", { configurable: true, value: { add: (face: unknown) => added.push(face), delete: () => {} } });

    loadSiteFonts([{ family: "Manrope", fallback: "system", files: [{ path: "fonts/manrope.woff2", weight: "200 800", style: "normal" }] }]);

    expect(sources).toEqual([["Manrope", 'url("/site-file/static/fonts/manrope.woff2")', { weight: "200 800", style: "normal", unicodeRange: undefined }]]);
    expect(added).toHaveLength(1);
    vi.unstubAllGlobals();
  });
});
