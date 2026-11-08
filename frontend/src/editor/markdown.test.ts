import { describe, expect, it } from "vitest";
import { htmlToMarkdown, liftShortcodes, markdownToHtml, stripUnsafeUrls } from "./markdown";

const roundTrip = (md: string) => htmlToMarkdown(markdownToHtml(md));

describe("markdown round trip", () => {
  it("keeps ordinary formatting", () => {
    const md = "# Title\n\nSome **bold** and _italic_ text with a [link](https://example.org).\n\n- one\n- two\n\n> quote\n";
    expect(roundTrip(md)).toBe(md);
  });

  it("keeps shortcodes byte for byte", () => {
    const md = 'Intro.\n\n{{< youtube id="dQw4w9WgXcQ" >}}\n\nInline {{< param "author" >}} here.\n';
    expect(roundTrip(md)).toBe(md);
  });

  it("keeps paired shortcodes with inner markdown", () => {
    const md = "{{< notice warning >}}\nBe **careful**.\n{{< /notice >}}\n\nAfter.\n";
    const lifted = liftShortcodes(md);
    expect(lifted.blocks).toHaveLength(1);
    expect(roundTrip(md)).toBe(md);
  });

  it("does not touch shortcodes inside code", () => {
    const md = "Use `{{< x >}}` like this:\n\n```\n{{< youtube abc >}}\n```\n";
    expect(liftShortcodes(md).blocks).toHaveLength(0);
    expect(liftShortcodes(md).inlines).toHaveLength(0);
  });

  it("keeps the summary divider and raw HTML", () => {
    const md = "Summary.\n\n<!--more-->\n\nRest with <span class=\"x\">inline</span> HTML.\n\n<div class=\"box\">\n  <p>raw</p>\n</div>\n";
    expect(roundTrip(md)).toBe(md);
  });

  it("keeps images with alt and title", () => {
    const md = 'Text.\n\n![A beach at sunset](beach-day.jpg "Sunset")\n\nMore.\n';
    expect(roundTrip(md)).toBe(md);
  });

  it("handles task lists", () => {
    const md = "- [ ] buy milk\n- [x] write post\n";
    expect(markdownToHtml(md)).toContain('data-type="taskList"');
    expect(roundTrip(md)).toBe(md);
  });

  it("handles tables", () => {
    const md = "| Name | Age |\n| --- | --- |\n| Ada | 36 |\n";
    expect(roundTrip(md)).toBe(md);
  });

  it("handles unicode in shortcodes", () => {
    const md = '{{< figure src="ø.jpg" caption="Blåbær 🫐" >}}\n';
    expect(roundTrip(md)).toBe(md);
  });

  it("handles nested and ordered lists, code and breaks", () => {
    const md = "1. First\n2. Second\n   - nested a\n   - nested b\n3. Third\n\n```js\nconst x = 1;\n```\n\nLine one  \nline two\n";
    expect(roundTrip(md)).toBe(md);
  });
});

describe("stripUnsafeUrls", () => {
  it("keeps http, https, mailto and relative links only", () => {
    const doc = new DOMParser().parseFromString(
      '<a id="a" href="https://x.org">a</a><a id="b" href="mailto:a@b.c">b</a><a id="c" href="/rel">c</a>' +
        '<a id="d" href="data:text/html,x">d</a><a id="e" href=" JaVa&#9;script:alert(1)">e</a><a id="f" href="vbscript:x">f</a>',
      "text/html",
    );
    stripUnsafeUrls(doc);
    const kept = ["a", "b", "c", "d", "e", "f"].filter((id) => doc.getElementById(id)!.hasAttribute("href"));
    expect(kept).toEqual(["a", "b", "c"]);
  });
});
