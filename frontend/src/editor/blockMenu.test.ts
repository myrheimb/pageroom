import { Editor } from "@tiptap/core";
import type { Node as PMNode } from "@tiptap/pm/model";
import { TaskItem, TaskList } from "@tiptap/extension-list";
import { TableKit } from "@tiptap/extension-table";
import StarterKit from "@tiptap/starter-kit";
import { describe, expect, it, vi } from "vitest";
import { htmlToMarkdown, markdownToHtml } from "./markdown";
import { BlockAlign, ButtonBlock, CardBlock, CardsBlock, ColumnBlock, ColumnsBlock, HeroBlock, ImageBlock, RawHtmlBlock, RawHtmlInline, ShortcodeBlock, ShortcodeInline, VideoBlock } from "./nodes";
import { deleteBlock, turnInto, turnIntoTargets, type Target } from "./blockMenu";

// TipTap scrolls to the selection one frame after focus, and jsdom can't measure text for that scroll.
vi.stubGlobal("requestAnimationFrame", () => 0);

function setup(markdown: string) {
  return new Editor({
    extensions: [StarterKit, TaskList, TaskItem, TableKit, BlockAlign, ImageBlock, ShortcodeBlock, ShortcodeInline, RawHtmlBlock, RawHtmlInline, HeroBlock, CardsBlock, CardBlock, ColumnsBlock, ColumnBlock, ButtonBlock, VideoBlock],
    content: markdownToHtml(markdown),
  });
}

async function convert(markdown: string, target: Target, templateAdded = true) {
  const editor = setup(markdown);
  const addPageBlock = vi.fn(async () => templateAdded);
  const moved = await turnInto(editor, 0, target, addPageBlock);
  const result = { markdown: htmlToMarkdown(editor.getHTML()), moved, templates: addPageBlock.mock.calls.map((call) => (call as unknown[])[0]) };
  editor.destroy();
  return result;
}

const cards = (...bodies: string[]) => `{{< pr-cards >}}\n${bodies.map((body) => `{{< pr-card >}}\n${body}\n{{< /pr-card >}}`).join("\n\n")}\n{{< /pr-cards >}}\n`;
const columns = (...bodies: string[]) => `{{< pr-columns >}}\n${bodies.map((body) => `{{< pr-column >}}\n${body}\n{{< /pr-column >}}`).join("\n\n")}\n{{< /pr-columns >}}\n`;

describe("turning a block into another", () => {
  it.each<[string, string, Target, string]>([
    ["text into a heading", "Hello\n", "heading2", "## Hello\n"],
    ["a heading into a bulleted list", "### Hello\n", "bulletList", "- Hello\n"],
    ["a bulleted list into text", "- One\n- Two\n", "paragraph", "One\n\nTwo\n"],
    ["a numbered list into a checklist", "1. One\n2. Two\n", "taskList", "- [ ] One\n- [ ] Two\n"],
    ["a quote into a small heading", "> Hello\n", "heading4", "#### Hello\n"],
  ])("turns %s", async (_, markdown, target, expected) => {
    const result = await convert(markdown, target);
    expect(result.markdown).toBe(expected);
    expect(result.templates).toEqual([]);
  });

  it.each<[string, string, Target, string, string]>([
    ["text into a banner", "Hello\n", "prHero", "{{< pr-hero >}}\nHello\n{{< /pr-hero >}}\n", "hero"],
    ["a heading into a card", "### Hello\n", "prCards", cards("### Hello"), "cards"],
  ])("turns %s and adds the site's template for it", async (_, markdown, target, expected, template) => {
    const result = await convert(markdown, target);
    expect(result.markdown).toBe(expected);
    expect(result.templates).toEqual([template]);
  });

  it("keeps a banner's picture and button when it turns into text", async () => {
    const result = await convert('{{< pr-hero image="beach.jpg" alt="A beach" button="Read more" link="/about/" >}}\n## Welcome\n{{< /pr-hero >}}\n', "paragraph");
    expect(result.markdown).toBe('![A beach](beach.jpg)\n\n## Welcome\n\n{{< pr-button link="/about/" >}}Read more{{< /pr-button >}}\n');
    expect(result.templates).toEqual(["button"]);
  });

  it.each([
    ["cards", cards("### One\n\nFirst.", "### Two")],
    ["columns", columns("### One\n\nFirst.", "### Two")],
  ])("turns %s into text in reading order", async (_, markdown) => {
    expect((await convert(markdown, "paragraph")).markdown).toBe("### One\n\nFirst.\n\n### Two\n");
  });

  it("turns cards into columns and moves what a column can't hold after them", async () => {
    const result = await convert(cards("One.\n\n---", "Two."), "prColumns");
    expect(result.markdown).toBe(`${columns("One.", "Two.")}\n---\n`);
    expect(result.moved).toBe(1);
    expect(result.templates).toEqual(["columns"]);
  });

  it("turns columns into cards and moves what a card can't hold after them", async () => {
    const result = await convert(columns("One.", 'Two.\n\n{{< pr-button link="/go/" >}}Go{{< /pr-button >}}'), "prCards");
    expect(result.markdown).toBe(`${cards("One.", "Two.")}\n{{< pr-button link="/go/" >}}Go{{< /pr-button >}}\n`);
    expect(result.moved).toBe(1);
  });

  it("changes nothing when the site's template can't be added", async () => {
    const result = await convert("Hello\n", "prHero", false);
    expect(result.markdown).toBe("Hello\n");
    expect(result.moved).toBeNull();
  });
});

