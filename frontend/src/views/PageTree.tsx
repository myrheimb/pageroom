import { useMemo, useState } from "react";
import { DndContext, DragOverlay, PointerSensor, useDraggable, useDroppable, useSensor, useSensors, type DragEndEvent } from "@dnd-kit/core";
import { ChevronDown, ChevronRight, FileText, Folder, FolderOpen, GripVertical, MoreHorizontal, Plus, Search, Trash2, Link2, FolderInput, Pencil, FilePlus2, FolderPlus, AlertTriangle } from "lucide-react";
import { api, type TreeNode } from "../api";
import { useStore, parentOf, findTranslation } from "../store";
import { ask, Menu } from "../ui";
import { t } from "../i18n";

interface Props {
  tree: TreeNode;
  selected?: string;
  languages: string[];
  changed: Set<string>;
  onOpen: (path: string) => void;
  onChanged: (newSelected?: string) => void;
  labelFor: (section: TreeNode) => string;
}

export function PageTree({ tree, selected, languages, changed, onOpen, onChanged, labelFor }: Props) {
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState<Set<string>>(() => new Set(tree.children?.filter((c) => c.kind === "section").map((c) => c.path) ?? []));
  const [dragging, setDragging] = useState<TreeNode | null>(null);
  const error = useStore((s) => s.error);
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }));

  const editing = (selected && findTranslation(tree, selected)?.language) || "";
  const multilingual = languages.length > 1;

  const visible = useMemo(() => (query.trim() ? filterTree(tree, query.trim().toLowerCase()) : tree), [tree, query]);
  const toggle = (p: string) => setOpen((o) => { const n = new Set(o); n.has(p) ? n.delete(p) : n.add(p); return n; });

  const newIn = async (section: TreeNode, kind: "page" | "section") => {
    const title = await ask.prompt(kind === "page" ? t.pageTree.newPageIn(section.title) : t.pageTree.newSectionIn(section.title), kind === "page" ? t.pageTree.title : t.pageTree.sectionName, kind === "page" ? t.pageTree.pagePlaceholder : t.pageTree.sectionPlaceholder);
    if (!title) return;
    try {
      const path = kind === "page" ? await api.createPage({ section: section.path, title, bundle: true, language: editing }) : await api.createSection(section.path, title);
      setOpen((o) => new Set(o).add(section.path).add(path));
      onChanged(kind === "page" ? path : undefined);
    } catch (e) { error(e); }
  };

  const onDragEnd = async (e: DragEndEvent) => {
    setDragging(null);
    const node = e.active.data.current?.node as TreeNode | undefined;
    const target = e.over?.id as string | undefined;
    if (!node || !target) return;
    try {
      if (target.startsWith("into:")) {
        const section = target.slice(5);
        if (parentOf(tree, node.path)?.path === section) return;
        const np = await api.movePage(node.path, section);
        setOpen((o) => new Set(o).add(section));
        onChanged(selected === node.path ? np : undefined);
      } else if (target.startsWith("before:") || target.startsWith("end:")) {
        const [kind, ...rest] = target.split(":");
        const ref = rest.join(":");
        const parent = kind === "end" ? findSection(tree, ref)! : parentOf(tree, ref)!;
        let path = node.path;
        if (parentOf(tree, node.path)?.path !== parent.path) path = await api.movePage(node.path, parent.path);
        const siblings = (parent.children ?? []).map((c) => c.path).filter((p) => p !== node.path);
        const at = kind === "end" ? siblings.length : siblings.indexOf(ref);
        siblings.splice(at < 0 ? siblings.length : at, 0, path);
        await api.reorderPages(siblings);
        onChanged(selected === node.path ? path : undefined);
      }
    } catch (err) { error(err, t.pageTree.couldntMove); }
  };

  const rows = (nodes: TreeNode[], depth: number): React.ReactNode => nodes.map((n, i) => (
    <div key={n.path}>
      <Gap id={"before:" + n.path} depth={depth} active={!!dragging && dragging.path !== n.path} />
      <Row
        node={n}
        depth={depth}
        label={n.kind === "section" ? labelFor(n) : n.title}
        selected={selected === n.path || selected === n.indexFile || !!n.translations?.some((x) => x.path === selected)}
        multilingual={multilingual}
        expanded={open.has(n.path) || !!query}
        changed={changed.has(n.path) || (!!n.indexFile && changed.has(n.indexFile)) || hasChangedChild(n, changed)}
        onToggle={() => toggle(n.path)}
        onOpen={onOpen}
        onActivate={() => (n.kind === "page" ? onOpen(n.path) : n.indexFile ? onOpen(n.indexFile) : toggle(n.path))}
        menu={rowMenu(n, { newIn, onChanged, onOpen, error })}
        dropInto={n.kind === "section" && !!dragging && dragging.path !== n.path}
      />
      {n.kind === "section" && (open.has(n.path) || query) && (
        <div>
          {n.children?.length ? rows(n.children, depth + 1) : <div className="dim small" style={{ padding: "4px 0 6px", paddingLeft: 34 + (depth + 1) * 16 }}>{t.pageTree.empty((text) => <a href="#" onClick={(e) => { e.preventDefault(); newIn(n, "page"); }}>{text}</a>)}</div>}
          <Gap id={"end:" + n.path} depth={depth + 1} active={!!dragging} />
        </div>
      )}
      {i === nodes.length - 1 && depth === 0 && <Gap id={"end:" + tree.path} depth={0} active={!!dragging} />}
    </div>
  ));

  return (
    <div className="tree-pane">
      <div className="tree-head">
        <div className="tree-search">
          <Search size={15} />
          <input className="input" placeholder={t.pageTree.findPage} value={query} onChange={(e) => setQuery(e.target.value)} aria-label={t.pageTree.findPage} />
        </div>
        <Menu
          trigger={<button className="btn primary icon" aria-label={t.pageTree.add}><Plus size={17} /></button>}
          items={[
            { label: t.pageTree.newPage, icon: <FilePlus2 size={14} />, run: () => newIn(tree, "page") },
            { label: t.pageTree.newSection, icon: <FolderPlus size={14} />, run: () => newIn(tree, "section") },
          ]}
        />
      </div>
      <DndContext sensors={sensors} onDragStart={(e) => setDragging(e.active.data.current?.node ?? null)} onDragEnd={onDragEnd} onDragCancel={() => setDragging(null)}>
        <div className="tree" role="tree" aria-label={t.pageTree.pages}>
          <Row node={tree} depth={0} label={t.pageTree.homePage} selected={selected === tree.indexFile} multilingual={false} expanded changed={!!tree.indexFile && changed.has(tree.indexFile)} isRoot
            onToggle={() => {}} onOpen={onOpen} onActivate={() => tree.indexFile && onOpen(tree.indexFile)} menu={rowMenu(tree, { newIn, onChanged, onOpen, error })} dropInto={!!dragging} />
          {rows(visible.children ?? [], 0)}
          {!visible.children?.length && <div className="empty small">{t.pageTree.noMatch(query)}</div>}
        </div>
        <DragOverlay>{dragging && <div className="tree-row selected" style={{ boxShadow: "var(--shadow)", background: "var(--surface)" }}><FileText size={15} /> {dragging.title}</div>}</DragOverlay>
      </DndContext>
      <div className="dim small" style={{ padding: "8px 14px", borderTop: "1px solid var(--line)" }}>{t.pageTree.dragHint}</div>
    </div>
  );
}

