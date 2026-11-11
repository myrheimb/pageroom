import { useEffect, useMemo, useState } from "react";
import { DndContext, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from "@dnd-kit/core";
import { SortableContext, arrayMove, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { ExternalLink, FileText, GripVertical, IndentDecrease, IndentIncrease, Link2, Plus, Save, Trash2, Pencil } from "lucide-react";
import { api, type Menu, type MenuItem } from "../api";
import { flatPages, useStore } from "../store";
import { ask, Banner, Dialog, Spinner } from "../ui";
import { t } from "../i18n";

export function MenusView() {
  const { site, error, toast, refreshChanges } = useStore();
  const [menus, setMenus] = useState<Menu[] | null>(null);
  const [current, setCurrent] = useState("");
  const [language, setLanguage] = useState("");
  const [items, setItems] = useState<MenuItem[]>([]);
  const [dirty, setDirty] = useState(false);
  const [adding, setAdding] = useState(false);
  const [saving, setSaving] = useState(false);
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }));

  const load = async () => {
    try {
      const m = await api.getMenus();
      setMenus(m);
      const lang = language || site?.info.languages?.[0] || m[0]?.language || "";
      const inLanguage = m.filter((x) => x.language === lang);
      const cur = inLanguage.find((x) => x.name === current) ?? inLanguage[0];
      setLanguage(lang);
      if (cur) { setCurrent(cur.name); setItems(withIds(cur.items ?? [])); }
      setDirty(false);
    } catch (e) { error(e); }
  };
  useEffect(() => { load(); }, []);

  const pick = (name: string, lang = language) => {
    const m = menus?.find((x) => x.language === lang && x.name === name);
    setCurrent(name);
    setItems(withIds(m?.items ?? []));
    setDirty(false);
  };
  const pickLanguage = (lang: string) => {
    setLanguage(lang);
    pick(menus?.find((x) => x.language === lang)?.name ?? "main", lang);
  };
  const change = (next: MenuItem[]) => { setItems(next); setDirty(true); };

  const onDragEnd = (e: DragEndEvent) => {
    if (!e.over || e.active.id === e.over.id) return;
    const from = items.findIndex((i) => i.id === e.active.id);
    const to = items.findIndex((i) => i.id === e.over!.id);
    change(arrayMove(items, from, to));
  };
  const indent = (i: number, dir: 1 | -1) => {
    const next = [...items];
    if (dir === 1) {
      const parent = [...next.slice(0, i)].reverse().find((x) => !x.parent);
      if (!parent) return;
      next[i] = { ...next[i], parent: parent.id };
    } else next[i] = { ...next[i], parent: undefined };
    change(next);
  };

  const save = async () => {
    setSaving(true);
    try {
      await api.saveMenu({ name: current, language, items });
      toast({ kind: "success", title: t.menus.saved, body: t.common.publishToUpdate });
      await load();
      refreshChanges();
    } catch (e) { error(e, t.menus.couldntSave); } finally { setSaving(false); }
  };

  const pages = useMemo(() => (site ? flatPages(site.tree) : []), [site]);
  const sectionsList = useMemo(() => (site?.tree.children ?? []).filter((c) => c.kind === "section"), [site]);

  if (!menus) return <div className="page-pad"><Spinner /></div>;
  const languages = site?.info.languages ?? [];
  const shown = menus.filter((m) => m.language === language);
  return (
    <div className="page-pad" style={{ maxWidth: 820 }}>
      <h1 className="page-title">{t.menus.title}</h1>
      <p className="page-sub">{t.menus.subtitle}</p>

      {languages.length > 1 && (
        <div className="seg" role="group" aria-label={t.menus.language} style={{ marginBottom: 12 }}>
          {languages.map((code) => <button key={code} className={code === language ? "on" : ""} onClick={() => pickLanguage(code)}>{code}</button>)}
        </div>
      )}
      <div className="row" style={{ marginBottom: 16 }}>
        <div className="seg">
          {shown.map((m) => <button key={m.name} className={m.name === current ? "on" : ""} onClick={() => pick(m.name)}>{label(m.name)}</button>)}
        </div>
        <button className="btn sm ghost" onClick={async () => {
          const n = await ask.prompt(t.menus.newMenu, t.menus.menuName, "footer");
          if (n) { const id = n.toLowerCase().replace(/[^a-z0-9]+/g, "-"); setMenus([...menus, { name: id, language, items: [] }]); setCurrent(id); setItems([]); setDirty(true); }
        }}><Plus size={13} /> {t.menus.newMenu}</button>
        <div className="spacer" />
        <button className="btn" onClick={() => setAdding(true)}><Plus size={15} /> {t.menus.addItem}</button>
        <button className="btn primary" disabled={!dirty || saving} onClick={save}>{saving ? <Spinner /> : <Save size={15} />} {t.menus.saveMenu}</button>
      </div>

      {current !== shown[0]?.name && <Banner kind="info"><span className="small">{t.menus.themeDecides(label(current))}</span></Banner>}

      {items.length === 0 ? (
        <div className="card empty"><h3>{t.menus.empty}</h3><p>{t.menus.emptyHint}</p><button className="btn primary" onClick={() => setAdding(true)}><Plus size={15} /> {t.menus.addItem}</button></div>
      ) : (
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
          <SortableContext items={items.map((i) => i.id)} strategy={verticalListSortingStrategy}>
            {items.map((it, i) => (
              <SortableRow
                key={it.id}
                item={it}
                pageTitle={pages.find((p) => refOf(site!.tree.path, p.path) === it.pageRef)?.title}
                onRename={async () => { const n = await ask.prompt(t.menus.renameTitle, t.menus.label, "", it.name); if (n) change(items.map((x) => (x.id === it.id ? { ...x, name: n } : x))); }}
                onRemove={() => change(items.filter((x) => x.id !== it.id).map((x) => (x.parent === it.id ? { ...x, parent: undefined } : x)))}
                onIndent={(d) => indent(i, d)}
                canIndent={i > 0 && !it.parent}
              />
            ))}
          </SortableContext>
        </DndContext>
      )}

      {adding && (
        <AddItemDialog
          pages={pages.map((p) => ({ title: p.title, ref: refOf(site!.tree.path, p.path) }))}
          sections={sectionsList.map((s) => ({ title: s.title, ref: "/" + s.path.split("/").slice(1).join("/") }))}
          onClose={() => setAdding(false)}
          onAdd={(it) => { change([...items, { ...it, id: "new-" + Math.random().toString(36).slice(2, 8), weight: 0, source: "config" }]); setAdding(false); }}
        />
      )}
    </div>
  );
}

