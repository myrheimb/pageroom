import { Editor } from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import { describe, expect, it, vi } from "vitest";
import { htmlToMarkdown, markdownToHtml } from "./markdown";
import { ButtonBlock, CardBlock, CardsBlock, HeroBlock, ImageBlock, RawHtmlBlock, RawHtmlInline, ShortcodeBlock, ShortcodeInline } from "./nodes";
import { slashItems } from "./slash";
import { t } from "../i18n";

function setup(markdown = "") {
  const addPageBlock = vi.fn(async () => true);
  const editor = new Editor({
    extensions: [StarterKit, ImageBlock, ShortcodeBlock, ShortcodeInline, RawHtmlBlock, RawHtmlInline, HeroBlock, CardsBlock, CardBlock, ButtonBlock],
    content: markdownToHtml(markdown),
  });
  const items = slashItems({ pickImage: async () => null, prompt: async () => null, addContactForm: async () => false, addPageBlock });
  return { editor, addPageBlock, markdown: () => htmlToMarkdown(editor.getHTML()), insert: (title: string) => items.find((item) => item.title === title)!.run(editor, { from: 1, to: 1 }) };
}

const find = (editor: Editor, type: string) => {
  let found = { pos: -1, size: 0 };
  editor.state.doc.forEach((node, offset) => {
    if (node.type.name === type) found = { pos: offset, size: node.nodeSize };
  });
  return found;
};

describe("hero on the canvas", () => {
  it("inserts, edits and writes the hero", async () => {
    const { editor, addPageBlock, markdown, insert } = setup();
    await insert(t.slash.hero);
    expect(addPageBlock).toHaveBeenCalledWith("hero");
    expect(editor.getJSON().content![0].type).toBe("prHero");

    editor.commands.insertContentAt(2, "Welcome");
    const hero = find(editor, "prHero");
    editor.commands.command(({ tr }) => {
      tr.setNodeMarkup(hero.pos, undefined, { image: "beach.jpg", alt: "A beach", button: "Read more", link: "/about/" });
      return true;
    });
    expect(markdown()).toBe('{{< pr-hero image="beach.jpg" alt="A beach" button="Read more" link="/about/" >}}\n## Welcome\n{{< /pr-hero >}}\n');
    editor.destroy();
  });

  it("keeps the hero whole when a block is added inside it", async () => {
    const { editor, insert } = setup('{{< pr-hero >}}\n## Hi\n{{< /pr-hero >}}\n');
    editor.commands.setTextSelection(3);
    await insert(t.slash.button);
    expect(editor.getJSON().content!.map((node) => node.type).slice(0, 2)).toEqual(["prHero", "prButton"]);
    expect(editor.getJSON().content![0].content!.map((node) => node.type)).toEqual(["heading"]);
    editor.destroy();
  });
});

describe("button on the canvas", () => {
  it("inserts, edits and writes the button", async () => {
    const { editor, addPageBlock, markdown, insert } = setup();
    await insert(t.slash.button);
    expect(addPageBlock).toHaveBeenCalledWith("button");
    expect(markdown()).toBe(`{{< pr-button >}}${t.slash.buttonTextPlaceholder}{{< /pr-button >}}\n`);

    editor.commands.selectAll();
    editor.commands.insertContent({ type: "prButton", attrs: { link: "/contact/" }, content: [{ type: "text", text: "Write to us" }] });
    expect(markdown()).toBe('{{< pr-button link="/contact/" >}}Write to us{{< /pr-button >}}\n');
    editor.destroy();
  });
});

describe("moving blocks", () => {
  it("moves a hero and a button past other content", () => {
    const md = '{{< pr-hero >}}\n## Hi\n{{< /pr-hero >}}\n\nMiddle.\n\n{{< pr-button link="/" >}}Go{{< /pr-button >}}\n';
    const { editor, markdown } = setup(md);
    const move = (type: string, to: "start" | "end") => {
      const block = find(editor, type);
      editor.commands.command(({ tr, state }) => {
        const node = state.doc.nodeAt(block.pos)!;
        tr.delete(block.pos, block.pos + block.size);
        tr.insert(to === "start" ? 0 : tr.doc.content.size, node);
        return true;
      });
    };
    move("prButton", "start");
    move("prHero", "end");
    expect(markdown()).toBe('{{< pr-button link="/" >}}Go{{< /pr-button >}}\n\nMiddle.\n\n{{< pr-hero >}}\n## Hi\n{{< /pr-hero >}}\n');
    editor.destroy();
  });
});