function Row(p: {
  node: TreeNode; depth: number; label: string; selected: boolean; expanded: boolean; changed: boolean; isRoot?: boolean; multilingual: boolean;
  onToggle: () => void; onActivate: () => void; onOpen: (path: string) => void; menu: React.ComponentProps<typeof Menu>["items"]; dropInto: boolean;
}) {
  const { node } = p;
  const drag = useDraggable({ id: "node:" + node.path, data: { node }, disabled: p.isRoot });
  const drop = useDroppable({ id: "into:" + node.path, disabled: node.kind !== "section" || !p.dropInto });
  const isSection = node.kind === "section";
  return (
    <div
      ref={(el) => { drag.setNodeRef(el); drop.setNodeRef(el); }}
      className={"tree-row" + (isSection ? " section" : "") + (p.selected ? " selected" : "") + (drag.isDragging ? " dragging" : "") + (drop.isOver ? " drop-into" : "")}
      style={{ paddingLeft: 6 + p.depth * 16 }}
      role="treeitem"
      aria-selected={p.selected}
      aria-expanded={isSection ? p.expanded : undefined}
      onClick={p.onActivate}
    >
      {!p.isRoot && <span className="grip" {...drag.listeners} {...drag.attributes} onClick={(e) => e.stopPropagation()}><GripVertical size={14} /></span>}
      {isSection && !p.isRoot ? (
        <button className="chev" onClick={(e) => { e.stopPropagation(); p.onToggle(); }} aria-label={p.expanded ? t.pageTree.collapse : t.pageTree.expand}>
          {p.expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </button>
      ) : (
        <span style={{ width: 18 }} />
      )}
      <span className="ticon">{p.isRoot ? <FileText size={15} /> : isSection ? (p.expanded ? <FolderOpen size={15} /> : <Folder size={15} />) : node.error ? <AlertTriangle size={15} color="var(--warning)" /> : <FileText size={15} />}</span>
      <span className="tname">{p.label}</span>
      {p.multilingual && !p.isRoot && (
        <span className="lang-tags" onClick={(e) => e.stopPropagation()}>
          {node.translations?.map((translation) => (
            <button key={translation.language} className="pill" aria-label={t.pageTree.openLanguage(translation.language)} onClick={() => p.onOpen(translation.path)}>{translation.language}</button>
          ))}
        </span>
      )}
      {p.changed && <span className="changed-dot" title={t.pageTree.unpublished} />}
      {node.kind === "page" && node.draft && <span className="pill draft">{t.common.draft}</span>}
      <span className="actions" onClick={(e) => e.stopPropagation()}>
        <Menu trigger={<button className="btn ghost icon sm" aria-label={t.pageTree.pageOptions}><MoreHorizontal size={15} /></button>} items={p.menu} />
      </span>
    </div>
  );
}

function Gap({ id, depth, active }: { id: string; depth: number; active: boolean }) {
  const { setNodeRef, isOver } = useDroppable({ id, disabled: !active });
  return (
    <div ref={setNodeRef} style={{ height: active ? 6 : 0, marginLeft: 6 + depth * 16, position: "relative" }}>
      {isOver && <div className="drop-line" style={{ position: "absolute", top: 2, left: 0, right: 8 }} />}
    </div>
  );
}

function rowMenu(n: TreeNode, h: { newIn: (s: TreeNode, k: "page" | "section") => void; onChanged: (p?: string) => void; onOpen: (p: string) => void; error: (e: unknown, t?: string) => void }) {
  const items: React.ComponentProps<typeof Menu>["items"] = [];
  if (n.kind === "section") {
    items.push({ label: t.pageTree.newPageHere, icon: <FilePlus2 size={14} />, run: () => h.newIn(n, "page") });
    items.push({ label: t.pageTree.newSectionHere, icon: <FolderPlus size={14} />, run: () => h.newIn(n, "section") });
    if (n.indexFile) items.push({ label: t.pageTree.editIntro, icon: <Pencil size={14} />, run: () => h.onOpen(n.indexFile!) });
  } else {
    items.push({ label: t.pageTree.edit, icon: <Pencil size={14} />, run: () => h.onOpen(n.path) });
    items.push({
      label: t.pageTree.changeAddress, icon: <Link2 size={14} />,
      run: async () => {
        const cur = n.url.replace(/\/$/, "").split("/").pop() ?? "";
        const slug = await ask.prompt(t.pageTree.changeAddressTitle, t.pageTree.newAddressOf(n.url), "my-page", cur);
        if (!slug || slug === cur) return;
        try { h.onChanged(await api.renameSlug(n.path, slug)); } catch (e) { h.error(e); }
      },
    });
  }
  if (n.path !== n.path.split("/")[0]) {
    items.push({ separator: true });
    items.push({
      label: n.kind === "section" ? t.pageTree.deleteSection : t.pageTree.deletePage, icon: <Trash2 size={14} />, danger: true,
      run: async () => {
        const count = n.kind === "section" ? countAll(n) : 0;
        const ok = await ask.confirm(
          t.pageTree.deleteTitle(n.title),
          n.kind === "section" ? t.pageTree.deleteSectionBody(count) : t.pageTree.deletePageBody,
          t.common.delete, true,
        );
        if (!ok) return;
        try { await api.deletePage(n.kind === "section" ? n.path : n.path); h.onChanged(); } catch (e) { h.error(e); }
      },
    });
  }
  if (n.kind === "page") items.splice(2, 0, { label: t.pageTree.moveToSection, icon: <FolderInput size={14} />, run: () => window.dispatchEvent(new CustomEvent("pageroom:move", { detail: n.path })) });
  return items;
}

function countAll(n: TreeNode): number {
  return (n.children ?? []).reduce((a, c) => a + (c.kind === "page" ? 1 : countAll(c)), 0);
}

function hasChangedChild(n: TreeNode, changed: Set<string>): boolean {
  if (n.kind !== "section" || changed.size === 0) return false;
  for (const p of changed) if (p.startsWith(n.path + "/")) return true;
  return false;
}

function findSection(n: TreeNode, path: string): TreeNode | undefined {
  if (n.path === path) return n;
  for (const c of n.children ?? []) {
    const f = findSection(c, path);
    if (f) return f;
  }
  return undefined;
}

function filterTree(n: TreeNode, q: string): TreeNode {
  const kids = (n.children ?? [])
    .map((c) => (c.kind === "section" ? filterTree(c, q) : c))
    .filter((c) => c.title.toLowerCase().includes(q) || (c.kind === "section" && (c.children?.length ?? 0) > 0));
  return { ...n, children: kids };
}
