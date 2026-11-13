import { Editor, type JSONContent } from "@tiptap/core";
import { TaskItem, TaskList } from "@tiptap/extension-list";
import { TableKit } from "@tiptap/extension-table";
import StarterKit from "@tiptap/starter-kit";
import { describe, expect, it } from "vitest";
import { htmlToMarkdown, liftShortcodes, markdownToHtml, stripUnsafeUrls } from "./markdown";
import { BlockAlign, ButtonBlock, CardBlock, CardsBlock, ColumnBlock, ColumnsBlock, HeroBlock, ImageBlock, RawHtmlBlock, RawHtmlInline, ShortcodeBlock, ShortcodeInline, VideoBlock } from "./nodes";

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

const columns = '{{< pr-columns >}}\n{{< pr-column >}}\nSome **text**.\n\n![Sea](sea.jpg)\n{{< /pr-column >}}\n\n{{< pr-column >}}\n```go\nfmt.Println("hi")\n```\n\n{{< pr-button link="/go/" align="right" >}}Go{{< /pr-button >}}\n{{< /pr-column >}}\n{{< /pr-columns >}}\n';
const columnsWith = (attributes: string, count: number) =>
  `{{< pr-columns${attributes} >}}\n${Array.from({ length: count }, (_, index) => `{{< pr-column >}}\nColumn ${index}.\n{{< /pr-column >}}`).join("\n\n")}\n{{< /pr-columns >}}\n`;

