import { useEffect, useState } from "react";
import { History as HistoryIcon, RotateCcw } from "lucide-react";
import { api, type Revision } from "../api";
import { markdownToHtml, stripUnsafeUrls } from "../editor/markdown";
import { useStore } from "../store";
import { ask, Dialog, Spinner, timeAgo } from "../ui";
import { language, t } from "../i18n";

export function HistoryDialog({ path, onClose, onRestored }: { path: string; onClose: () => void; onRestored: () => void }) {
  const [revs, setRevs] = useState<Revision[] | null>(null);
  const [sel, setSel] = useState<Revision | null>(null);
  const [content, setContent] = useState<string>("");
  const error = useStore((s) => s.error);

  useEffect(() => {
    api.history(path).then((r) => { setRevs(r ?? []); if (r?.length) setSel(r[0]); }).catch((e) => { setRevs([]); if (e.code !== "no-repo") error(e); });
  }, [path]);
  useEffect(() => {
    if (!sel) return;
    api.revisionContent(path, sel.commit).then(setContent).catch(error);
  }, [sel]);

  const body = content.replace(/^(---|\+\+\+)[\s\S]*?\n\1\s*\n/, "");
  return (
    <Dialog open wide onClose={onClose} title={t.history.title} description={t.history.description}>
      {!revs ? <Spinner /> : revs.length === 0 ? (
        <div className="empty"><div className="art"><HistoryIcon size={26} /></div><h3>{t.history.none}</h3><p>{t.history.noneHint}</p></div>
      ) : (
        <div style={{ display: "grid", gridTemplateColumns: "240px 1fr", gap: 16, minHeight: 360 }}>
          <div className="card list" style={{ maxHeight: 420, overflow: "auto" }}>
            {revs.map((r, i) => (
              <div key={r.commit} className={"list-item clickable"} style={sel?.commit === r.commit ? { background: "var(--accent-soft)" } : undefined} onClick={() => setSel(r)}>
                <div style={{ minWidth: 0 }}>
                  <div style={{ fontWeight: 600 }}>{i === 0 ? t.history.latest : timeAgo(r.when)}</div>
                  <div className="dim small truncate">{r.author} · {new Date(r.when).toLocaleString(language)}</div>
                  <div className="dim small truncate">{r.message}</div>
                </div>
              </div>
            ))}
          </div>
          <div className="card card-pad" style={{ maxHeight: 420, overflow: "auto" }}>
            <div className="prose-editor selectable" style={{ fontSize: 15 }} dangerouslySetInnerHTML={{ __html: sanitize(markdownToHtml(body)) }} />
          </div>
        </div>
      )}
      {sel && revs && revs.length > 0 && (
        <div className="dialog-actions">
          <button className="btn" onClick={onClose}>{t.common.close}</button>
          <button className="btn primary" onClick={async () => {
            if (!(await ask.confirm(t.history.restoreTitle, t.history.restoreBody, t.history.restore))) return;
            try { await api.restoreRevision(path, sel.commit); onRestored(); } catch (e) { error(e); }
          }}><RotateCcw size={15} /> {t.history.restoreThis}</button>
        </div>
      )}
    </Dialog>
  );
}

/** Strip anything executable from preview HTML (old versions may contain raw HTML). */
function sanitize(html: string): string {
  const doc = new DOMParser().parseFromString(html, "text/html");
  doc.querySelectorAll("script, iframe, object, embed, style, link").forEach((n) => n.remove());
  doc.querySelectorAll("*").forEach((el) => {
    for (const a of Array.from(el.attributes)) {
      if (/^on/i.test(a.name)) el.removeAttribute(a.name);
    }
  });
  stripUnsafeUrls(doc);
  doc.querySelectorAll("img").forEach((img) => img.removeAttribute("src"));
  return doc.body.innerHTML;
}
