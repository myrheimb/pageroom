// Markdown <-> editor HTML.
//
// Shortcodes ({{< youtube id >}}), raw HTML and the <!--more--> divider must
// survive editing. They are lifted out before rendering, shown as locked
// blocks and written back byte-for-byte.
import MarkdownIt from "markdown-it";
import TurndownService from "turndown";
// @ts-expect-error: the plugin ships without types
import { gfm } from "turndown-plugin-gfm";
import { t } from "../i18n";

export function b64(s: string): string {
  const bytes = new TextEncoder().encode(s);
  let bin = "";
  bytes.forEach((b) => (bin += String.fromCharCode(b)));
  return btoa(bin);
}

// Inline placeholders carry their text so Turndown treats them as content
// (empty inline elements make it eat the neighbouring spaces).
export function esc(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

export function unb64(s: string): string {
  const bin = atob(s);
  const bytes = Uint8Array.from(bin, (c) => c.charCodeAt(0));
  return new TextDecoder().decode(bytes);
}

const md = new MarkdownIt({ html: true, linkify: false, typographer: false, breaks: false });

md.renderer.rules.html_block = (tokens, idx) => `<div data-raw-html="${b64(tokens[idx].content.replace(/\n+$/, ""))}"></div>`;
md.renderer.rules.html_inline = (tokens, idx) => `<span data-raw-inline="${b64(tokens[idx].content)}">${esc(tokens[idx].content)}</span>`;

// GFM task lists: "- [ ] todo" / "- [x] done"
md.core.ruler.after("inline", "tasklists", (state) => {
  const t = state.tokens;
  for (let i = 2; i < t.length; i++) {
    if (t[i].type !== "inline" || t[i - 1].type !== "paragraph_open" || t[i - 2].type !== "list_item_open") continue;
    const m = /^\[([ xX])\]\s+/.exec(t[i].content);
    if (!m) continue;
    t[i - 2].attrSet("data-type", "taskItem");
    t[i - 2].attrSet("data-checked", m[1] === " " ? "false" : "true");
    const first = t[i].children?.[0];
    if (first && first.type === "text") first.content = first.content.slice(m[0].length);
    for (let j = i - 3; j >= 0; j--) {
      if (t[j].type === "bullet_list_open" && t[j].level === t[i - 2].level - 1) {
        t[j].attrSet("data-type", "taskList");
        break;
      }
    }
  }
});

// Shortcode tokens: {{< name args >}}, {{% name %}}, {{< /name >}}, {{</* escaped */>}}
const SC = /\{\{([<%])(\/\*)?\s*(\/)?\s*([\w\-./]+)([\s\S]*?)(\*\/)?\s*([>%])\}\}/g;

interface Lifted {
  text: string;
  blocks: string[];
  inlines: string[];
}

export function liftShortcodes(src: string): Lifted {
  const blocks: string[] = [];
  const inlines: string[] = [];
  const codeRanges = findCodeRanges(src);
  const inCode = (i: number) => codeRanges.some(([a, b]) => i >= a && i < b);

  type Tok = { start: number; end: number; name: string; closing: boolean; selfClosing: boolean };
  const toks: Tok[] = [];
  SC.lastIndex = 0;
  let m: RegExpExecArray | null;
  while ((m = SC.exec(src))) {
    if (inCode(m.index)) continue;
    const args = m[5] ?? "";
    toks.push({ start: m.index, end: m.index + m[0].length, name: m[4], closing: !!m[3], selfClosing: /\/\s*$/.test(args.trim()) && !m[2] });
  }

  // Pair opening and closing tags (with nesting) into spans.
  const spans: [number, number][] = [];
  const used = new Set<number>();
  for (let i = 0; i < toks.length; i++) {
    if (used.has(i) || toks[i].closing) continue;
    let depth = 0, close = -1;
    if (!toks[i].selfClosing) {
      for (let j = i + 1; j < toks.length; j++) {
        if (toks[j].name !== toks[i].name) continue;
        if (!toks[j].closing && !toks[j].selfClosing) depth++;
        else if (toks[j].closing) {
          if (depth === 0) { close = j; break; }
          depth--;
        }
      }
    }
    if (close >= 0) {
      for (let k = i; k <= close; k++) used.add(k);
      spans.push([toks[i].start, toks[close].end]);
    } else {
      used.add(i);
      spans.push([toks[i].start, toks[i].end]);
    }
  }
  // stray closing tags
  toks.forEach((t, i) => { if (!used.has(i)) spans.push([t.start, t.end]); });
  spans.sort((a, b) => a[0] - b[0]);

  let out = "", pos = 0;
  for (const [s, e] of spans) {
    if (s < pos) continue; // nested inside an earlier span
    out += src.slice(pos, s);
    const raw = src.slice(s, e);
    const lineStart = s === 0 || src[s - 1] === "\n" || /^[ \t]*$/.test(src.slice(src.lastIndexOf("\n", s - 1) + 1, s));
    const after = src.slice(e, src.indexOf("\n", e) === -1 ? src.length : src.indexOf("\n", e));
    const lineEnd = /^[ \t]*$/.test(after);
    if (lineStart && lineEnd) {
      out = out.replace(/[ \t]*$/, "");
      out += `PRSCBLOCK${blocks.length}X`;
      blocks.push(raw);
    } else {
      out += `PRSCINLINE${inlines.length}X`;
      inlines.push(raw);
    }
    pos = e;
  }
  out += src.slice(pos);
  return { text: out, blocks, inlines };
}

function findCodeRanges(src: string): [number, number][] {
  const ranges: [number, number][] = [];
  const fence = /^( {0,3})(`{3,}|~{3,})[^\n]*\n[\s\S]*?(?:\n\1?\2[`~]*[ \t]*(?:\n|$)|$)/gm;
  let m: RegExpExecArray | null;
  while ((m = fence.exec(src))) ranges.push([m.index, m.index + m[0].length]);
  const inline = /`[^`\n]+`/g;
  while ((m = inline.exec(src))) {
    const at = m.index;
    if (!ranges.some(([a, b]) => at >= a && at < b)) ranges.push([at, at + m[0].length]);
  }
  return ranges;
}

export function markdownToHtml(src: string): string {
  const lifted = liftShortcodes(src);
  let html = md.render(lifted.text);
  html = html.replace(/<p>PRSCBLOCK(\d+)X<\/p>\n?/g, (_, i) => `<div data-shortcode="${b64(lifted.blocks[+i])}"></div>`);
  // a block placeholder that markdown-it merged into a paragraph
  html = html.replace(/PRSCBLOCK(\d+)X/g, (_, i) => `<span data-shortcode-inline="${b64(lifted.blocks[+i])}">${esc(lifted.blocks[+i])}</span>`);
  html = html.replace(/PRSCINLINE(\d+)X/g, (_, i) => `<span data-shortcode-inline="${b64(lifted.inlines[+i])}">${esc(lifted.inlines[+i])}</span>`);
  return html;
}

const safeLinkSchemes = new Set(["http:", "https:", "mailto:"]);

/** Drops href and src attributes that point anywhere but http, https, mailto or a relative path. */
export function stripUnsafeUrls(root: ParentNode): void {
  root.querySelectorAll("[href], [src]").forEach((el) => {
    for (const name of ["href", "src"]) {
      const value = el.getAttribute(name);
      if (value === null) continue;
      // browsers ignore whitespace and control characters anywhere in a URL scheme
      const scheme = /^([a-z][a-z0-9+.-]*:)/i.exec(value.replace(/[\s\u0000-\u001f]/g, ""))?.[1].toLowerCase();
      if (scheme && !safeLinkSchemes.has(scheme)) el.removeAttribute(name);
    }
  });
}


const td = new TurndownService({
  headingStyle: "atx",
  hr: "---",
  bulletListMarker: "-",
  codeBlockStyle: "fenced",
  emDelimiter: "_",
  strongDelimiter: "**",
  linkStyle: "inlined",
  br: "  ",
  // Our placeholders are empty elements. Turndown would drop them as blank.
  blankReplacement: (_content, node) => {
    const el = node as HTMLElement;
    const raw = (attr: string) => unb64(el.getAttribute(attr) || "");
    if (el.hasAttribute?.("data-shortcode")) return `\n\n${raw("data-shortcode")}\n\n`;
    if (el.hasAttribute?.("data-raw-html")) return `\n\n${raw("data-raw-html")}\n\n`;
    if (el.hasAttribute?.("data-shortcode-inline")) return raw("data-shortcode-inline");
    if (el.hasAttribute?.("data-raw-inline")) return raw("data-raw-inline");
    return (node as any).isBlock ? "\n\n" : "";
  },
});
td.use(gfm);

td.addRule("shortcodeBlock", {
  filter: (n) => n.nodeName === "DIV" && (n as HTMLElement).hasAttribute("data-shortcode"),
  replacement: (_c, n) => `\n\n${unb64((n as HTMLElement).getAttribute("data-shortcode") || "")}\n\n`,
});
td.addRule("shortcodeInline", {
  filter: (n) => n.nodeName === "SPAN" && (n as HTMLElement).hasAttribute("data-shortcode-inline"),
  replacement: (_c, n) => unb64((n as HTMLElement).getAttribute("data-shortcode-inline") || ""),
});
td.addRule("rawBlock", {
  filter: (n) => n.nodeName === "DIV" && (n as HTMLElement).hasAttribute("data-raw-html"),
  replacement: (_c, n) => `\n\n${unb64((n as HTMLElement).getAttribute("data-raw-html") || "")}\n\n`,
});
td.addRule("rawInline", {
  filter: (n) => n.nodeName === "SPAN" && (n as HTMLElement).hasAttribute("data-raw-inline"),
  replacement: (_c, n) => unb64((n as HTMLElement).getAttribute("data-raw-inline") || ""),
});
td.addRule("image", {
  filter: "img",
  replacement: (_c, n) => {
    const el = n as HTMLElement;
    const alt = (el.getAttribute("alt") || "").replace(/([\[\]])/g, "\\$1");
    const src = (el.getAttribute("data-src") || el.getAttribute("src") || "").replace(/\s/g, "%20");
    const title = el.getAttribute("title");
    const t = title ? ` "${title.replace(/"/g, '\\"')}"` : "";
    const block = el.parentElement?.nodeName === "BODY" || el.parentElement?.getAttribute("data-image-block") != null;
    const out = `![${alt}](${src}${t})`;
    return block ? `\n\n${out}\n\n` : out;
  },
});
td.addRule("listItem", {
  filter: (n) => n.nodeName === "LI" && (n as HTMLElement).getAttribute("data-type") !== "taskItem",
  replacement: (content, n) => {
    const parent = n.parentNode as HTMLElement;
    let prefix = "- ";
    if (parent.nodeName === "OL") {
      const start = Number(parent.getAttribute("start") || 1);
      prefix = `${start + Array.prototype.indexOf.call(parent.children, n)}. `;
    }
    const indent = " ".repeat(prefix.length);
    const body = content.replace(/^\n+/, "").replace(/\n+$/, "\n").replace(/\n(?!$)/gm, "\n" + indent);
    return prefix + body + (n.nextSibling && !/\n$/.test(body) ? "\n" : "");
  },
});
td.addRule("taskItem", {
  filter: (n) => n.nodeName === "LI" && (n as HTMLElement).getAttribute("data-type") === "taskItem",
  replacement: (content, n) => {
    const checked = (n as HTMLElement).getAttribute("data-checked") === "true";
    const text = content.replace(/^\n+/, "").replace(/\n+$/, "\n").replace(/\n/gm, "\n  ");
    return `- [${checked ? "x" : " "}] ${text.trimEnd()}\n`;
  },
});
td.addRule("tableCellOneLine", {
  filter: ["th", "td"],
  replacement: (content, n) => {
    const cell = content.trim().replace(/\n+/g, " ").replace(/\|/g, "\\|");
    const isFirst = !(n as HTMLElement).previousElementSibling;
    return (isFirst ? "| " : " ") + cell + " |";
  },
});
// Keep underline and similar as HTML, since Markdown has no syntax for them.
td.keep(["u", "sub", "sup", "mark", "kbd"] as any);

export function htmlToMarkdown(html: string): string {
  let out = td.turndown(html);
  out = out.replace(/\n{3,}/g, "\n\n").replace(/^\n+/, "");
  return out.trimEnd() + "\n";
}

/** Human label for a shortcode, e.g. "YouTube video" for {{< youtube id >}}. */
export function describeShortcode(raw: string): { name: string; label: string; args: string } {
  const m = /\{\{[<%](?:\/\*)?\s*\/?\s*([\w\-./]+)([\s\S]*?)(?:\*\/)?\s*[>%]\}\}/.exec(raw);
  const name = m?.[1] ?? "shortcode";
  const args = (m?.[2] ?? "").trim();
  return { name, label: t.shortcodes.known[name] ?? t.shortcodes.unknown(name), args };
}
