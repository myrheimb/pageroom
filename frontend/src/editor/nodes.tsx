import { Extension, mergeAttributes, Node } from "@tiptap/core";
import { NodeSelection, Plugin } from "@tiptap/pm/state";
import type { EditorView } from "@tiptap/pm/view";
import { NodeViewContent, NodeViewWrapper, ReactNodeViewRenderer, useEditorState, type NodeViewProps } from "@tiptap/react";
import { useState } from "react";
import { AlertTriangle, AlignCenter, AlignLeft, AlignRight, ArrowLeft, ArrowRight, Code2, GripVertical, Image as ImageIcon, Plus, Puzzle, Scissors, Trash2, RefreshCw, SquarePlay } from "lucide-react";
import { b64, describeShortcode, unb64 } from "./markdown";
import { t } from "../i18n";

export interface ImageOptions {
  /** Turn a Markdown image reference into a URL the editor can display. */
  resolve: (src: string) => string | null;
  /** Ask the user to pick a replacement image. It resolves to a reference. */
  pick: () => Promise<{ ref: string } | null>;
}

export interface ImageBlockOptions extends ImageOptions {
  /** Make sure the site can publish an aligned picture. */
  addAlignTemplate: () => Promise<boolean>;
}

const keepSelection = (event: React.MouseEvent) => event.preventDefault();

export function AlignControl({ value, onChange }: { value: string | null; onChange: (value: string) => void }) {
  const options = [
    { option: "", Icon: AlignLeft, label: t.editor.alignLeft },
    { option: "center", Icon: AlignCenter, label: t.editor.alignCenter },
    { option: "right", Icon: AlignRight, label: t.editor.alignRight },
  ];
  return (
    <div className="block-align" contentEditable={false} role="group" aria-label={t.editor.alignment}>
      {options.map(({ option, Icon, label }) => (
        <button key={option} type="button" className={"btn sm ghost" + ((value ?? "") === option ? " on" : "")} aria-pressed={(value ?? "") === option} title={label} aria-label={label} onMouseDown={keepSelection} onClick={() => onChange(option)}>
          <Icon size={13} />
        </button>
      ))}
    </div>
  );
}

export const BlockAlign = Extension.create({
  name: "blockAlign",
  addGlobalAttributes() {
    return [{
      types: ["paragraph", "heading", "image"],
      attributes: {
        align: {
          default: null,
          parseHTML: (element: HTMLElement) => element.getAttribute("data-align"),
          renderHTML: (attributes: Record<string, string | null>) => (attributes.align ? { "data-align": attributes.align, style: `text-align: ${attributes.align}` } : {}),
        },
      },
    }];
  },
});

export const ImageBlock = Node.create<ImageBlockOptions>({
  name: "image",
  group: "block",
  atom: true,
  draggable: true,
  addOptions() {
    return { resolve: (s: string) => s, pick: async () => null, addAlignTemplate: async () => true };
  },
  addAttributes() {
    return { src: { default: "" }, alt: { default: "" }, title: { default: null } };
  },
  parseHTML() {
    return [{ tag: "img[src]", getAttrs: (el) => ({ src: (el as HTMLElement).getAttribute("src"), alt: (el as HTMLElement).getAttribute("alt") ?? "", title: (el as HTMLElement).getAttribute("title") }) }];
  },
  renderHTML({ HTMLAttributes }) {
    return ["img", mergeAttributes(HTMLAttributes)];
  },
  addNodeView() {
    return ReactNodeViewRenderer(ImageView);
  },
});