function SortableRow({ item, pageTitle, onRename, onRemove, onIndent, canIndent }: { item: MenuItem; pageTitle?: string; onRename: () => void; onRemove: () => void; onIndent: (d: 1 | -1) => void; canIndent: boolean }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: item.id });
  return (
    <div ref={setNodeRef} className={"menu-row" + (item.parent ? " child" : "")} style={{ transform: transform ? `translate3d(0, ${transform.y}px, 0)` : undefined, transition, opacity: isDragging ? 0.6 : 1 }}>
      <span className="grip" {...listeners} {...attributes}><GripVertical size={16} /></span>
      {item.url ? <ExternalLink size={15} className="dim" /> : <FileText size={15} className="dim" />}
      <span className="mr-name">{item.name}</span>
      <span className="mr-target">{item.url ?? (pageTitle ? t.menus.pageTarget(pageTitle) : item.pageRef)}</span>
      {item.parent ? (
        <button className="btn ghost icon sm" title={t.menus.moveOut} onClick={() => onIndent(-1)}><IndentDecrease size={14} /></button>
      ) : (
        <button className="btn ghost icon sm" title={t.menus.putInside} disabled={!canIndent} onClick={() => onIndent(1)}><IndentIncrease size={14} /></button>
      )}
      <button className="btn ghost icon sm" title={t.menus.rename} onClick={onRename}><Pencil size={14} /></button>
      <button className="btn ghost icon sm danger" title={t.menus.removeFromMenu} onClick={onRemove}><Trash2 size={14} /></button>
    </div>
  );
}

function AddItemDialog({ pages, sections, onAdd, onClose }: { pages: { title: string; ref: string }[]; sections: { title: string; ref: string }[]; onAdd: (i: Omit<MenuItem, "id" | "weight" | "source">) => void; onClose: () => void }) {
  const [tab, setTab] = useState<"page" | "link">("page");
  const [q, setQ] = useState("");
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const all = [{ title: t.menus.home, ref: "/" }, ...sections, ...pages];
  const shown = all.filter((p) => p.title.toLowerCase().includes(q.toLowerCase()));
  return (
    <Dialog open onClose={onClose} title={t.menus.addTitle}>
      <div className="seg" style={{ marginBottom: 14 }}>
        <button className={tab === "page" ? "on" : ""} onClick={() => setTab("page")}><FileText size={13} /> {t.menus.aPage}</button>
        <button className={tab === "link" ? "on" : ""} onClick={() => setTab("link")}><Link2 size={13} /> {t.menus.aLink}</button>
      </div>
      {tab === "page" ? (
        <>
          <input className="input" autoFocus placeholder={t.menus.findPage} value={q} onChange={(e) => setQ(e.target.value)} style={{ marginBottom: 10 }} />
          <div className="card list" style={{ maxHeight: 320, overflow: "auto" }}>
            {shown.map((p) => (
              <div key={p.ref} className="list-item clickable" onClick={() => onAdd({ name: p.title, pageRef: p.ref })}>
                <FileText size={14} className="dim" /> <span style={{ flex: 1 }}>{p.title}</span> <span className="mono dim small">{p.ref}</span>
              </div>
            ))}
          </div>
        </>
      ) : (
        <>
          <div className="field"><label htmlFor="ml-name">{t.menus.label}</label><input id="ml-name" className="input" autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="Instagram" /></div>
          <div className="field"><label htmlFor="ml-url">{t.menus.webAddress}</label><input id="ml-url" className="input" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://instagram.com/…" /></div>
          <div className="dialog-actions"><button className="btn primary" disabled={!name.trim() || !url.trim()} onClick={() => onAdd({ name: name.trim(), url: /^(https?:|mailto:|\/)/.test(url.trim()) ? url.trim() : "https://" + url.trim() })}>{t.menus.addLink}</button></div>
        </>
      )}
    </Dialog>
  );
}

function withIds(items: MenuItem[]): MenuItem[] {
  return items.map((i, n) => ({ ...i, id: i.id || `item-${n}` }));
}

function refOf(contentDir: string, path: string): string {
  const r = path.slice(contentDir.length + 1);
  const parts = r.split("/");
  const last = parts.pop() ?? "";
  const stem = last.replace(/\.[^.]+$/, "");
  if (stem === "index" || stem === "_index") return "/" + parts.join("/");
  return "/" + [...parts, stem].join("/");
}

function label(name: string): string {
  return name === "main" ? t.menus.mainMenu : t.menus.namedMenu(name);
}
