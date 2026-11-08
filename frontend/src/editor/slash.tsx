import { Extension, type Editor, type Range } from "@tiptap/core";
import Suggestion, { type SuggestionProps } from "@tiptap/suggestion";
import { ReactRenderer } from "@tiptap/react";
import { forwardRef, useEffect, useImperativeHandle, useState } from "react";
import {
  CheckSquare, Code, Heading1, Heading2, Heading3, Image, List, ListOrdered, Mail, Minus, Puzzle, Quote, Scissors, Table, Type, SquarePlay, MousePointerClick, PanelTop,
} from "lucide-react";
import { t } from "../i18n";

export interface SlashItem {
  title: string;
  description: string;
  group: string;
  keywords?: string[];
  icon: React.ReactNode;
  run: (editor: Editor, range: Range) => void;
}

const PAGE_BLOCKS = ["prHero", "prCards", "prCard", "prButton"];

export interface SlashHooks {
  pickImage: () => Promise<{ ref: string; alt?: string } | null>;
  prompt: (title: string, label: string, placeholder?: string) => Promise<string | null>;
  addContactForm: () => Promise<boolean>;
  addPageBlock: (kind: "button" | "cards" | "hero") => Promise<boolean>;
}

export function slashItems(h: SlashHooks): SlashItem[] {
  const block = (fn: (e: Editor) => void) => (e: Editor, r: Range) => {
    e.chain().focus().deleteRange(r).run();
    fn(e);
  };
  const pageBlock = (kind: "button" | "hero", content: () => object) => async (e: Editor, r: Range) => {
    e.chain().focus().deleteRange(r).run();
    if (!(await h.addPageBlock(kind))) return;
    const { $from } = e.state.selection;
    let depth = $from.depth;
    while (depth > 0 && !PAGE_BLOCKS.includes($from.node(depth).type.name)) depth--;
    const chain = e.chain().focus();
    if (depth > 0) chain.insertContentAt($from.after(depth), content()).run();
    else chain.insertContent(content()).run();
  };
  return [
    { group: t.slash.basic, title: t.slash.text, description: t.slash.textHint, icon: <Type size={16} />, run: block((e) => e.chain().setParagraph().run()) },
    { group: t.slash.basic, title: t.slash.heading, description: t.slash.headingHint, keywords: ["h2", "title"], icon: <Heading1 size={16} />, run: block((e) => e.chain().setHeading({ level: 2 }).run()) },
    { group: t.slash.basic, title: t.slash.subheading, description: t.slash.subheadingHint, keywords: ["h3"], icon: <Heading2 size={16} />, run: block((e) => e.chain().setHeading({ level: 3 }).run()) },
    { group: t.slash.basic, title: t.slash.smallHeading, description: t.slash.smallHeadingHint, keywords: ["h4"], icon: <Heading3 size={16} />, run: block((e) => e.chain().setHeading({ level: 4 }).run()) },
    { group: t.slash.basic, title: t.slash.bulletedList, description: t.slash.bulletedListHint, keywords: ["ul", "bullet"], icon: <List size={16} />, run: block((e) => e.chain().toggleBulletList().run()) },
    { group: t.slash.basic, title: t.slash.numberedList, description: t.slash.numberedListHint, keywords: ["ol", "number"], icon: <ListOrdered size={16} />, run: block((e) => e.chain().toggleOrderedList().run()) },
    { group: t.slash.basic, title: t.slash.checklist, description: t.slash.checklistHint, keywords: ["todo", "task"], icon: <CheckSquare size={16} />, run: block((e) => e.chain().toggleTaskList().run()) },
    { group: t.slash.basic, title: t.slash.quote, description: t.slash.quoteHint, keywords: ["blockquote"], icon: <Quote size={16} />, run: block((e) => e.chain().toggleBlockquote().run()) },
    { group: t.slash.basic, title: t.slash.divider, description: t.slash.dividerHint, keywords: ["hr", "line", "separator"], icon: <Minus size={16} />, run: block((e) => e.chain().setHorizontalRule().run()) },
    {
      group: t.slash.sections, title: t.slash.hero, description: t.slash.heroHint, keywords: ["banner", "intro", "header"], icon: <PanelTop size={16} />,
      run: pageBlock("hero", () => ({ type: "prHero", content: [{ type: "heading", attrs: { level: 2 } }, { type: "paragraph" }] })),
    },
    {
      group: t.slash.sections, title: t.slash.button, description: t.slash.buttonHint, keywords: ["cta", "link"], icon: <MousePointerClick size={16} />,
      run: pageBlock("button", () => ({ type: "prButton", content: [{ type: "text", text: t.slash.buttonTextPlaceholder }] })),
    },
    {
      group: t.slash.media, title: t.slash.image, description: t.slash.imageHint, keywords: ["photo", "picture", "img"], icon: <Image size={16} />,
      run: async (e, r) => {
        e.chain().focus().deleteRange(r).run();
        const img = await h.pickImage();
        if (img) e.chain().focus().insertContent({ type: "image", attrs: { src: img.ref, alt: img.alt ?? "" } }).run();
      },
    },
    {
      group: t.slash.media, title: t.slash.youtube, description: t.slash.youtubeHint, keywords: ["video", "embed"], icon: <SquarePlay size={16} />,
      run: async (e, r) => {
        e.chain().focus().deleteRange(r).run();
        const url = await h.prompt(t.slash.youtubeTitle, t.slash.youtubeLabel, "https://www.youtube.com/watch?v=…");
        const id = url && (/(?:v=|youtu\.be\/|embed\/|shorts\/)([\w-]{6,})/.exec(url)?.[1] ?? (/^[\w-]{6,}$/.test(url) ? url : null));
        if (id) e.chain().focus().insertContent({ type: "shortcodeBlock", attrs: { raw: `{{< youtube ${id} >}}` } }).run();
      },
    },
    {
      group: t.slash.media, title: t.slash.contactForm, description: t.slash.contactFormHint, keywords: ["form", "email", "contact"], icon: <Mail size={16} />,
      run: async (e, r) => {
        e.chain().focus().deleteRange(r).run();
        if (await h.addContactForm()) e.chain().focus().insertContent({ type: "shortcodeBlock", attrs: { raw: "{{< contact-form >}}" } }).run();
      },
    },
    { group: t.slash.advanced, title: t.slash.table, description: t.slash.tableHint, icon: <Table size={16} />, run: block((e) => e.chain().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run()) },
    { group: t.slash.advanced, title: t.slash.code, description: t.slash.codeHint, keywords: ["pre"], icon: <Code size={16} />, run: block((e) => e.chain().toggleCodeBlock().run()) },
    {
      group: t.slash.advanced, title: t.slash.summaryBreak, description: t.slash.summaryBreakHint, keywords: ["more", "excerpt"], icon: <Scissors size={16} />,
      run: block((e) => e.chain().insertContent({ type: "rawHtmlBlock", attrs: { raw: "<!--more-->" } }).run()),
    },
    {
      group: t.slash.advanced, title: t.slash.shortcode, description: t.slash.shortcodeHint, keywords: ["hugo", "embed"], icon: <Puzzle size={16} />,
      run: async (e, r) => {
        e.chain().focus().deleteRange(r).run();
        const code = await h.prompt(t.slash.shortcodeTitle, t.slash.shortcode, '{{< figure src="photo.jpg" caption="…" >}}');
        if (code) e.chain().focus().insertContent({ type: "shortcodeBlock", attrs: { raw: code } }).run();
      },
    },
  ];
}