function ImageView({ node, updateAttributes, deleteNode, extension, selected }: NodeViewProps) {
  const opts = extension.options as ImageBlockOptions;
  const url = opts.resolve(node.attrs.src);
  const [broken, setBroken] = useState(false);
  const missingAlt = !node.attrs.alt?.trim();
  return (
    <NodeViewWrapper className="img-block" data-drag-handle data-align={node.attrs.align ?? undefined}>
      {url && !broken ? (
        <img src={url} alt={node.attrs.alt} onError={() => setBroken(true)} draggable={false} />
      ) : (
        <div className="img-missing">
          <ImageIcon size={22} /> <div>{t.editor.imageNotFound(node.attrs.src)}</div>
        </div>
      )}
      {(selected || missingAlt) && (
        <div className="alt-bar" contentEditable={false}>
          <input
            value={node.attrs.alt ?? ""}
            placeholder={t.editor.altPlaceholder}
            onChange={(e) => updateAttributes({ alt: e.target.value })}
            onKeyDown={(e) => e.stopPropagation()}
          />
          {missingAlt && (
            <span className="alt-warn" title={t.editor.altWhy}>
              <AlertTriangle size={13} /> {t.editor.addDescription}
            </span>
          )}
          {selected && (
            <>
              <AlignControl value={node.attrs.align} onChange={async (align) => { if (!align || (await opts.addAlignTemplate())) updateAttributes({ align: align || null }); }} />
              <button className="btn sm ghost" onClick={async () => { const r = await opts.pick(); if (r) { setBroken(false); updateAttributes({ src: r.ref }); } }}>
                <RefreshCw size={13} /> {t.common.replace}
              </button>
              <button className="btn sm ghost danger" onClick={() => deleteNode()}>
                <Trash2 size={13} /> {t.common.remove}
              </button>
            </>
          )}
        </div>
      )}
    </NodeViewWrapper>
  );
}


function atomNode(name: string, attr: string, inline: boolean, view?: (p: NodeViewProps) => React.ReactElement) {
  return Node.create({
    name,
    group: inline ? "inline" : "block",
    inline,
    atom: true,
    selectable: true,
    draggable: !inline,
    addAttributes() {
      return { raw: { default: "" } };
    },
    parseHTML() {
      const tag = inline ? "span" : "div";
      return [{ tag: `${tag}[${attr}]`, getAttrs: (el) => ({ raw: unb64((el as HTMLElement).getAttribute(attr) || "") }) }];
    },
    renderHTML({ node }) {
      return [inline ? "span" : "div", { [attr]: b64(node.attrs.raw) }, node.attrs.raw];
    },
    addNodeView: view ? () => ReactNodeViewRenderer(view) : undefined,
  });
}

function RawEditor({ node, updateAttributes }: NodeViewProps) {
  const [editing, setEditing] = useState(false);
  return editing ? (
    <textarea
      className="textarea mono"
      autoFocus
      value={node.attrs.raw}
      rows={Math.min(12, node.attrs.raw.split("\n").length + 1)}
      onChange={(e) => updateAttributes({ raw: e.target.value })}
      onBlur={() => setEditing(false)}
      onKeyDown={(e) => e.stopPropagation()}
    />
  ) : (
    <pre onDoubleClick={() => setEditing(true)} title={t.editor.editCode}>{node.attrs.raw}</pre>
  );
}

function ShortcodeView(props: NodeViewProps) {
  const d = describeShortcode(props.node.attrs.raw);
  const Icon = d.name === "youtube" ? SquarePlay : Puzzle;
  return (
    <NodeViewWrapper className="atom-block" data-drag-handle contentEditable={false}>
      <div className="ab-icon"><Icon size={16} /></div>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div className="ab-title">{d.label}</div>
        <div className="dim small">{t.editor.shortcodeHelp}</div>
        <RawEditor {...props} />
      </div>
    </NodeViewWrapper>
  );
}

function RawHtmlView(props: NodeViewProps) {
  const raw: string = props.node.attrs.raw;
  if (/^<!--\s*more\s*-->$/i.test(raw.trim())) {
    return (
      <NodeViewWrapper className="atom-block" data-drag-handle contentEditable={false} style={{ alignItems: "center" }}>
        <div className="ab-icon"><Scissors size={16} /></div>
        <div>
          <div className="ab-title">{t.editor.summaryBreak}</div>
          <div className="dim small">{t.editor.summaryBreakHelp}</div>
        </div>
      </NodeViewWrapper>
    );
  }
  return (
    <NodeViewWrapper className="atom-block" data-drag-handle contentEditable={false}>
      <div className="ab-icon"><Code2 size={16} /></div>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div className="ab-title">{t.editor.html}</div>
        <RawEditor {...props} />
      </div>
    </NodeViewWrapper>
  );
}

