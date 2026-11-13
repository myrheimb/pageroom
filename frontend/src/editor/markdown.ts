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

// Page blocks: {{< pr-hero >}}, {{< pr-cards >}} with {{< pr-card >}} items and
// {{< pr-button >}}. A block the editor can represent exactly becomes a TipTap
// node. Anything else (unknown attributes, raw HTML inside, odd nesting) is
// left to liftShortcodes and stays a locked shortcode block.
const PR_ATTRIBUTES: Record<string, string[]> = {
  "pr-align": ["to"],
  "pr-hero": ["image", "alt", "button", "link", "align"],
  "pr-cards": ["align"],
  "pr-card": [],
  "pr-columns": ["layout", "valign"],
  "pr-column": [],
  "pr-button": ["link", "align"],
  "pr-video": ["id", "vimeo", "src"],
};

interface PrToken {
  start: number;
  end: number;
  name: string;
  closing: boolean;
  selfClosing: boolean;
  raw: string;
}

function prTokens(src: string): PrToken[] {
  const codeRanges = findCodeRanges(src);
  const toks: PrToken[] = [];
  const re = new RegExp(SC.source, "g");
  let m: RegExpExecArray | null;
  while ((m = re.exec(src))) {
    const at = m.index;
    if (!m[4].startsWith("pr-") || codeRanges.some(([a, b]) => at >= a && at < b)) continue;
    toks.push({ start: at, end: at + m[0].length, name: m[4], closing: !!m[3], selfClosing: /\/\s*$/.test((m[5] ?? "").trim()) && !m[2], raw: m[0] });
  }
  return toks;
}

function prMatchingClose(toks: PrToken[], open: number): number {
  let depth = 0;
  for (let j = open + 1; j < toks.length; j++) {
    if (toks[j].name !== toks[open].name) continue;
    if (toks[j].closing) {
      if (depth === 0) return j;
      depth--;
    } else if (!toks[j].selfClosing) depth++;
  }
  return -1;
}

function parsePrAttributes(openTag: string, name: string): Record<string, string> | null {
  const m = /^\{\{<\s*pr-[\w-]+([\s\S]*?)\s*>\}\}$/.exec(openTag);
  if (!m) return null;
  const attrs: Record<string, string> = {};
  const pair = /^\s+(\w+)="((?:[^"\\]|\\.)*)"/;
  let rest = m[1];
  let found: RegExpExecArray | null;
  while ((found = pair.exec(rest))) {
    if (!PR_ATTRIBUTES[name].includes(found[1]) || found[1] in attrs) return null;
    attrs[found[1]] = found[2].replace(/\\(.)/g, "$1");
    rest = rest.slice(found[0].length);
  }
  return rest.trim() === "" ? attrs : null;
}

function hasRawHtml(markdown: string): boolean {
  const flatten = (tokens: { type: string; children?: unknown }[]): boolean =>
    tokens.some((token) => token.type === "html_block" || token.type === "html_inline" || (Array.isArray(token.children) && flatten(token.children)));
  return flatten(md.parse(liftShortcodes(markdown).text, {}));
}

const COLUMN_TAGS = /^(P|H[1-4]|UL|OL|BLOCKQUOTE|TABLE|PRE|IMG)$/;

// A column holds only what the column schema accepts, so that loading never drops content.
function renderColumn(inner: string): string | null {
  const content = inner.trim();
  if (content === "") return '<div data-pr="column"></div>';
  if (prTokens(content).some((token) => !["pr-align", "pr-button", "pr-video"].includes(token.name)) || hasRawHtml(content)) return null;
  const leftover = liftShortcodes(extractPrBlocks(content).text);
  if (leftover.blocks.length > 0 || leftover.inlines.length > 0) return null;

  const html = markdownToHtml(content);
  const body = new DOMParser().parseFromString(html, "text/html").body;
  const allowed = Array.from(body.children).every((element) => COLUMN_TAGS.test(element.nodeName) || ["button", "video"].includes(element.getAttribute("data-pr") ?? ""));
  return allowed ? `<div data-pr="column">${html}</div>` : null;
}

