import { useEditor, useEditorState, EditorContent, type Editor } from "@tiptap/react";
import { BubbleMenu } from "@tiptap/react/menus";
import DragHandle from "@tiptap/extension-drag-handle-react";
import StarterKit from "@tiptap/starter-kit";
import { TaskItem, TaskList } from "@tiptap/extension-list";
import { TableKit } from "@tiptap/extension-table";
import { Placeholder } from "@tiptap/extensions";
import { forwardRef, useImperativeHandle, useRef, useState } from "react";
import { Bold, Code, GripVertical, Heading2, Heading3, Italic, Link2, Plus, Quote, Strikethrough, Unlink, Underline } from "lucide-react";
import { htmlToMarkdown, markdownToHtml } from "./markdown";
import { ButtonBlock, CardBlock, CardsBlock, HeroBlock, ImageBlock, RawHtmlBlock, RawHtmlInline, ShortcodeBlock, ShortcodeInline } from "./nodes";
import { SlashCommand, type SlashHooks } from "./slash";
import { t } from "../i18n";

export interface BlockEditorHandle {
  getMarkdown: () => string;
  insertImage: (ref: string, alt?: string, at?: { x: number; y: number }) => void;
  focus: () => void;
}

interface Props {
  markdown: string;
  resolveImage: (src: string) => string | null;
  pickImage: () => Promise<{ ref: string; alt?: string } | null>;
  prompt: SlashHooks["prompt"];
  addContactForm: SlashHooks["addContactForm"];
  addPageBlock: SlashHooks["addPageBlock"];
  pasteImage: (file: File) => Promise<{ ref: string } | null>;
  onChange: () => void;
}

export const BlockEditor = forwardRef<BlockEditorHandle, Props>(function BlockEditor(props, ref) {
  // The extensions capture hooks once. A ref keeps them fresh.
  const hooks = useRef(props);
  hooks.current = props;

  const editor = useEditor({
    extensions: [
      StarterKit.configure({
        heading: { levels: [1, 2, 3, 4] },
        link: { openOnClick: false, autolink: true, defaultProtocol: "https" },
        dropcursor: { color: "var(--accent)", width: 2 },
      }),
      TaskList,
      TaskItem.configure({ nested: true }),
      TableKit.configure({ table: { resizable: false } }),
      Placeholder.configure({
        placeholder: ({ node }) => (node.type.name === "heading" ? t.editor.headingPlaceholder : t.editor.bodyPlaceholder),
      }),
      ImageBlock.configure({
        resolve: (s: string) => hooks.current.resolveImage(s),
        pick: () => hooks.current.pickImage(),
      }),
      HeroBlock.configure({
        resolve: (s: string) => hooks.current.resolveImage(s),
        pick: () => hooks.current.pickImage(),
        describe: () => hooks.current.prompt(t.editor.heroImageDescribeTitle, t.editor.heroImageDescribeLabel),
      }),
      CardsBlock,
      CardBlock,
      ButtonBlock,
      ShortcodeBlock,
      ShortcodeInline,
      RawHtmlBlock,
      RawHtmlInline,
      SlashCommand.configure({
        hooks: {
          pickImage: () => hooks.current.pickImage(),
          prompt: (...a) => hooks.current.prompt(...a),
          addContactForm: () => hooks.current.addContactForm(),
          addPageBlock: (kind) => hooks.current.addPageBlock(kind),
        },
      }),
    ],
    content: markdownToHtml(props.markdown),
    editorProps: {
      attributes: { class: "prose-editor", spellcheck: "true" },
      handlePaste: (view, event) => {
        const files = Array.from(event.clipboardData?.files ?? []).filter((f) => f.type.startsWith("image/"));
        if (!files.length) return false;
        event.preventDefault();
        (async () => {
          for (const f of files) {
            const r = await hooks.current.pasteImage(f);
            if (r) view.dispatch(view.state.tr.replaceSelectionWith(view.state.schema.nodes.image.create({ src: r.ref, alt: "" })));
          }
        })();
        return true;
      },
    },
    onUpdate: () => hooks.current.onChange(),
    immediatelyRender: true,
    shouldRerenderOnTransaction: false,
  });

  useImperativeHandle(ref, () => ({
    getMarkdown: () => (editor ? htmlToMarkdown(editor.getHTML()) : props.markdown),
    insertImage: (src, alt = "", at) => {
      if (!editor) return;
      let pos: number | undefined;
      if (at) pos = editor.view.posAtCoords({ left: at.x, top: at.y })?.pos;
      const chain = editor.chain().focus();
      if (pos != null) chain.insertContentAt(pos, { type: "image", attrs: { src, alt } }).run();
      else chain.insertContent({ type: "image", attrs: { src, alt } }).run();
    },
    focus: () => editor?.commands.focus(),
  }), [editor, props.markdown]);

  if (!editor) return null;
  return (
    <>
      <DragHandle editor={editor} className="drag-handle-wrap">
        <div className="row" style={{ gap: 0, marginLeft: -50 }}>
          <button
            className="add-block"
            title={t.editor.addBlock}
            onMouseDown={(e) => {
              e.preventDefault();
              const { $from } = editor.state.selection;
              const end = $from.end();
              editor.chain().focus().insertContentAt(end + 1, { type: "paragraph" }).setTextSelection(end + 2).insertContent("/").run();
            }}
          >
            <Plus size={16} />
          </button>
          <div className="drag-handle" title={t.editor.dragToMove}><GripVertical size={15} /></div>
        </div>
      </DragHandle>
      <FormatBubble editor={editor} />
      <EditorContent editor={editor} />
      <button
        className="btn add-block-canvas"
        onMouseDown={(e) => {
          e.preventDefault();
          const last = editor.state.doc.lastChild;
          const chain = editor.chain().focus("end");
          if (!last || last.type.name !== "paragraph" || last.content.size > 0) chain.insertContentAt(editor.state.doc.content.size, { type: "paragraph" });
          chain.insertContent("/").run();
        }}
      >
        <Plus size={15} /> {t.editor.addBlockButton}
      </button>
    </>
  );
});