function InlineChip({ node }: NodeViewProps) {
  const raw: string = node.attrs.raw;
  const label = raw.startsWith("{{") ? describeShortcode(raw).label : raw.length > 24 ? raw.slice(0, 24) + "…" : raw;
  return (
    <NodeViewWrapper as="span" className="atom-inline" title={raw}>
      {label}
    </NodeViewWrapper>
  );
}

export const ShortcodeBlock = atomNode("shortcodeBlock", "data-shortcode", false, ShortcodeView);
export const ShortcodeInline = atomNode("shortcodeInline", "data-shortcode-inline", true, InlineChip);
export const RawHtmlBlock = atomNode("rawHtmlBlock", "data-raw-html", false, RawHtmlView);
export const RawHtmlInline = atomNode("rawHtmlInline", "data-raw-inline", true, InlineChip);


function pageBlock<Options = unknown>(name: string, kind: string, content: string, attributeNames: string[], group = "block", view?: (props: NodeViewProps) => React.ReactElement, options?: Options) {
  return Node.create<Options>({
    name,
    group,
    content,
    defining: true,
    draggable: true,
    addOptions() {
      return options as Options;
    },
    addAttributes() {
      return Object.fromEntries(attributeNames.map((attribute) => [attribute, { default: "", parseHTML: (el: HTMLElement) => el.getAttribute(`data-${attribute}`) || "", renderHTML: (attrs: Record<string, string>) => ({ [`data-${attribute}`]: attrs[attribute] }) }]));
    },
    parseHTML() {
      return [{ tag: `div[data-pr="${kind}"]` }];
    },
    renderHTML({ HTMLAttributes }) {
      return ["div", mergeAttributes({ "data-pr": kind }, HTMLAttributes), 0];
    },
    addNodeView: view ? () => ReactNodeViewRenderer(view) : undefined,
  });
}

export interface HeroOptions extends ImageOptions {
  /** Ask for a description of a newly chosen picture. */
  describe: () => Promise<string | null>;
}

function HeroView({ node, updateAttributes, extension }: NodeViewProps) {
  const opts = extension.options as HeroOptions;
  const url = node.attrs.image ? opts.resolve(node.attrs.image) : null;
  const stop = (e: React.KeyboardEvent) => e.stopPropagation();
  const choose = async () => {
    const picked = await opts.pick();
    if (!picked) return;
    const alt = (picked as { alt?: string }).alt || (await opts.describe()) || "";
    updateAttributes({ image: picked.ref, alt });
  };
  return (
    <NodeViewWrapper className="pr-hero">
      <div className="pr-hero-text" style={{ textAlign: node.attrs.align || undefined }}>
        <NodeViewContent className="pr-hero-content" />
        <div className="pr-hero-button" contentEditable={false}>
          <input value={node.attrs.button} placeholder={t.editor.heroButtonText} onChange={(e) => updateAttributes({ button: e.target.value })} onKeyDown={stop} />
          <input value={node.attrs.link} placeholder={t.editor.linkAddress} onChange={(e) => updateAttributes({ link: e.target.value })} onKeyDown={stop} />
        </div>
        <AlignControl value={node.attrs.align} onChange={(align) => updateAttributes({ align })} />
      </div>
      <div className="pr-hero-media" contentEditable={false}>
        <button type="button" className="pr-hero-picture" onClick={choose} title={url ? t.editor.heroChangePicture : t.editor.heroAddPicture}>
          {url ? <img src={url} alt={node.attrs.alt} draggable={false} /> : <><ImageIcon size={22} /> <span>{t.editor.heroAddPicture}</span></>}
        </button>
        {url && (
          <button type="button" className="btn sm ghost danger" onClick={() => updateAttributes({ image: "", alt: "" })}>
            <Trash2 size={13} /> {t.common.remove}
          </button>
        )}
      </div>
    </NodeViewWrapper>
  );
}