function renderPrBlock(raw: string): string | null {
  const toks = prTokens(raw);
  if (toks.length === 1 && toks[0].name === "pr-video") {
    const attrs = parsePrAttributes(toks[0].raw, "pr-video");
    const entries = Object.entries(attrs ?? {});
    if (entries.length !== 1) return null;
    const [key, value] = entries[0];
    const valid = key === "src" ? value.trim() !== "" && !/[\s<>]/.test(value) : key === "vimeo" ? /^\d+$/.test(value) : /^[\w-]+$/.test(value);
    return valid ? `<div data-pr="video" data-${key}="${esc(value)}"></div>` : null;
  }
  if (toks.length < 2 || toks[0].start !== 0 || toks[0].closing || toks[0].selfClosing) return null;
  const close = prMatchingClose(toks, 0);
  if (close !== toks.length - 1 || toks[close].end !== raw.length) return null;

  const name = toks[0].name;
  if (!(name in PR_ATTRIBUTES)) return null;
  const attrs = parsePrAttributes(toks[0].raw, name);
  if (!attrs) return null;
  const inner = raw.slice(toks[0].end, toks[close].start).trim();
  const attributeHtml = Object.entries(attrs).map(([key, value]) => ` data-${key}="${esc(value)}"`).join("");
  const kind = name.slice("pr-".length);
  const wrap = (content: string) => `<div data-pr="${kind}"${attributeHtml}>${content}</div>`;

  if (name === "pr-align") {
    if (toks.length !== 2 || !["center", "right"].includes(attrs.to) || hasRawHtml(inner)) return null;
    const html = markdownToHtml(inner).trim();
    const picture = /^<p>(<img [^>]*>)<\/p>$/.exec(html);
    if (picture) return picture[1].replace("<img ", `<img data-align="${attrs.to}" `);
    const parsed = new DOMParser().parseFromString(html, "text/html").body;
    const single = parsed.children.length === 1 && /^(P|H[1-6]|BLOCKQUOTE)$/.test(parsed.children[0].nodeName) ? parsed.children[0] : null;
    return single ? html.replace(/^<(p|h[1-6]|blockquote)>/, `<$1 data-align="${attrs.to}">`) : null;
  }
  if (name === "pr-button") {
    return /\n|\{\{/.test(inner) ? null : wrap(esc(inner));
  }
  if (name === "pr-column") return null;
  if (name === "pr-columns") {
    if ("layout" in attrs && !["1-1-1", "1-2", "2-1"].includes(attrs.layout)) return null;
    if ("valign" in attrs && !["bottom", "center"].includes(attrs.valign)) return null;
    const columns: string[] = [];
    let cursor = toks[0].end;
    for (let i = 1; i < close; ) {
      const open = toks[i];
      const columnClose = open.name === "pr-column" && !open.closing && !open.selfClosing ? prMatchingClose(toks, i) : -1;
      if (columnClose < 0 || raw.slice(cursor, open.start).trim() !== "" || !parsePrAttributes(open.raw, "pr-column")) return null;
      const html = renderColumn(raw.slice(open.end, toks[columnClose].start));
      if (html === null) return null;
      columns.push(html);
      cursor = toks[columnClose].end;
      i = columnClose + 1;
    }
    const expectedColumns = attrs.layout === "1-1-1" ? 3 : 2;
    return columns.length === expectedColumns && raw.slice(cursor, toks[close].start).trim() === "" ? wrap(columns.join("")) : null;
  }
  if (name === "pr-cards") {
    if (toks.length < 4) return null;
    let cards = "";
    let cursor = toks[0].end;
    for (let i = 1; i < toks.length - 1; i += 2) {
      const card = toks[i];
      const cardClose = toks[i + 1];
      if (card.name !== "pr-card" || card.closing || card.selfClosing || cardClose.name !== "pr-card" || !cardClose.closing) return null;
      if (raw.slice(cursor, card.start).trim() !== "") return null;
      const html = renderPrBlock(raw.slice(card.start, cardClose.end));
      if (html === null) return null;
      cards += html;
      cursor = cardClose.end;
    }
    return raw.slice(cursor, toks[close].start).trim() === "" ? wrap(cards) : null;
  }
  if (toks.length !== 2 || hasRawHtml(inner)) return null;
  return wrap(inner === "" ? "" : markdownToHtml(inner));
}

export function extractPrBlocks(src: string): { text: string; blocks: string[] } {
  const toks = prTokens(src);
  const blocks: string[] = [];
  let out = "";
  let pos = 0;
  let i = 0;
  while (i < toks.length) {
    const tok = toks[i];
    const hasNoClosingTag = tok.name === "pr-video";
    const close = tok.closing || tok.selfClosing ? -1 : hasNoClosingTag ? i : prMatchingClose(toks, i);
    if (close < 0) {
      i++;
      continue;
    }
    const end = toks[close].end;
    const lineStart = tok.start === 0 || src[tok.start - 1] === "\n";
    const lineEnd = /^[ \t]*$/.test(src.slice(end, src.indexOf("\n", end) === -1 ? src.length : src.indexOf("\n", end)));
    const html = lineStart && lineEnd ? renderPrBlock(src.slice(tok.start, end)) : null;
    if (html !== null) {
      out += `${src.slice(pos, tok.start)}\n\nPRBLOCK${blocks.length}X\n\n`;
      blocks.push(html);
      pos = end;
    }
    while (i < toks.length && toks[i].start < end) i++;
  }
  return { text: out + src.slice(pos), blocks };
}

export function markdownToHtml(src: string): string {
  const extracted = extractPrBlocks(src);
  const lifted = liftShortcodes(extracted.text);
  let html = md.render(lifted.text);
  html = html.replace(/<p>PRBLOCK(\d+)X<\/p>\n?/g, (_, i) => extracted.blocks[+i]);
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
  blankReplacement: (content, node) => {
    const el = node as HTMLElement;
    const raw = (attr: string) => unb64(el.getAttribute(attr) || "");
    if (el.hasAttribute?.("data-pr")) return prBlockMarkdown(el, content);
    if (el.hasAttribute?.("data-shortcode")) return `\n\n${raw("data-shortcode")}\n\n`;
    if (el.hasAttribute?.("data-raw-html")) return `\n\n${raw("data-raw-html")}\n\n`;
    if (el.hasAttribute?.("data-shortcode-inline")) return raw("data-shortcode-inline");
    if (el.hasAttribute?.("data-raw-inline")) return raw("data-raw-inline");
    return (node as any).isBlock ? "\n\n" : "";
  },
});
td.use(gfm);

function prBlockMarkdown(el: HTMLElement, content: string): string {
  const kind = el.getAttribute("data-pr") || "";
  const name = `pr-${kind}`;
  const attrs = (PR_ATTRIBUTES[name] ?? [])
    .map((key) => [key, el.getAttribute(`data-${key}`) || ""])
    .filter(([, value]) => value !== "")
    .map(([key, value]) => ` ${key}="${value.replace(/[\\"]/g, "\\$&")}"`)
    .join("");
  const body = kind === "button" ? (el.textContent || "").trim() : content.replace(/\n{3,}/g, "\n\n").trim();
  const open = `{{< ${name}${attrs} >}}`;
  const close = `{{< /${name} >}}`;
  if (kind === "video") return `\n\n${open}\n\n`;
  if (kind === "button") return `\n\n${open}${body}${close}\n\n`;
  return `\n\n${open}\n${body === "" ? "" : body + "\n"}${close}\n\n`;
}

td.addRule("prBlock", {
  filter: (n) => n.nodeName === "DIV" && (n as HTMLElement).hasAttribute("data-pr"),
  replacement: (content, n) => prBlockMarkdown(n as HTMLElement, content),
});
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
function imageMarkdown(el: HTMLElement): string {
  const alt = (el.getAttribute("alt") || "").replace(/([\[\]])/g, "\\$1");
  const src = (el.getAttribute("data-src") || el.getAttribute("src") || "").replace(/\s/g, "%20");
  const title = el.getAttribute("title");
  const t = title ? ` "${title.replace(/"/g, '\\"')}"` : "";
  return `![${alt}](${src}${t})`;
}

td.addRule("image", {
  filter: "img",
  replacement: (_c, n) => {
    const el = n as HTMLElement;
    const block = el.parentElement?.nodeName === "BODY" || el.parentElement?.getAttribute("data-image-block") != null || el.parentElement?.getAttribute("data-pr") === "column";
    const out = imageMarkdown(el);
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
// A shortcode can't sit inside another page block or a list, so only top-level content is aligned.
td.addRule("aligned", {
  filter: (n) => /^(P|H[1-6]|IMG|BLOCKQUOTE)$/.test(n.nodeName) && !!(n as HTMLElement).getAttribute("data-align") && ((n as HTMLElement).parentElement?.id === "turndown-root" || (n as HTMLElement).parentElement?.getAttribute("data-pr") === "column"),
  replacement: (content, n) => {
    const el = n as HTMLElement;
    const heading = /^H([1-6])$/.exec(el.nodeName);
    const quote = el.nodeName === "BLOCKQUOTE" ? content.trim().replace(/^/gm, "> ") : null;
    const body = el.nodeName === "IMG" ? imageMarkdown(el) : heading ? `${"#".repeat(+heading[1])} ${content}` : quote ?? content;
    return `\n\n{{< pr-align to="${el.getAttribute("data-align")}" >}}\n${body}\n{{< /pr-align >}}\n\n`;
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
