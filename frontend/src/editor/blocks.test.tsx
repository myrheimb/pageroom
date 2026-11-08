import { Editor, type JSONContent } from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import { EditorContent } from "@tiptap/react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";
import { htmlToMarkdown, markdownToHtml } from "./markdown";
import { BlockAlign, ButtonBlock, cardDropGap, CardBlock, CardsBlock, HeroBlock, ImageBlock, RawHtmlBlock, RawHtmlInline, ShortcodeBlock, ShortcodeInline } from "./nodes";
import { slashItems } from "./slash";
import { t } from "../i18n";

// TipTap scrolls to the selection one frame after focus, and jsdom can't measure text for that scroll.
vi.stubGlobal("requestAnimationFrame", () => 0);

function setup(markdown = "") {
  const addPageBlock = vi.fn(async () => true);
  const editor = new Editor({
    extensions: [StarterKit, BlockAlign, ImageBlock, ShortcodeBlock, ShortcodeInline, RawHtmlBlock, RawHtmlInline, HeroBlock, CardsBlock, CardBlock, ButtonBlock],
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

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

async function mount(markdown: string) {
  const editor = new Editor({
    extensions: [StarterKit, BlockAlign, ImageBlock, ShortcodeBlock, ShortcodeInline, RawHtmlBlock, RawHtmlInline, HeroBlock, CardsBlock, CardBlock, ButtonBlock],
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