function ButtonView({ node, updateAttributes }: NodeViewProps) {
  return (
    <NodeViewWrapper className="pr-button-block">
      <div className="pr-button-row" style={{ textAlign: node.attrs.align || undefined }}>
        <div className="pr-button-face"><NodeViewContent className="pr-button-text" /></div>
      </div>
      <div className="pr-button-link" contentEditable={false}>
        <input value={node.attrs.link} placeholder={t.editor.linkAddress} onChange={(e) => updateAttributes({ link: e.target.value })} onKeyDown={(e) => e.stopPropagation()} />
        <AlignControl value={node.attrs.align} onChange={(align) => updateAttributes({ align })} />
      </div>
    </NodeViewWrapper>
  );
}

function CardsView({ node, editor, getPos, updateAttributes }: NodeViewProps) {
  const addCard = () => {
    const pos = getPos();
    if (pos === undefined) return;
    editor.chain().focus().insertContentAt(pos + node.nodeSize - 1, { type: "prCard", content: [{ type: "heading", attrs: { level: 3 } }, { type: "paragraph" }] }).run();
  };
  return (
    <NodeViewWrapper className="pr-cards" style={{ textAlign: node.attrs.align || undefined }}>
      <NodeViewContent className="pr-cards-content" />
      <div className="pr-cards-tools" contentEditable={false}>
        <button type="button" className="btn sm ghost" onMouseDown={keepSelection} onClick={addCard}>
          <Plus size={13} /> {t.editor.cardAdd}
        </button>
        <AlignControl value={node.attrs.align} onChange={(align) => updateAttributes({ align })} />
      </div>
    </NodeViewWrapper>
  );
}

function CardView({ editor, getPos, node }: NodeViewProps) {
  const locate = () => {
    const pos = getPos();
    if (pos === undefined || pos > editor.state.doc.content.size) return null;
    const $pos = editor.state.doc.resolve(pos);
    return { pos, index: $pos.index(), siblings: $pos.parent.childCount, parentStart: pos - $pos.parentOffset };
  };
  const remove = () => {
    const at = locate();
    if (at && at.siblings > 1) editor.chain().focus().deleteRange({ from: at.pos, to: at.pos + node.nodeSize }).run();
  };
  const move = (step: -1 | 1) => {
    const at = locate();
    if (!at) return;
    const $pos = editor.state.doc.resolve(at.pos);
    const sibling = $pos.parent.maybeChild(at.index + step);
    if (!sibling) return;
    const from = step < 0 ? at.pos - sibling.nodeSize : at.pos;
    const to = step < 0 ? at.pos + node.nodeSize : at.pos + node.nodeSize + sibling.nodeSize;
    const swapped = step < 0 ? [node, sibling] : [sibling, node];
    editor.chain().focus().command(({ tr }) => {
      tr.replaceWith(from, to, swapped);
      return true;
    }).run();
  };
  // A card's view does not re-render when a sibling moves, so its position has to be watched.
  const place = useEditorState({
    editor,
    selector: () => {
      const at = locate();
      return at && { first: at.index === 0, last: at.index === at.siblings - 1, only: at.siblings < 2 };
    },
  });
  return (
    <NodeViewWrapper className="pr-card">
      <div className="pr-card-tools" contentEditable={false}>
        <span className="pr-card-grip" draggable data-drag-handle title={t.editor.dragToMove}><GripVertical size={14} /></span>
        <button type="button" className="btn sm ghost pr-card-step" disabled={!place || place.first} title={t.editor.cardMoveEarlier} aria-label={t.editor.cardMoveEarlier} onMouseDown={keepSelection} onClick={() => move(-1)}><ArrowLeft size={13} /></button>
        <button type="button" className="btn sm ghost pr-card-step" disabled={!place || place.last} title={t.editor.cardMoveLater} aria-label={t.editor.cardMoveLater} onMouseDown={keepSelection} onClick={() => move(1)}><ArrowRight size={13} /></button>
        <button type="button" className="btn sm ghost danger" disabled={!place || place.only} title={t.editor.cardRemove} aria-label={t.editor.cardRemove} onMouseDown={keepSelection} onClick={remove}><Trash2 size={13} /></button>
      </div>
      <NodeViewContent className="pr-card-content" />
    </NodeViewWrapper>
  );
}

