import { useEffect, useMemo, useState } from "react";
import { Copy, FileText, ImagePlus, Search, Trash2, Upload } from "lucide-react";
import { api, on, siteFileURL, translate, type MediaItem } from "../api";
import { useStore, findNode } from "../store";
import { ask, bytes, Dialog, formatDate, Spinner } from "../ui";
import { t } from "../i18n";

export function MediaView() {
  const { site, go, error, toast, refreshChanges } = useStore();
  const [items, setItems] = useState<MediaItem[] | null>(null);
  const [q, setQ] = useState("");
  const [sel, setSel] = useState<MediaItem | null>(null);
  const [usage, setUsage] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);

  const load = () => api.listMedia().then((l) => setItems(l ?? [])).catch(error);
  useEffect(() => { load(); }, []);
  useEffect(() => { if (sel) api.mediaUsage(sel.path).then((u) => setUsage(u ?? [])).catch(() => setUsage([])); }, [sel]);

  const importPaths = async (paths: string[]) => {
    if (!paths.length) return;
    setBusy(true);
    try {
      const out = await api.importMedia(paths, "");
      const n = out.imported?.length ?? 0;
      if (n) toast({ kind: "success", title: t.media.added(n), body: out.imported!.some((r) => r.stripped || r.resized) ? t.media.optimized : undefined });
      out.failed?.forEach((f) => error(translate({ code: f.code ?? "invalid", message: f.message }), t.pageEditor.couldntAdd(f.name)));
      await load();
      refreshChanges();
    } catch (e) { error(e); } finally { setBusy(false); }
  };
  useEffect(() => on("files:dropped", (paths: string[]) => importPaths(paths)), []);

  const shown = useMemo(() => (items ?? []).filter((m) => !q || m.name.toLowerCase().includes(q.toLowerCase())), [items, q]);
  const pageTitle = (p?: string) => (p && site ? findNode(site.tree, p)?.title ?? p : "");

  return (
    <div className="page-pad" style={{ maxWidth: "none" }}>
      <div className="row" style={{ marginBottom: 18 }}>
        <div style={{ flex: 1 }}>
          <h1 className="page-title">{t.media.title}</h1>
          <p className="page-sub" style={{ margin: 0 }}>{t.media.subtitle}</p>
        </div>
        <div className="tree-search" style={{ width: 240, flex: "none" }}>
          <Search size={15} />
          <input className="input" placeholder={t.media.search} value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        <button className="btn primary" onClick={async () => importPaths((await api.pickImages()) ?? [])}>{busy ? <Spinner /> : <Upload size={15} />} {t.media.upload}</button>
      </div>

      {!items ? <Spinner /> : items.length === 0 ? (
        <div className="dropzone" style={{ padding: 60 }}>
          <div className="empty" style={{ padding: 0 }}>
            <div className="art"><ImagePlus size={28} /></div>
            <h3>{t.media.none}</h3>
            <p>{t.media.noneHint}</p>
          </div>
        </div>
      ) : (
        <div className="media-grid">
          {shown.map((m) => (
            <div key={m.path} className="media-tile" onClick={() => setSel(m)}>
              <div className="mt-img"><img src={siteFileURL(m.path, m.modified)} alt="" loading="lazy" /></div>
              <div className="mt-name" title={m.path}>{m.name}</div>
            </div>
          ))}
        </div>
      )}

      {sel && (
        <Dialog open wide onClose={() => setSel(null)} title={sel.name}>
          <div className="media-detail">
            <div className="preview"><img src={siteFileURL(sel.path, sel.modified)} alt="" /></div>
            <div>
              <dl className="kv">
                <dt>{t.media.size}</dt><dd>{sel.width && sel.height ? t.media.pixels(sel.width, sel.height) : ""}{bytes(sel.size)}</dd>
                <dt>{t.media.addedOn}</dt><dd>{formatDate(sel.modified)}</dd>
                <dt>{t.media.location}</dt><dd className="mono small" title={sel.path}>{sel.path}</dd>
                {sel.page && <><dt>{t.media.belongsTo}</dt><dd>{pageTitle(sel.page)}</dd></>}
              </dl>
              <div className="field" style={{ marginTop: 16 }}>
                <label>{t.media.useInMarkdown}</label>
                <div className="row">
                  <input className="input mono" readOnly value={sel.ref} onFocus={(e) => e.target.select()} />
                  <button className="btn icon" aria-label={t.common.copy} onClick={() => { navigator.clipboard?.writeText(sel.ref); toast({ kind: "success", title: t.common.copied }); }}><Copy size={14} /></button>
                </div>
              </div>
              <div className="label" style={{ marginBottom: 6 }}>{t.media.usedOn}</div>
              {usage.length === 0 ? <div className="dim small">{t.media.notUsed}</div> : (
                <div className="card list">
                  {usage.map((u) => (
                    <div key={u} className="list-item clickable" onClick={() => { setSel(null); go({ kind: "pages", path: u }); }}>
                      <FileText size={14} className="dim" /> <span className="truncate">{pageTitle(u)}</span>
                    </div>
                  ))}
                </div>
              )}
              <div className="row" style={{ marginTop: 20 }}>
                <button className="btn danger" onClick={async () => {
                  const body = usage.length ? t.media.deleteUsed(usage.length) : t.media.deleteUnused;
                  if (!(await ask.confirm(t.media.deleteTitle(sel.name), body, t.common.delete, true))) return;
                  try { await api.deleteMedia(sel.path, true); setSel(null); await load(); refreshChanges(); } catch (e) { error(e); }
                }}><Trash2 size={14} /> {t.common.delete}</button>
              </div>
            </div>
          </div>
        </Dialog>
      )}
    </div>
  );
}
