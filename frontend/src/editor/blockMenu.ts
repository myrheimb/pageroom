import type { Editor } from "@tiptap/core";
import { Fragment, type Node as PMNode, type NodeType } from "@tiptap/pm/model";
import { TextSelection } from "@tiptap/pm/state";
import type { SlashHooks } from "./slash";

export type TextTarget = "paragraph" | "heading2" | "heading3" | "heading4" | "bulletList" | "orderedList" | "taskList" | "blockquote" | "codeBlock";
export type SectionTarget = "prHero" | "prCards" | "prColumns";
export type Target = TextTarget | SectionTarget;

const TEXT_TARGETS: TextTarget[] = ["paragraph", "heading2", "heading3", "heading4", "bulletList", "orderedList", "taskList", "blockquote", "codeBlock"];
const TEXT_BLOCKS = ["paragraph", "heading", "bulletList", "orderedList", "taskList", "blockquote", "codeBlock"];

const currentTarget = (node: PMNode): string => (node.type.name === "heading" ? `heading${node.attrs.level}` : node.type.name);

/** The blocks a block can turn into, given the block and the node that holds it. */
export function turnIntoTargets(node: PMNode, parent: PMNode): Target[] {
  const name = node.type.name;
  if (TEXT_BLOCKS.includes(name)) {
    const others = TEXT_TARGETS.filter((target) => target !== currentTarget(node));
    return parent.type.name === "doc" ? [...others, "prHero", "prCards"] : others;
  }
  if (name === "prHero") return ["paragraph"];
  if (name === "prCards") return node.childCount >= 2 && node.childCount <= 3 ? ["paragraph", "prColumns"] : ["paragraph"];
  if (name === "prColumns") return ["paragraph", "prCards"];
  return [];
}

const fits = (type: NodeType, child: PMNode) => type.validContent(Fragment.from(child));

/**
 * Moves each child of a Card or Column into the other kind of block. Children the new block can't hold are
 * returned separately so they can go after it instead of being lost.
 */
function regroup(container: PMNode, itemType: NodeType) {
  const items: PMNode[] = [];
  const leftover: PMNode[] = [];
  container.forEach((item) => {
    const kept: PMNode[] = [];
    item.forEach((child) => (fits(itemType, child) ? kept.push(child) : leftover.push(child)));
    items.push(itemType.create(null, kept.length ? kept : [itemType.schema.nodes.paragraph.create()]));
  });
  return { items, leftover };
}

/**
 * Turns the block at `pos` into `target` and returns how many blocks had to move after it because the new
 * block can't hold them. Returns null when nothing changed, such as when the site's block template couldn't
 * be added.
 */
export async function turnInto(editor: Editor, pos: number, target: Target, addPageBlock: SlashHooks["addPageBlock"]): Promise<number | null> {
  const node = editor.state.doc.nodeAt(pos);
  if (!node) return null;
  const { schema } = editor.state;
  const name = node.type.name;
  const replace = (nodes: PMNode[]) => editor.chain().focus().command(({ tr }) => {
    tr.replaceWith(pos, pos + node.nodeSize, nodes);
    return true;
  }).run();

  if (TEXT_BLOCKS.includes(name)) {
    if (target === "prHero" || target === "prCards") {
      if (!(await addPageBlock(target === "prHero" ? "hero" : "cards"))) return null;
      replace([target === "prHero" ? schema.nodes.prHero.create(null, node) : schema.nodes.prCards.create(null, schema.nodes.prCard.create(null, node))]);
      return 0;
    }
    const chain = editor.chain().focus().command(({ tr }) => {
      tr.setSelection(TextSelection.between(tr.doc.resolve(pos), tr.doc.resolve(pos + node.nodeSize)));
      return true;
    }).clearNodes();
    if (target === "heading2" || target === "heading3" || target === "heading4") chain.setHeading({ level: Number(target.slice(-1)) as 2 | 3 | 4 });
    else if (target === "bulletList") chain.toggleBulletList();
    else if (target === "orderedList") chain.toggleOrderedList();
    else if (target === "taskList") chain.toggleTaskList();
    else if (target === "blockquote") chain.toggleBlockquote();
    else if (target === "codeBlock") chain.toggleCodeBlock();
    chain.run();
    return 0;
  }

  if (target === "paragraph") {
    const nodes: PMNode[] = [];
    if (name === "prHero" && node.attrs.image) nodes.push(schema.nodes.image.create({ src: node.attrs.image, alt: node.attrs.alt }));
    if (name === "prHero") node.forEach((child) => nodes.push(child));
    else node.forEach((item) => item.forEach((child) => nodes.push(child)));
    if (name === "prHero" && node.attrs.button) {
      if (!(await addPageBlock("button"))) return null;
      nodes.push(schema.nodes.prButton.create({ link: node.attrs.link, align: node.attrs.align }, schema.text(node.attrs.button)));
    }
    replace(nodes);
    return 0;
  }

  if (name === "prCards" && target === "prColumns") {
    if (!(await addPageBlock("columns"))) return null;
    const { items, leftover } = regroup(node, schema.nodes.prColumn);
    replace([schema.nodes.prColumns.create(null, items), ...leftover]);
    return leftover.length;
  }

  if (name === "prColumns" && target === "prCards") {
    if (!(await addPageBlock("cards"))) return null;
    const { items, leftover } = regroup(node, schema.nodes.prCard);
    replace([schema.nodes.prCards.create(null, items), ...leftover]);
    return leftover.length;
  }

  return null;
}

/** Deletes the block at `pos`. A blank paragraph takes its place when its parent can't be left empty. */
export function deleteBlock(editor: Editor, pos: number) {
  const node = editor.state.doc.nodeAt(pos);
  if (!node) return;
  const parent = editor.state.doc.resolve(pos).parent;
  editor.chain().focus().command(({ tr }) => {
    if (parent.childCount === 1) tr.replaceWith(pos, pos + node.nodeSize, editor.schema.nodes.paragraph.create());
    else tr.delete(pos, pos + node.nodeSize);
    return true;
  }).run();
}