type Box = Pick<DOMRect, "left" | "right" | "top" | "bottom">;

export function cardDropGap(cards: Box[], x: number, y: number) {
  const distance = (card: Box) => Math.hypot(x - Math.min(Math.max(x, card.left), card.right), y - Math.min(Math.max(y, card.top), card.bottom));
  const nearest = cards.reduce((best, card, index) => (distance(card) < distance(cards[best]) ? index : best), 0);
  const card = cards[nearest];
  const inRow = cards.some((other) => other !== card && other.top < card.bottom && other.bottom > card.top);
  const after = inRow ? x > (card.left + card.right) / 2 : y > (card.top + card.bottom) / 2;
  return { index: nearest + (after ? 1 : 0), card, inRow, after };
}

// ProseMirror's drop line is always horizontal, so in a row of cards it cuts through a card.
function cardDropPlugin() {
  const halfGap = 7;
  let bar: HTMLElement | null = null;
  const hide = () => {
    bar?.remove();
    bar = null;
    document.documentElement.removeAttribute("data-card-drop");
  };
  const findDrop = (view: EditorView, event: DragEvent) => {
    const selection = (view.dragging as { node?: NodeSelection } | null)?.node ?? view.state.selection;
    if (!(selection instanceof NodeSelection) || selection.node.type.name !== "prCard") return null;
    const list = (event.target as Element).closest?.(".pr-cards")?.querySelector("[data-node-view-content-react]");
    if (!list) return null;
    const $start = view.state.doc.resolve(view.posAtDOM(list, 0));
    if ($start.pos !== selection.$from.start()) return null;
    const gap = cardDropGap(Array.from(list.children, (card) => card.getBoundingClientRect()), event.clientX, event.clientY);
    return { ...gap, selection, pos: $start.posAtIndex(gap.index) };
  };
  const show = ({ card, inRow, after }: ReturnType<typeof cardDropGap>) => {
    bar ??= document.body.appendChild(Object.assign(document.createElement("div"), { className: "card-drop-bar" }));
    const edge = (inRow ? (after ? card.right : card.left) : after ? card.bottom : card.top) + (after ? halfGap : -halfGap);
    Object.assign(bar.style, inRow
      ? { left: `${edge - 1.5}px`, top: `${card.top}px`, width: "3px", height: `${card.bottom - card.top}px` }
      : { left: `${card.left}px`, top: `${edge - 1.5}px`, width: `${card.right - card.left}px`, height: "3px" });
    document.documentElement.setAttribute("data-card-drop", "");
  };
  return new Plugin({
    view: () => ({ destroy: hide }),
    props: {
      handleDOMEvents: {
        dragover(view, event) {
          const drop = findDrop(view, event);
          if (drop) show(drop);
          else hide();
          return false;
        },
        dragleave(view, event) {
          if (!view.dom.contains(event.relatedTarget as globalThis.Node | null)) hide();
          return false;
        },
        dragend() {
          hide();
          return false;
        },
        drop(view, event) {
          const drop = findDrop(view, event);
          hide();
          if (!drop) return false;

          event.preventDefault();
          view.dragging = null;
          const { from, to, node } = drop.selection;
          if (drop.pos !== from && drop.pos !== to) {
            const tr = view.state.tr.delete(from, to);
            view.dispatch(tr.insert(tr.mapping.map(drop.pos), node));
          }
          return true;
        },
      },
    },
  });
}

export const HeroBlock = pageBlock<HeroOptions>("prHero", "hero", "block+", ["image", "alt", "button", "link", "align"], "block", HeroView, { resolve: (s: string) => s, pick: async () => null, describe: async () => null });
export const CardsBlock = pageBlock("prCards", "cards", "prCard+", ["align"], "block", CardsView).extend({
  addProseMirrorPlugins() {
    return [cardDropPlugin()];
  },
});
export const CardBlock = pageBlock("prCard", "card", "block+", [], "prCardItem", CardView);
export const ButtonBlock = pageBlock("prButton", "button", "text*", ["link", "align"], "block", ButtonView);
