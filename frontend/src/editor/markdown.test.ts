import { Editor } from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import { describe, expect, it } from "vitest";
import { htmlToMarkdown, liftShortcodes, markdownToHtml, stripUnsafeUrls } from "./markdown";
import { ButtonBlock, CardBlock, CardsBlock, HeroBlock, ImageBlock, RawHtmlBlock, RawHtmlInline, ShortcodeBlock, ShortcodeInline } from "./nodes";

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

const hero = '{{< pr-hero image="beach.jpg" alt="A beach" button="Read more" link="/about/" >}}\n## Welcome\n\nA **bold** start with a [link](https://example.org).\n{{< /pr-hero >}}\n';
const cards = "{{< pr-cards >}}\n{{< pr-card >}}\n### One\n\nFirst _card_.\n{{< /pr-card >}}\n\n{{< pr-card >}}\n### Two\n\n- a\n- b\n{{< /pr-card >}}\n{{< /pr-cards >}}\n";
const button = '{{< pr-button link="/contact/" >}}Get in touch{{< /pr-button >}}\n';

function editorFor(markdown: string) {
  return new Editor({
    extensions: [StarterKit, ImageBlock, ShortcodeBlock, ShortcodeInline, RawHtmlBlock, RawHtmlInline, HeroBlock, CardsBlock, CardBlock, ButtonBlock],
    content: markdownToHtml(markdown),
  });
}

describe("page blocks", () => {
  it("round trips a hero with inner formatting", () => {
    expect(roundTrip(hero)).toBe(hero);
    expect(markdownToHtml(hero)).toContain('data-pr="hero"');
  });

  it("round trips cards with nested card items", () => {
    expect(roundTrip(cards)).toBe(cards);
    expect(markdownToHtml(cards).match(/data-pr="card"/g)).toHaveLength(2);
  });

  it("round trips a button", () => {
    expect(roundTrip(button)).toBe(button);
    expect(markdownToHtml(button)).toContain("Get in touch");
  });

  it("round trips blocks between ordinary paragraphs", () => {
    const md = `Before.\n\n${hero}\nMiddle.\n\n${cards}\n${button}\nAfter.\n`;
    expect(roundTrip(md)).toBe(md);
  });

  it("keeps empty blocks", () => {
    const md = "{{< pr-hero >}}\n{{< /pr-hero >}}\n\n{{< pr-button >}}{{< /pr-button >}}\n";
    expect(roundTrip(md)).toBe(md);
  });

  it("escapes quotes in attributes", () => {
    const md = '{{< pr-hero alt="Say \\"hi\\"" >}}\nText.\n{{< /pr-hero >}}\n';
    expect(roundTrip(md)).toBe(md);
  });

  it("loads into TipTap nodes and writes the same Markdown", () => {
    const md = `${hero}\n${cards}\n${button}`;
    const editor = editorFor(md);
    const types = editor.getJSON().content!.map((node) => node.type);
    expect(types).toEqual(["prHero", "prCards", "prButton"]);
    expect(editor.getJSON().content![0].attrs).toMatchObject({ image: "beach.jpg", alt: "A beach", button: "Read more", link: "/about/" });
    expect(editor.getJSON().content![1].content!.map((node) => node.type)).toEqual(["prCard", "prCard"]);
    expect(htmlToMarkdown(editor.getHTML())).toBe(md);
    editor.destroy();
  });

  it.each([
    ["unknown attributes", '{{< pr-hero color="red" >}}\nText.\n{{< /pr-hero >}}\n'],
    ["hand-written markup inside", '{{< pr-hero >}}\n<div class="x">raw</div>\n{{< /pr-hero >}}\n'],
    ["inline markup inside", "{{< pr-card >}}\nSome <b>bold</b>.\n{{< /pr-card >}}\n"],
    ["a hero inside a hero", "{{< pr-hero >}}\n{{< pr-hero >}}\nx\n{{< /pr-hero >}}\n{{< /pr-hero >}}\n"],
    ["text between cards", "{{< pr-cards >}}\nstray\n{{< pr-card >}}\nx\n{{< /pr-card >}}\n{{< /pr-cards >}}\n"],
    ["a card outside cards content", "{{< pr-cards >}}\n{{< pr-hero >}}\nx\n{{< /pr-hero >}}\n{{< /pr-cards >}}\n"],
    ["a multiline button", "{{< pr-button >}}a\nb{{< /pr-button >}}\n"],
    ["percent delimiters", "{{% pr-hero %}}\nText.\n{{% /pr-hero %}}\n"],
    ["a self-closing block", '{{< pr-button link="/" />}}\n'],
    ["an unclosed block", "{{< pr-hero >}}\n\nText.\n"],
  ])("keeps %s as a locked block with its text unchanged", (_label, md) => {
    expect(markdownToHtml(md)).not.toContain("data-pr=");
    expect(markdownToHtml(md)).toContain("data-shortcode");
    expect(roundTrip(md)).toBe(md);
  });

  it("locks the whole outer block and does not extract blocks inside it", () => {
    const md = `{{< pr-hero color="red" >}}\n${button.trim()}\n{{< /pr-hero >}}\n`;
    expect(markdownToHtml(md)).not.toContain("data-pr=");
    expect(roundTrip(md)).toBe(md);
  });

  it("does not touch block shortcodes inside code", () => {
    const md = "```\n{{< pr-button >}}x{{< /pr-button >}}\n```\n";
    expect(markdownToHtml(md)).not.toContain("data-pr=");
    expect(roundTrip(md)).toBe(md);
  });

  it("leaves other shortcodes as they were", () => {
    const md = `{{< notice >}}\nHi\n{{< /notice >}}\n\n${button}\n{{< youtube abc >}}\n`;
    expect(markdownToHtml(md).match(/data-shortcode=/g)).toHaveLength(2);
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