describe("what a block can turn into", () => {
  const targetsOf = (markdown: string, pick = (doc: PMNode): [PMNode, PMNode] => [doc.firstChild!, doc]) => {
    const editor = setup(markdown);
    const found = turnIntoTargets(...pick(editor.state.doc));
    editor.destroy();
    return found;
  };
  const firstInColumn = (doc: PMNode): [PMNode, PMNode] => {
    const column = doc.firstChild!.firstChild!;
    return [column.firstChild!, column];
  };

  it("offers every other text style plus banner and cards for top-level text", () => {
    expect(targetsOf("## Hello\n")).toEqual(["paragraph", "heading3", "heading4", "bulletList", "orderedList", "taskList", "blockquote", "codeBlock", "prHero", "prCards"]);
  });

  it("offers only text styles inside a column, which can't hold sections", () => {
    expect(targetsOf(columns("One.", "Two."), firstInColumn)).toEqual(["heading2", "heading3", "heading4", "bulletList", "orderedList", "taskList", "blockquote", "codeBlock"]);
  });

  it("offers columns only for two or three cards", () => {
    expect(targetsOf(cards("One.", "Two."))).toEqual(["paragraph", "prColumns"]);
    expect(targetsOf(cards("One.", "Two.", "Three.", "Four."))).toEqual(["paragraph"]);
  });

  it("offers nothing for an image", () => {
    expect(targetsOf("![A beach](beach.jpg)\n", (doc) => [doc.lastChild!, doc])).toEqual([]);
  });
});

describe("deleting a block", () => {
  const remove = (markdown: string, pick: (doc: PMNode) => number) => {
    const editor = setup(markdown);
    deleteBlock(editor, pick(editor.state.doc));
    const result = htmlToMarkdown(editor.getHTML());
    editor.destroy();
    return result;
  };

  it("deletes a block that has no other way out, such as a video", () => {
    expect(remove('Before.\n\n{{< pr-video id="abc123" >}}\n\nAfter.\n', (doc) => doc.child(0).nodeSize)).toBe("Before.\n\nAfter.\n");
  });

  it("leaves a blank line in a column whose only block is deleted, so the column stays valid", () => {
    const markdown = columns('{{< pr-button link="/go/" >}}Go{{< /pr-button >}}', "Two.");
    expect(remove(markdown, () => 2)).toBe(columns("", "Two.").replace("{{< pr-column >}}\n\n", "{{< pr-column >}}\n"));
  });
});
