import { Editor, type JSONContent } from "@tiptap/core";
import { TaskItem, TaskList } from "@tiptap/extension-list";
import { TableKit } from "@tiptap/extension-table";
import StarterKit from "@tiptap/starter-kit";
import { EditorContent } from "@tiptap/react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";
import { htmlToMarkdown, markdownToHtml } from "./markdown";
import { alignableBlock, BlockAlign, ButtonBlock, cardDropGap, CardBlock, CardsBlock, ColumnBlock, ColumnsBlock, HeroBlock, ImageBlock, RawHtmlBlock, RawHtmlInline, setBlockAlign, ShortcodeBlock, ShortcodeInline, VideoBlock } from "./nodes";
import { availableItems, slashItems } from "./slash";
import { t } from "../i18n";

// TipTap scrolls to the selection one frame after focus, and jsdom can't measure text for that scroll.
vi.stubGlobal("requestAnimationFrame", () => 0);

function setup(markdown = "", prompt: () => Promise<string | null> = async () => null) {
  const addPageBlock = vi.fn(async () => true);
  const editor = new Editor({
    extensions: [StarterKit, TaskList, TaskItem, TableKit, BlockAlign, ImageBlock, ShortcodeBlock, ShortcodeInline, RawHtmlBlock, RawHtmlInline, HeroBlock, CardsBlock, CardBlock, ColumnsBlock, ColumnBlock, ButtonBlock, VideoBlock],
    content: markdownToHtml(markdown),
  });
  const items = slashItems({ pickImage: async () => null, prompt, addContactForm: async () => false, addPageBlock });
  return { editor, addPageBlock, menu: (query = "") => availableItems(items, editor, query).map((item) => item.title), markdown: () => htmlToMarkdown(editor.getHTML()), insert: (title: string) => items.find((item) => item.title === title)!.run(editor, { from: 1, to: 1 }) };
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

describe("summary break in the slash menu", () => {
  const cursorIn = (editor: Editor, text: string) => {
    let found = -1;
    editor.state.doc.descendants((node, pos) => {
      if (node.isText && node.text === text) found = pos + 1;
    });
    editor.commands.setTextSelection(found);
  };

  it("is offered in a plain paragraph", () => {
    const { editor, menu } = setup("Plain.\n");
    cursorIn(editor, "Plain.");
    expect(menu()).toContain(t.slash.summaryBreak);
    editor.destroy();
  });

  it.each([
    ["a banner", "{{< pr-hero >}}\n## Hi\n{{< /pr-hero >}}\n", "Hi"],
    ["a card", "{{< pr-cards >}}\n{{< pr-card >}}\n### One\n{{< /pr-card >}}\n{{< /pr-cards >}}\n", "One"],
  ])("is left out inside %s", (_label, md, text) => {
    const { editor, menu } = setup(md);
    cursorIn(editor, text);
    expect(menu()).not.toContain(t.slash.summaryBreak);
    expect(menu("more")).toEqual([]);
    expect(menu()).toContain(t.slash.divider);
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

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

async function mount(markdown: string) {
  const editor = new Editor({
    extensions: [StarterKit, TaskList, TaskItem, TableKit, BlockAlign, ImageBlock, ShortcodeBlock, ShortcodeInline, RawHtmlBlock, RawHtmlInline, HeroBlock, CardsBlock, CardBlock, ColumnsBlock, ColumnBlock, ButtonBlock, VideoBlock],
    content: markdownToHtml(markdown),
  });
  const host = document.createElement("div");
  document.body.appendChild(host);
  await act(async () => createRoot(host).render(<EditorContent editor={editor} />));
  return { editor, host, markdown: () => htmlToMarkdown(editor.getHTML()) };
}

async function typeInto(input: HTMLInputElement, value: string) {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

const press = async (button: Element | null | undefined) => {
  await act(async () => {
    button!.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, cancelable: true }));
    (button as HTMLElement).click();
  });
};

const inputWith = (host: HTMLElement, placeholder: string) => host.querySelector<HTMLInputElement>(`input[placeholder="${placeholder}"]`)!;

describe("columns on the canvas", () => {
  const columns = '{{< pr-columns >}}\n{{< pr-column >}}\nLeft.\n{{< /pr-column >}}\n\n{{< pr-column >}}\nRight.\n\n{{< pr-button link="/go/" >}}Go{{< /pr-button >}}\n{{< /pr-column >}}\n{{< /pr-columns >}}\n';
  const swapped = '{{< pr-columns >}}\n{{< pr-column >}}\nRight.\n\n{{< pr-button link="/go/" >}}Go{{< /pr-button >}}\n{{< /pr-column >}}\n\n{{< pr-column >}}\nLeft.\n{{< /pr-column >}}\n{{< /pr-columns >}}\n';
  const cursorIn = (editor: Editor, text: string) => {
    let found = -1;
    editor.state.doc.descendants((node, pos) => {
      if (node.isText && node.text === text) found = pos + 1;
    });
    editor.commands.setTextSelection(found);
  };
  const topLevel = (editor: Editor) => editor.getJSON().content!.map((node) => node.type).filter((type) => type !== "paragraph");
  const buttonWith = (host: HTMLElement, text: string) => Array.from(host.querySelectorAll("button")).find((button) => button.textContent?.trim() === text);
  const threeColumns = '{{< pr-columns layout="1-1-1" >}}\n{{< pr-column >}}\nA.\n{{< /pr-column >}}\n\n{{< pr-column >}}\nB.\n{{< /pr-column >}}\n\n{{< pr-column >}}\nC.\n{{< /pr-column >}}\n{{< /pr-columns >}}\n';
  const blankColumn = "{{< pr-column >}}\n{{< /pr-column >}}";
  const blankBesideRight = `{{< pr-columns >}}\n${blankColumn}\n\n{{< pr-column >}}\nRight.\n{{< /pr-column >}}\n{{< /pr-columns >}}\n`;

  it("adds a third column and removes it again when it is blank", async () => {
    const { editor, host, markdown } = await mount(columns);
    await press(buttonWith(host, "1:1:1"));
    expect(host.querySelectorAll(".pr-column")).toHaveLength(3);
    expect(markdown()).toContain(`layout="1-1-1"`);
    expect(markdown()).toContain(blankColumn);
    expect(buttonWith(host, t.editor.columnsSwap)).toBeUndefined();

    await press(buttonWith(host, "1:1"));
    expect(markdown()).toBe(columns);
    editor.destroy();
  });

  it("moves the third column's content into the second when going to two columns", async () => {
    const { editor, host, markdown } = await mount(threeColumns);
    await press(buttonWith(host, "2:1"));
    expect(markdown()).toBe('{{< pr-columns layout="2-1" >}}\n{{< pr-column >}}\nA.\n{{< /pr-column >}}\n\n{{< pr-column >}}\nB.\n\nC.\n{{< /pr-column >}}\n{{< /pr-columns >}}\n');
    editor.destroy();
  });

  it("flips the widths along with the content when swapping", async () => {
    const { editor, host, markdown } = await mount(columns.replace("{{< pr-columns >}}", '{{< pr-columns layout="2-1" >}}'));
    await press(buttonWith(host, t.editor.columnsSwap));
    expect(markdown()).toBe(swapped.replace("{{< pr-columns >}}", '{{< pr-columns layout="1-2" >}}'));
    editor.destroy();
  });

  it("sets the vertical position with the visible control", async () => {
    const { editor, host, markdown } = await mount(columns);
    await press(host.querySelector(`button[aria-label="${t.editor.alignMiddle}"]`));
    expect(markdown()).toBe(columns.replace("{{< pr-columns >}}", '{{< pr-columns valign="center" >}}'));
    editor.destroy();
  });

  it("removes a button with the visible control and keeps the column valid", async () => {
    const { editor, host, markdown } = await mount(blankBesideRight.replace(blankColumn, '{{< pr-column >}}\n{{< pr-button link="/go/" >}}Go{{< /pr-button >}}\n{{< /pr-column >}}'));
    await press(buttonWith(host, t.common.remove));
    expect(markdown()).toBe(blankBesideRight);
    expect(editor.state.doc.check()).toBeUndefined();
    editor.destroy();
  });

  it("replaces a blank paragraph when a button is chosen in a column", async () => {
    const { editor, insert } = setup(blankBesideRight);
    editor.commands.setTextSelection(3);
    await insert(t.slash.button);
    expect((editor.getJSON().content![0].content as JSONContent[])[0].content!.map((node) => node.type)).toEqual(["prButton"]);
    editor.destroy();
  });

  describe("alignment", () => {
    const alignmentTarget = (editor: Editor) => alignableBlock(editor.state.selection.$from)?.type.name ?? null;
    const quoted = '{{< pr-columns >}}\n{{< pr-column >}}\n> Said.\n{{< /pr-column >}}\n\n{{< pr-column >}}\n- Item\n{{< /pr-column >}}\n{{< /pr-columns >}}\n';

    it("aligns a paragraph inside a column", () => {
      const { editor, markdown } = setup(columns);
      cursorIn(editor, "Left.");
      expect(alignmentTarget(editor)).toBe("paragraph");
      setBlockAlign(editor, "center");
      expect(markdown()).toContain('{{< pr-column >}}\n{{< pr-align to="center" >}}\nLeft.\n{{< /pr-align >}}\n{{< /pr-column >}}');
      setBlockAlign(editor, "");
      expect(markdown()).toBe(columns);
      editor.destroy();
    });

    it("aligns the whole quote from inside it", () => {
      const { editor, markdown } = setup(quoted);
      cursorIn(editor, "Said.");
      expect(alignmentTarget(editor)).toBe("blockquote");
      setBlockAlign(editor, "right");
      expect(markdown()).toContain('{{< pr-align to="right" >}}\n> Said.\n{{< /pr-align >}}');
      editor.destroy();
    });

    it("offers no alignment in a list or a banner", () => {
      const list = setup(quoted);
      cursorIn(list.editor, "Item");
      expect(alignmentTarget(list.editor)).toBeNull();
      list.editor.destroy();

      const banner = setup("{{< pr-hero >}}\nText.\n{{< /pr-hero >}}\n");
      cursorIn(banner.editor, "Text.");
      expect(alignmentTarget(banner.editor)).toBeNull();
      banner.editor.destroy();
    });

    it("aligns a picture inside a column with its visible control", async () => {
      const picture = "{{< pr-columns >}}\n{{< pr-column >}}\n![Sea](sea.jpg)\n{{< /pr-column >}}\n\n{{< pr-column >}}\nB\n{{< /pr-column >}}\n{{< /pr-columns >}}\n";
      const { editor, host, markdown } = await mount(picture);
      let imagePosition = -1;
      editor.state.doc.descendants((node, pos) => {
        if (node.type.name === "image") imagePosition = pos;
      });
      await act(async () => editor.commands.setNodeSelection(imagePosition));
      await press(host.querySelector(`button[aria-label="${t.editor.alignCenter}"]`));
      expect(markdown()).toContain('{{< pr-align to="center" >}}\n![Sea](sea.jpg)\n{{< /pr-align >}}');
      editor.destroy();
    });
  });

  describe("videos", () => {
    const videoLink = async () => "https://www.youtube.com/watch?v=abc123";
    const video = '{{< pr-video id="abc123" >}}';
    const leftColumnTypes = (editor: Editor) => (editor.getJSON().content![0].content as JSONContent[])[0].content!.map((node) => node.type);

    it("inserts a video on the page from the menu", async () => {
      const { editor, addPageBlock, markdown, insert } = setup("", videoLink);
      await insert(t.slash.youtube);
      expect(addPageBlock).toHaveBeenCalledWith("video");
      expect(markdown()).toBe(`${video}\n`);
      editor.destroy();
    });

    it("puts a video after the text it is chosen in, inside the column", async () => {
      const { editor, insert } = setup(columns, videoLink);
      cursorIn(editor, "Left.");
      await insert(t.slash.youtube);
      expect(topLevel(editor)).toEqual(["prColumns"]);
      expect(leftColumnTypes(editor)).toEqual(["paragraph", "prVideo"]);
      editor.destroy();
    });

    it("goes after a banner it is chosen in", async () => {
      const { editor, insert } = setup("{{< pr-hero >}}\nText.\n{{< /pr-hero >}}\n", videoLink);
      cursorIn(editor, "Text.");
      await insert(t.slash.youtube);
      expect(topLevel(editor)).toEqual(["prHero", "prVideo"]);
      editor.destroy();
    });

    it("replaces a blank paragraph when chosen in a column", async () => {
      const { editor, insert } = setup(blankBesideRight, videoLink);
      editor.commands.setTextSelection(3);
      await insert(t.slash.youtube);
      expect(leftColumnTypes(editor)).toEqual(["prVideo"]);
      editor.destroy();
    });

    it("removes a video with the visible control and keeps the column valid", async () => {
      const { editor, host, markdown } = await mount(blankBesideRight.replace(blankColumn, `{{< pr-column >}}\n${video}\n{{< /pr-column >}}`));
      await press(buttonWith(host, t.common.remove));
      expect(markdown()).toBe(blankBesideRight);
      expect(editor.state.doc.check()).toBeUndefined();
      editor.destroy();
    });
  });

  it("inserts two columns from the menu", async () => {
    const { editor, addPageBlock, insert } = setup();
    await insert(t.slash.columns);
    expect(addPageBlock).toHaveBeenCalledWith("columns");
    expect(editor.getJSON().content![0].content!.map((node) => node.type)).toEqual(["prColumn", "prColumn"]);
    editor.destroy();
  });

  it("swaps the sides with the visible button", async () => {
    const { editor, host, markdown } = await mount(columns);
    expect(host.querySelectorAll(".pr-column")).toHaveLength(2);

    await press(Array.from(host.querySelectorAll("button")).find((button) => button.textContent?.includes(t.editor.columnsSwap)));
    expect(markdown()).toBe(swapped);

    await press(Array.from(host.querySelectorAll("button")).find((button) => button.textContent?.includes(t.editor.columnsSwap)));
    expect(markdown()).toBe(columns);
    editor.destroy();
  });

  it("puts a chosen button into the column and any other block after the columns", async () => {
    const { editor, markdown, insert } = setup(columns);
    cursorIn(editor, "Left.");
    await insert(t.slash.button);
    expect(topLevel(editor)).toEqual(["prColumns"]);
    expect(((editor.getJSON().content![0].content as JSONContent[])[0].content as JSONContent[]).map((node) => node.type)).toEqual(["paragraph", "prButton"]);

    for (const title of [t.slash.hero, t.slash.cards, t.slash.columns]) {
      cursorIn(editor, "Right.");
      await insert(title);
    }
    expect(topLevel(editor)).toEqual(["prColumns", "prColumns", "prCards", "prHero"]);
    for (const column of editor.getJSON().content![0].content as JSONContent[]) {
      expect((column.content as JSONContent[]).every((node) => ["paragraph", "prButton"].includes(node.type!))).toBe(true);
    }
    expect(markdown()).toContain("{{< pr-hero >}}");
    editor.destroy();
  });

  it("puts a block chosen inside a column's button after the columns", async () => {
    const { editor, insert } = setup(columns);
    cursorIn(editor, "Go");
    await insert(t.slash.hero);
    expect(topLevel(editor)).toEqual(["prColumns", "prHero"]);
    editor.destroy();
  });

  it("does not let a banner or card hold another page block or raw HTML", () => {
    const { editor } = setup(columns);
    for (const parent of ["prHero", "prCard"]) {
      for (const child of ["prButton", "prCards", "prColumns", "prHero", "prVideo", "rawHtmlBlock"]) expect(editor.schema.nodes[parent].contentMatch.matchType(editor.schema.nodes[child])).toBeNull();
      for (const child of ["heading", "image", "paragraph", "shortcodeBlock"]) expect(editor.schema.nodes[parent].contentMatch.matchType(editor.schema.nodes[child])).not.toBeNull();
    }
    editor.destroy();
  });

  it("does not let a column hold a banner, cards or columns", () => {
    const { editor } = setup(columns);
    const column = editor.schema.nodes.prColumn;
    for (const name of ["prHero", "prCards", "prColumns"]) expect(column.contentMatch.matchType(editor.schema.nodes[name])).toBeNull();
    expect(editor.schema.nodes.prColumns.contentMatch.matchType(column)).not.toBeNull();
    editor.destroy();
  });
});

describe("block inputs", () => {
  it("changes an existing button's text and link", async () => {
    const { editor, host, markdown } = await mount('{{< pr-button link="/old/" >}}Old{{< /pr-button >}}\n');
    expect(inputWith(host, t.editor.linkAddress).value).toBe("/old/");

    await typeInto(inputWith(host, t.editor.linkAddress), "/contact/");
    await act(async () => {
      editor.commands.setTextSelection({ from: 1, to: editor.state.doc.firstChild!.nodeSize - 1 });
      editor.commands.insertContent("Write to us");
    });
    expect(markdown()).toBe('{{< pr-button link="/contact/" >}}Write to us{{< /pr-button >}}\n');
    editor.destroy();
  });

  it("changes a hero's button text and link", async () => {
    const { editor, host, markdown } = await mount('{{< pr-hero button="Old" link="/old/" >}}\n## Hi\n{{< /pr-hero >}}\n');
    expect(inputWith(host, t.editor.heroButtonText).value).toBe("Old");

    await typeInto(inputWith(host, t.editor.heroButtonText), "Read more");
    await typeInto(inputWith(host, t.editor.linkAddress), "/about/");
    expect(markdown()).toBe('{{< pr-hero button="Read more" link="/about/" >}}\n## Hi\n{{< /pr-hero >}}\n');
    editor.destroy();
  });
});

describe("cards on the canvas", () => {
  const cards = '{{< pr-cards >}}\n{{< pr-card >}}\n### One\n\nFirst.\n{{< /pr-card >}}\n{{< pr-card >}}\n### Two\n\nSecond.\n{{< /pr-card >}}\n{{< /pr-cards >}}\n';
  const tools = (host: HTMLElement, label: string) => Array.from(host.querySelectorAll(`button[aria-label="${label}"]`));
  const cardTitles = (editor: Editor) => (editor.getJSON().content![0].content as JSONContent[]).map((card) => card.content![0].content?.[0].text);

  it("inserts three cards from the menu", async () => {
    const { editor, addPageBlock, insert } = setup();
    await insert(t.slash.cards);
    expect(addPageBlock).toHaveBeenCalledWith("cards");
    expect(editor.getJSON().content![0].content!.map((node) => node.type)).toEqual(["prCard", "prCard", "prCard"]);
    editor.destroy();
  });

  it("adds, edits, reorders and removes cards with the visible controls", async () => {
    const { editor, host, markdown } = await mount(cards);
    expect(host.querySelectorAll(".pr-card")).toHaveLength(2);

    await press(Array.from(host.querySelectorAll("button")).find((button) => button.textContent?.includes(t.editor.cardAdd)));
    expect(host.querySelectorAll(".pr-card")).toHaveLength(3);

    await act(async () => {
      editor.commands.insertContentAt(editor.state.doc.firstChild!.nodeSize - 3 - 2, "Three");
    });
    expect(cardTitles(editor)).toEqual(["One", "Two", "Three"]);

    await press(tools(host, t.editor.cardMoveLater)[0]);
    expect(cardTitles(editor).slice(0, 2)).toEqual(["Two", "One"]);

    await press(tools(host, t.editor.cardMoveEarlier)[1]);
    expect(cardTitles(editor).slice(0, 2)).toEqual(["One", "Two"]);

    await press(tools(host, t.editor.cardRemove)[2]);
    expect(host.querySelectorAll(".pr-card")).toHaveLength(2);
    expect(markdown()).toBe('{{< pr-cards >}}\n{{< pr-card >}}\n### One\n\nFirst.\n{{< /pr-card >}}\n\n{{< pr-card >}}\n### Two\n\nSecond.\n{{< /pr-card >}}\n{{< /pr-cards >}}\n');
    editor.destroy();
  });

  it("keeps every card's arrows correct as the cards change", async () => {
    const { editor, host } = await mount(cards);
    const enabled = (label: string) => tools(host, label).map((button) => !(button as HTMLButtonElement).disabled);
    expect(enabled(t.editor.cardMoveEarlier)).toEqual([false, true]);
    expect(enabled(t.editor.cardMoveLater)).toEqual([true, false]);

    await press(Array.from(host.querySelectorAll("button")).find((button) => button.textContent?.includes(t.editor.cardAdd)));
    expect(enabled(t.editor.cardMoveEarlier)).toEqual([false, true, true]);
    expect(enabled(t.editor.cardMoveLater)).toEqual([true, true, false]);

    await press(tools(host, t.editor.cardRemove)[0]);
    expect(enabled(t.editor.cardMoveEarlier)).toEqual([false, true]);
    expect(enabled(t.editor.cardMoveLater)).toEqual([true, false]);
    editor.destroy();
  });

  it("gives every card a drag grip", async () => {
    const { editor, host } = await mount(cards);
    expect(host.querySelectorAll('.pr-card [data-drag-handle][draggable="true"]')).toHaveLength(2);
    expect(editor.schema.nodes.prCard.spec.draggable).toBe(true);
    editor.destroy();
  });

  it("keeps the last card", async () => {
    const { editor, host } = await mount('{{< pr-cards >}}\n{{< pr-card >}}\nOnly\n{{< /pr-card >}}\n{{< /pr-cards >}}\n');
    await press(tools(host, t.editor.cardRemove)[0]);
    expect(host.querySelectorAll(".pr-card")).toHaveLength(1);
    editor.destroy();
  });
});

describe("dropping a card", () => {
  const box = (left: number, top: number) => ({ left, top, right: left + 100, bottom: top + 100 });
  const row = [box(0, 0), box(120, 0), box(240, 0)];
  const stack = [box(0, 0), box(0, 120), box(0, 240)];
  const grid = [box(0, 0), box(120, 0), box(0, 120), box(120, 120)];

  it.each([
    ["the left half of a card in a row", row, 130, 50, { index: 1, inRow: true }],
    ["the right half of a card in a row", row, 210, 50, { index: 2, inRow: true }],
    ["the gap between two cards in a row", row, 112, 50, { index: 1, inRow: true }],
    ["past the last card in a row", row, 400, 50, { index: 3, inRow: true }],
    ["the top half of a stacked card", stack, 50, 130, { index: 1, inRow: false }],
    ["the bottom half of a stacked card", stack, 50, 210, { index: 2, inRow: false }],
    ["the second row of a grid", grid, 210, 150, { index: 4, inRow: true }],
  ])("picks the right gap for %s", (_label, cards, x, y, expected) => {
    expect(cardDropGap(cards, x, y)).toMatchObject(expected);
  });
});

describe("alignment", () => {
  const alignButton = (host: HTMLElement, label: string) => host.querySelector(`.block-align button[aria-label="${label}"]`);

  it.each([
    ["button", '{{< pr-button link="/" >}}Go{{< /pr-button >}}\n', '{{< pr-button link="/" align="center" >}}Go{{< /pr-button >}}\n'],
    ["banner", "{{< pr-hero >}}\n## Hi\n{{< /pr-hero >}}\n", '{{< pr-hero align="center" >}}\n## Hi\n{{< /pr-hero >}}\n'],
    ["cards block", "{{< pr-cards >}}\n{{< pr-card >}}\nOne\n{{< /pr-card >}}\n{{< /pr-cards >}}\n", '{{< pr-cards align="center" >}}\n{{< pr-card >}}\nOne\n{{< /pr-card >}}\n{{< /pr-cards >}}\n'],
  ])("centers a %s and clears it again", async (_label, plain, centered) => {
    const { editor, host, markdown } = await mount(plain);
    await press(alignButton(host, t.editor.alignCenter));
    expect(markdown()).toBe(centered);
    await press(alignButton(host, t.editor.alignLeft));
    expect(markdown()).toBe(plain);
    editor.destroy();
  });

  it.each([
    ["paragraph", '{{< pr-align to="center" >}}\nSome **bold** text.\n{{< /pr-align >}}\n'],
    ["heading", '{{< pr-align to="right" >}}\n## Hello\n{{< /pr-align >}}\n'],
    ["picture", '{{< pr-align to="center" >}}\n![A beach](beach.jpg)\n{{< /pr-align >}}\n'],
  ])("keeps an aligned %s through the editor", (_label, source) => {
    const { editor, markdown } = setup(source);
    expect(markdown()).toBe(source);
    editor.destroy();
  });
});
