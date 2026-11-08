import { mergeAttributes, Node } from "@tiptap/core";
import { NodeViewWrapper, ReactNodeViewRenderer, type NodeViewProps } from "@tiptap/react";
import { useState } from "react";
import { AlertTriangle, Code2, Image as ImageIcon, Puzzle, Scissors, Trash2, RefreshCw, SquarePlay } from "lucide-react";
import { b64, describeShortcode, unb64 } from "./markdown";
import { t } from "../i18n";

export interface ImageOptions {
  /** Turn a Markdown image reference into a URL the editor can display. */
  resolve: (src: string) => string | null;
  /** Ask the user to pick a replacement image. It resolves to a reference. */
  pick: () => Promise<{ ref: string } | null>;
}


export const ImageBlock = Node.create<ImageOptions>({
  name: "image",
  group: "block",
  atom: true,
  draggable: true,
  addOptions() {
    return { resolve: (s: string) => s, pick: async () => null };
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
  const opts = extension.options as ImageOptions;
  const url = opts.resolve(node.attrs.src);
  const [broken, setBroken] = useState(false);
  const missingAlt = !node.attrs.alt?.trim();
  return (
    <NodeViewWrapper className="img-block" data-drag-handle>
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
