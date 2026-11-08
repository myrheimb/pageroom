import { Editor, type JSONContent } from "@tiptap/core";
import StarterKit from "@tiptap/starter-kit";
import { EditorContent } from "@tiptap/react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it, vi } from "vitest";
import { htmlToMarkdown, markdownToHtml } from "./markdown";
import { ButtonBlock, CardBlock, CardsBlock, HeroBlock, ImageBlock, RawHtmlBlock, RawHtmlInline, ShortcodeBlock, ShortcodeInline } from "./nodes";
import { slashItems } from "./slash";
import { t } from "../i18n";

// TipTap scrolls to the selection one frame after focus, and jsdom can't measure text for that scroll.
vi.stubGlobal("requestAnimationFrame", () => 0);

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

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

async function mount(markdown: string) {
  const editor = new Editor({
    extensions: [StarterKit, ImageBlock, ShortcodeBlock, ShortcodeInline, RawHtmlBlock, RawHtmlInline, HeroBlock, CardsBlock, CardBlock, ButtonBlock],
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
  const press = async (button: Element | null | undefined) => {
    await act(async () => {
      button!.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, cancelable: true }));
    });
  };
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

  it("keeps the last card", async () => {
    const { editor, host } = await mount('{{< pr-cards >}}\n{{< pr-card >}}\nOnly\n{{< /pr-card >}}\n{{< /pr-cards >}}\n');
    await press(tools(host, t.editor.cardRemove)[0]);
    expect(host.querySelectorAll(".pr-card")).toHaveLength(1);
    editor.destroy();
  });
});
