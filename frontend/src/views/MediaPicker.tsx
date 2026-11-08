import { useEffect, useState } from "react";
import { Upload } from "lucide-react";
import { api, on, siteFileURL, translate, type MediaItem } from "../api";
import { useStore } from "../store";
import { Dialog, Spinner } from "../ui";
import { t } from "../i18n";

export function MediaPicker({ page, onPick, onClose, refFor }: { page: string; onPick: (r: { ref: string; alt?: string }) => void; onClose: () => void; refFor: (m: MediaItem) => string }) {
  const [items, setItems] = useState<MediaItem[] | null>(null);
  const [sel, setSel] = useState<MediaItem | null>(null);
  const [alt, setAlt] = useState("");
  const [busy, setBusy] = useState(false);
  const error = useStore((s) => s.error);
  const pageDir = page.slice(0, page.lastIndexOf("/"));

  const load = () => api.listMedia().then((l) => setItems(sortForPage(l ?? [], page))).catch(error);
  useEffect(() => { load(); }, []);

  const importPaths = async (paths: string[]) => {
    if (!paths.length) return;
    setBusy(true);
    try {
      const out = await api.importMedia(paths, page);
      out.failed?.forEach((f) => error(translate({ code: f.code ?? "invalid", message: f.message }), t.pageEditor.couldntAdd(f.name)));
      await load();
      const first = out.imported?.[0];
      if (first) setSel(first);
    } catch (e) { error(e); } finally { setBusy(false); }
  };
  useEffect(() => on("files:dropped", (paths: string[]) => importPaths(paths)), [page]);

  return (
    <Dialog
      open
      wide
      onClose={onClose}
      title={t.mediaPicker.title}
      actions={
        <>
          <button className="btn" onClick={onClose}>{t.common.cancel}</button>
          <button className="btn primary" disabled={!sel} onClick={() => sel && onPick({ ref: refFor(sel), alt })}>{t.mediaPicker.useThis}</button>
        </>
      }
    >
      <div className="dropzone" style={{ marginBottom: 14 }}>
        <div className="row" style={{ justifyContent: "center" }}>
          <Upload size={18} /> {t.mediaPicker.dragHere}
          <button className="btn sm" onClick={async () => importPaths((await api.pickImages()) ?? [])}>{busy ? <Spinner size={13} /> : null} {t.mediaPicker.uploadFromComputer}</button>
        </div>
        <div className="dim small" style={{ marginTop: 6 }}>{t.mediaPicker.resizedNote}</div>
      </div>
      {!items ? <Spinner /> : (
        <div className="media-grid" style={{ maxHeight: 360, overflow: "auto", padding: 2 }}>
          {items.map((m) => (
            <div key={m.path} className={"media-tile" + (sel?.path === m.path ? " selected" : "")} onClick={() => setSel(m)} onDoubleClick={() => onPick({ ref: refFor(m), alt })}>
              <div className="mt-img"><img src={siteFileURL(m.path, m.modified)} alt="" loading="lazy" /></div>
              <div className="mt-name">{m.name}{m.path.startsWith(pageDir + "/") ? <span className="pill accent" style={{ marginLeft: 4 }}>{t.mediaPicker.thisPage}</span> : null}</div>
            </div>
          ))}
          {items.length === 0 && <div className="dim">{t.mediaPicker.none}</div>}
        </div>
      )}
      {sel && (
        <div className="field" style={{ marginTop: 14, marginBottom: 0 }}>
          <label htmlFor="pick-alt">{t.mediaPicker.describe}</label>
          <input id="pick-alt" className="input" value={alt} onChange={(e) => setAlt(e.target.value)} placeholder={t.mediaPicker.describePlaceholder} />
          <span className="help">{t.mediaPicker.describeHelp}</span>
        </div>
      )}
    </Dialog>
  );
}

function sortForPage(items: MediaItem[], page: string): MediaItem[] {
  const dir = page.slice(0, page.lastIndexOf("/") + 1);
  return [...items].sort((a, b) => Number(b.path.startsWith(dir)) - Number(a.path.startsWith(dir)));
}