function FormatBubble({ editor }: { editor: Editor }) {
  const [linking, setLinking] = useState(false);
  const [href, setHref] = useState("");
  // Re-render only when these values change (re-rendering on every
  // transaction makes the bubble menu and the editor update each other forever).
  const active = useEditorState({
    editor,
    selector: ({ editor: e }) => ({
      bold: e.isActive("bold"), italic: e.isActive("italic"), underline: e.isActive("underline"), strike: e.isActive("strike"),
      code: e.isActive("code"), link: e.isActive("link"), h2: e.isActive("heading", { level: 2 }), h3: e.isActive("heading", { level: 3 }),
      quote: e.isActive("blockquote"),
    }),
  });

  const B = (p: { on?: boolean; title: string; run: () => void; children: React.ReactNode }) => (
    <button className={p.on ? "on" : ""} title={p.title} onMouseDown={(e) => { e.preventDefault(); p.run(); }}>{p.children}</button>
  );
  const applyLink = () => {
    const v = href.trim();
    if (!v) editor.chain().focus().unsetLink().run();
    else editor.chain().focus().extendMarkRange("link").setLink({ href: /^(https?:|mailto:|\/|#)/.test(v) ? v : "https://" + v }).run();
    setLinking(false);
  };
  return (
    <BubbleMenu
      editor={editor}
      shouldShow={({ editor: e, from, to }) => from !== to && !e.isActive("image") && !e.isActive("shortcodeBlock") && !e.isActive("rawHtmlBlock") && !e.isActive("codeBlock")}
      options={{ placement: "top" }}
    >
      <div className="bubble">
        {linking ? (
          <>
            <input
              autoFocus
              value={href}
              placeholder={t.editor.linkPlaceholder}
              onChange={(e) => setHref(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); applyLink(); } if (e.key === "Escape") setLinking(false); }}
            />
            <B title={t.editor.apply} run={applyLink}><Link2 size={15} /></B>
          </>
        ) : (
          <>
            <B title={t.editor.bold} on={active.bold} run={() => editor.chain().focus().toggleBold().run()}><Bold size={15} /></B>
            <B title={t.editor.italic} on={active.italic} run={() => editor.chain().focus().toggleItalic().run()}><Italic size={15} /></B>
            <B title={t.editor.underline} on={active.underline} run={() => editor.chain().focus().toggleUnderline().run()}><Underline size={15} /></B>
            <B title={t.editor.strikethrough} on={active.strike} run={() => editor.chain().focus().toggleStrike().run()}><Strikethrough size={15} /></B>
            <B title={t.editor.code} on={active.code} run={() => editor.chain().focus().toggleCode().run()}><Code size={15} /></B>
            <div className="bsep" />
            {active.link ? (
              <B title={t.editor.removeLink} on run={() => editor.chain().focus().unsetLink().run()}><Unlink size={15} /></B>
            ) : (
              <B title={t.editor.link} run={() => { setHref(editor.getAttributes("link").href ?? ""); setLinking(true); }}><Link2 size={15} /></B>
            )}
            <div className="bsep" />
            <B title={t.editor.heading} on={active.h2} run={() => editor.chain().focus().toggleHeading({ level: 2 }).run()}><Heading2 size={15} /></B>
            <B title={t.editor.subheading} on={active.h3} run={() => editor.chain().focus().toggleHeading({ level: 3 }).run()}><Heading3 size={15} /></B>
            <B title={t.editor.quote} on={active.quote} run={() => editor.chain().focus().toggleBlockquote().run()}><Quote size={15} /></B>
          </>
        )}
      </div>
    </BubbleMenu>
  );
}