export function filterItems(items: SlashItem[], q: string): SlashItem[] {
  const s = q.toLowerCase().trim();
  if (!s) return items;
  return items.filter((i) => i.title.toLowerCase().includes(s) || i.keywords?.some((k) => k.includes(s)) || i.description.toLowerCase().includes(s));
}

interface ListRef { onKeyDown: (e: KeyboardEvent) => boolean }

const SlashList = forwardRef<ListRef, SuggestionProps<SlashItem>>((props, ref) => {
  const [sel, setSel] = useState(0);
  useEffect(() => setSel(0), [props.items]);
  const choose = (i: number) => {
    const item = props.items[i];
    if (item) props.command(item);
  };
  useImperativeHandle(ref, () => ({
    onKeyDown: (e) => {
      if (e.key === "ArrowDown") { setSel((s) => (s + 1) % Math.max(1, props.items.length)); return true; }
      if (e.key === "ArrowUp") { setSel((s) => (s - 1 + props.items.length) % Math.max(1, props.items.length)); return true; }
      if (e.key === "Enter") { choose(sel); return true; }
      return false;
    },
  }));
  if (!props.items.length) return <div className="slash"><div className="s-desc" style={{ padding: 10 }}>{t.slash.noMatch}</div></div>;
  let lastGroup = "";
  return (
    <div className="slash" role="listbox">
      {props.items.map((item, i) => {
        const head = item.group !== lastGroup ? <div className="s-group">{item.group}</div> : null;
        lastGroup = item.group;
        return (
          <div key={item.title}>
            {head}
            <div className={"s-item" + (i === sel ? " on" : "")} role="option" aria-selected={i === sel} onMouseEnter={() => setSel(i)} onMouseDown={(e) => { e.preventDefault(); choose(i); }}>
              <div className="s-icon">{item.icon}</div>
              <div>
                <div className="s-name">{item.title}</div>
                <div className="s-desc">{item.description}</div>
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
});

export const SlashCommand = Extension.create<{ hooks: SlashHooks }>({
  name: "slashCommand",
  addOptions() {
    return { hooks: { pickImage: async () => null, prompt: async () => null, addContactForm: async () => false, addPageBlock: async () => false } };
  },
  addProseMirrorPlugins() {
    const all = slashItems(this.options.hooks);
    return [
      Suggestion<SlashItem>({
        editor: this.editor,
        char: "/",
        startOfLine: false,
        allowSpaces: false,
        items: ({ query }) => filterItems(all, query),
        command: ({ editor, range, props }) => props.run(editor, range),
        render: () => {
          let comp: ReactRenderer<ListRef, SuggestionProps<SlashItem>> | null = null;
          let el: HTMLDivElement | null = null;
          const place = (p: SuggestionProps<SlashItem>) => {
            const rect = p.clientRect?.();
            if (!rect || !el) return;
            const below = window.innerHeight - rect.bottom > 380;
            el.style.left = `${Math.min(rect.left, window.innerWidth - 320)}px`;
            el.style.top = below ? `${rect.bottom + 6}px` : `${rect.top - 6}px`;
            el.style.transform = below ? "" : "translateY(-100%)";
          };
          return {
            onStart: (p) => {
              comp = new ReactRenderer(SlashList, { props: p, editor: p.editor });
              el = document.createElement("div");
              el.style.position = "fixed";
              el.style.zIndex = "70";
              el.appendChild(comp.element);
              document.body.appendChild(el);
              place(p);
            },
            onUpdate: (p) => { comp?.updateProps(p); place(p); },
            onKeyDown: (p) => {
              if (p.event.key === "Escape") { el?.remove(); return true; }
              return comp?.ref?.onKeyDown(p.event) ?? false;
            },
            onExit: () => { el?.remove(); comp?.destroy(); el = null; comp = null; },
          };
        },
      }),
    ];
  },
});