function editorFor(markdown: string) {
  return new Editor({
    extensions: [StarterKit, TaskList, TaskItem, TableKit, BlockAlign, ImageBlock, ShortcodeBlock, ShortcodeInline, RawHtmlBlock, RawHtmlInline, HeroBlock, CardsBlock, CardBlock, ColumnsBlock, ColumnBlock, ButtonBlock, VideoBlock],
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

  it("round trips columns holding text, a picture, a code block and a button", () => {
    expect(roundTrip(columns)).toBe(columns);
    expect(markdownToHtml(columns).match(/data-pr="column"/g)).toHaveLength(2);
    expect(markdownToHtml(columns)).toContain('data-align="right"');
  });

  it("loads columns into TipTap nodes with a button that keeps its alignment", () => {
    const editor = editorFor(columns);
    const [block] = editor.getJSON().content!;
    expect(block.type).toBe("prColumns");
    expect(block.content!.map((column) => column.type)).toEqual(["prColumn", "prColumn"]);
    const [left, right] = (block.content as JSONContent[]).map((column) => column.content as JSONContent[]);
    expect(left.filter((node) => node.type !== "paragraph" || node.content).map((node) => node.type)).toEqual(["paragraph", "image"]);
    expect(right.map((node) => node.type)).toEqual(["codeBlock", "prButton"]);
    expect(right[1].attrs).toMatchObject({ link: "/go/", align: "right" });
    expect(htmlToMarkdown(editor.getHTML())).toBe(columns);
    editor.destroy();
  });

  it.each([
    [' layout="2-1"', 2],
    [' layout="1-2"', 2],
    [' layout="1-1-1"', 3],
    [' valign="center"', 2],
    [' layout="1-1-1" valign="bottom"', 3],
  ])("round trips columns with%s", (attributes, count) => {
    const md = columnsWith(attributes, count);
    expect(markdownToHtml(md)).toContain('data-pr="columns"');
    expect(roundTrip(md)).toBe(md);
  });

  it.each(['{{< pr-video vimeo="76979871" >}}\n', '{{< pr-video src="knead.mp4" >}}\n', '{{< pr-video src="https://example.com/a/knead.mp4" >}}\n'])("round trips a Vimeo video or a video file: %s", (video) => {
    expect(markdownToHtml(video)).toContain('data-pr="video"');
    expect(roundTrip(video)).toBe(video);

    const editor = editorFor(video);
    expect(editor.getJSON().content!.map((node) => node.type)).toEqual(["prVideo"]);
    editor.destroy();
  });

  it("round trips a video on the page and inside a column", () => {
    const video = '{{< pr-video id="abc_123" >}}\n';
    const inColumn = `{{< pr-columns >}}\n{{< pr-column >}}\nText.\n\n${video}{{< /pr-column >}}\n\n{{< pr-column >}}\nB\n{{< /pr-column >}}\n{{< /pr-columns >}}\n`;
    expect(roundTrip(video)).toBe(video);
    expect(roundTrip(inColumn)).toBe(inColumn);

    const editor = editorFor(inColumn);
    expect(((editor.getJSON().content![0].content as JSONContent[])[0].content as JSONContent[]).map((node) => node.type)).toEqual(["paragraph", "prVideo"]);
    expect(htmlToMarkdown(editor.getHTML())).toBe(inColumn);
    editor.destroy();
  });

  const alignedInColumn = (body: string, to = "center") => `{{< pr-columns >}}\n{{< pr-column >}}\n{{< pr-align to="${to}" >}}\n${body}\n{{< /pr-align >}}\n{{< /pr-column >}}\n\n{{< pr-column >}}\nB\n{{< /pr-column >}}\n{{< /pr-columns >}}\n`;

  it.each([
    ["a paragraph", "Text."],
    ["a heading", "## Title"],
    ["a quote", "> Said once."],
    ["a quote with two paragraphs", "> One.\n> \n> Two."],
    ["a quote with a nested quote", "> Outer.\n> \n> > Inner."],
    ["a picture", "![Sea](sea.jpg)"],
  ])("round trips %s aligned inside a column", (_label, body) => {
    const md = alignedInColumn(body);
    expect(markdownToHtml(md)).toContain('data-pr="columns"');
    expect(markdownToHtml(md)).toContain('data-align="center"');
    expect(roundTrip(md)).toBe(md);

    const editor = editorFor(md);
    expect(htmlToMarkdown(editor.getHTML())).toBe(md);
    editor.destroy();
  });

  it("loads an aligned quote with a nested quote as an editable quote on the page", () => {
    const md = '{{< pr-align to="right" >}}\n> Outer.\n> \n> > Inner.\n{{< /pr-align >}}\n';
    expect(markdownToHtml(md)).toContain('<blockquote data-align="right">');
    expect(markdownToHtml(md)).not.toContain('data-shortcode');
    expect(roundTrip(md)).toBe(md);

    const editor = editorFor(md);
    expect(editor.state.doc.firstChild?.type.name).toBe("blockquote");
    expect(htmlToMarkdown(editor.getHTML())).toBe(md);
    editor.destroy();
  });

  it("round trips an aligned quote on the page", () => {
    const md = '{{< pr-align to="right" >}}\n> Said once.\n{{< /pr-align >}}\n';
    expect(roundTrip(md)).toBe(md);
  });

  it("keeps empty columns", () => {
    const md = "{{< pr-columns >}}\n{{< pr-column >}}\n{{< /pr-column >}}\n\n{{< pr-column >}}\n{{< /pr-column >}}\n{{< /pr-columns >}}\n";
    expect(roundTrip(md)).toBe(md);
    expect(markdownToHtml(md)).toContain('data-pr="columns"');
  });

  it("only writes alignment for top-level content", () => {
    expect(htmlToMarkdown('<div data-pr="hero"><p data-align="center">Text.</p></div>')).toBe("{{< pr-hero >}}\nText.\n{{< /pr-hero >}}\n");
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
    ["an unknown alignment", '{{< pr-align to="middle" >}}\nText.\n{{< /pr-align >}}\n'],
    ["two aligned paragraphs", '{{< pr-align to="center" >}}\nOne.\n\nTwo.\n{{< /pr-align >}}\n'],
    ["one column", "{{< pr-columns >}}\n{{< pr-column >}}\nOne.\n{{< /pr-column >}}\n{{< /pr-columns >}}\n"],
    ["three columns", "{{< pr-columns >}}\n{{< pr-column >}}\nA\n{{< /pr-column >}}\n{{< pr-column >}}\nB\n{{< /pr-column >}}\n{{< pr-column >}}\nC\n{{< /pr-column >}}\n{{< /pr-columns >}}\n"],
    ["a three-column layout with two columns", columnsWith(' layout="1-1-1"', 2)],
    ["a two-column layout with three columns", columnsWith(' layout="2-1"', 3)],
    ["an unknown column layout", columnsWith(' layout="3-1"', 2)],
    ["an unknown vertical position", columnsWith(' valign="middle"', 2)],
    ["a video without an id", "{{< pr-video >}}\n"],
    ["a video with an unsafe id", '{{< pr-video id="a/b" >}}\n'],
    ["a video with an unknown attribute", '{{< pr-video id="abc" start="10" >}}\n'],
    ["a video with two sources", '{{< pr-video id="abc" vimeo="123" >}}\n'],
    ["a Vimeo video with a non-numeric id", '{{< pr-video vimeo="abc" >}}\n'],
    ["a video file with an empty src", '{{< pr-video src="" >}}\n'],
    ["a self-closing video", '{{< pr-video id="abc" />}}\n'],
    ["a banner inside a column", "{{< pr-columns >}}\n{{< pr-column >}}\n{{< pr-hero >}}\nx\n{{< /pr-hero >}}\n{{< /pr-column >}}\n\n{{< pr-column >}}\nB\n{{< /pr-column >}}\n{{< /pr-columns >}}\n"],
    ["cards inside a column", "{{< pr-columns >}}\n{{< pr-column >}}\nA\n{{< /pr-column >}}\n\n{{< pr-column >}}\n{{< pr-cards >}}\n{{< pr-card >}}\nx\n{{< /pr-card >}}\n{{< /pr-cards >}}\n{{< /pr-column >}}\n{{< /pr-columns >}}\n"],
    ["columns inside a column", "{{< pr-columns >}}\n{{< pr-column >}}\n{{< pr-columns >}}\n{{< pr-column >}}\nx\n{{< /pr-column >}}\n{{< /pr-columns >}}\n{{< /pr-column >}}\n\n{{< pr-column >}}\nB\n{{< /pr-column >}}\n{{< /pr-columns >}}\n"],
    ["an aligned list inside a column", '{{< pr-columns >}}\n{{< pr-column >}}\n{{< pr-align to="center" >}}\n- x\n{{< /pr-align >}}\n{{< /pr-column >}}\n\n{{< pr-column >}}\nB\n{{< /pr-column >}}\n{{< /pr-columns >}}\n'],
    ["another shortcode inside a column", "{{< pr-columns >}}\n{{< pr-column >}}\n{{< youtube abc >}}\n{{< /pr-column >}}\n\n{{< pr-column >}}\nB\n{{< /pr-column >}}\n{{< /pr-columns >}}\n"],
    ["a divider inside a column", "{{< pr-columns >}}\n{{< pr-column >}}\nA\n\n---\n\nB\n{{< /pr-column >}}\n\n{{< pr-column >}}\nB\n{{< /pr-column >}}\n{{< /pr-columns >}}\n"],
    ["raw HTML inside a column", "{{< pr-columns >}}\n{{< pr-column >}}\n<b>x</b>\n\n<div>y</div>\n{{< /pr-column >}}\n\n{{< pr-column >}}\nB\n{{< /pr-column >}}\n{{< /pr-columns >}}\n"],
    ["text between columns", "{{< pr-columns >}}\nstray\n{{< pr-column >}}\nA\n{{< /pr-column >}}\n\n{{< pr-column >}}\nB\n{{< /pr-column >}}\n{{< /pr-columns >}}\n"],
    ["a column outside columns", "{{< pr-column >}}\nx\n{{< /pr-column >}}\n"],
    ["an aligned block inside a banner", '{{< pr-hero >}}\n{{< pr-align to="center" >}}\nText.\n{{< /pr-align >}}\n{{< /pr-hero >}}\n'],
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
