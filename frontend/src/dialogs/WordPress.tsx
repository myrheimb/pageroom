import { useEffect, useState } from "react";
import { AlertTriangle, CheckCircle2, FileUp, Image, MessageSquareOff } from "lucide-react";
import { api, on, type AppError, type SiteState, type WPReport, type WPSummary } from "../api";
import { Banner, Dialog } from "../ui";
import { t } from "../i18n";

type Step = { kind: "intro" } | { kind: "summary"; path: string; summary: WPSummary } | { kind: "working" } | { kind: "done"; report: WPReport };

/**
 * Bring a WordPress site over. Mode "new" creates a site from the export.
 * Mode "existing" adds the content to the open site.
 */
export function WordPressImport({ mode, onClose, onSite, onImported }: {
  mode: "new" | "existing";
  onClose: () => void;
  onSite?: (s: SiteState, r: WPReport) => void;
  onImported?: (r: WPReport) => void;
}) {
  const [step, setStep] = useState<Step>({ kind: "intro" });
  const [images, setImages] = useState(true);
  const [err, setErr] = useState<AppError | null>(null);
  const [progress, setProgress] = useState<{ done: number; total: number; label: string } | null>(null);
  useEffect(() => on("wp:progress", setProgress), []);

  const choose = async () => {
    setErr(null);
    try {
      const path = await api.pickWordPressFile();
      if (!path) return;
      setStep({ kind: "summary", path, summary: await api.peekWordPress(path) });
    } catch (e) { setErr(e as AppError); }
  };

  const run = async (path: string) => {
    setStep({ kind: "working" });
    try {
      if (mode === "new") {
        const r = await api.createSiteFromWordPress(path, images);
        onSite?.(r.site, r.report);
      } else {
        const r = await api.importWordPress(path, images);
        onImported?.(r);
        setStep({ kind: "done", report: r });
      }
    } catch (e) {
      setErr(e as AppError);
      setStep({ kind: "intro" });
    }
  };

  if (step.kind === "working") {
    return (
      <Dialog open dismissable={false} onClose={() => {}} title={t.wordpress.bringing}>
        <p className="muted">{progress?.label ? t.wordpress.now(progress.label) : progress?.total && progress.done === progress.total ? t.wordpress.done : t.wordpress.reading}</p>
        <div className="progress"><div style={{ width: `${progress && progress.total ? (progress.done / progress.total) * 100 : 3}%` }} /></div>
        {images && <p className="dim small">{t.wordpress.imagesTakeTime}</p>}
      </Dialog>
    );
  }
  if (step.kind === "done") return <ImportReport report={step.report} onClose={onClose} />;

  if (step.kind === "summary") {
    const s = step.summary;
    return (
      <Dialog
        open
        onClose={onClose}
        title={s.title ? t.wordpress.importTitle(s.title) : t.wordpress.importUntitled}
        description={s.link}
        actions={
          <>
            <button className="btn" onClick={onClose}>{t.common.cancel}</button>
            <button className="btn primary" onClick={() => run(step.path)}><FileUp size={15} /> {mode === "new" ? t.wordpress.createAndImport : t.wordpress.import}</button>
          </>
        }
      >
        <div className="card card-pad row" style={{ gap: 28, marginBottom: 14 }}>
          <div><div className="stat">{s.posts}</div><div className="stat-label">{t.wordpress.posts}</div></div>
          <div><div className="stat">{s.pages}</div><div className="stat-label">{t.wordpress.pages}</div></div>
          <div><div className="stat">{s.images}</div><div className="stat-label">{t.wordpress.images}</div></div>
          <div><div className="stat">{s.menuItems}</div><div className="stat-label">{t.wordpress.menuLinks}</div></div>
        </div>
        <label className="row" style={{ cursor: "pointer", marginBottom: 12 }}>
          <input type="checkbox" checked={images} onChange={(e) => setImages(e.target.checked)} />
          <span><Image size={13} /> {t.wordpress.downloadImages} <span className="dim">{t.wordpress.mustBeOnline}</span></span>
        </label>
        <ul className="muted small" style={{ paddingLeft: 18, lineHeight: 1.7, margin: 0 }}>
          {s.drafts > 0 && <li>{t.wordpress.drafts(s.drafts)}</li>}
          <li>{t.wordpress.oldAddresses}</li>
          <li>{t.wordpress.nothingLive}</li>
          {s.comments > 0 && <li><MessageSquareOff size={12} /> {t.wordpress.comments(s.comments)}</li>}
          {s.skipped.length > 0 && <li>{t.wordpress.skipped(s.skipped)}</li>}
        </ul>
      </Dialog>
    );
  }

  return (
    <Dialog
      open
      onClose={onClose}
      title={t.wordpress.movingTitle}
      description={t.wordpress.movingDescription}
      actions={
        <>
          <button className="btn" onClick={onClose}>{t.common.cancel}</button>
          <button className="btn primary" onClick={choose}><FileUp size={15} /> {t.wordpress.chooseFile}</button>
        </>
      }
    >
      <ol className="muted" style={{ paddingLeft: 18, margin: "0 0 12px", lineHeight: 1.8 }}>
        <li>{t.wordpress.exportStep}</li>
        <li>{t.wordpress.chooseStep(mode === "new")}</li>
      </ol>
      {err && <Banner kind="danger" title={t.wordpress.didntWork}>{err.message}</Banner>}
    </Dialog>
  );
}

export function ImportReport({ report: r, onClose }: { report: WPReport; onClose: () => void }) {
  return (
    <Dialog open onClose={onClose} title={t.wordpress.reportTitle} actions={<button className="btn primary" onClick={onClose}>{t.wordpress.startExploring}</button>}>
      <div className="row" style={{ alignItems: "flex-start", gap: 14, marginBottom: 14 }}>
        <CheckCircle2 size={34} color="var(--success)" />
        <div className="muted">
          {t.wordpress.imported(r.posts, r.pages, r.images, r.files, r.menuItems)}
        </div>
      </div>
      {(r.warnings?.length ?? 0) > 0 && (
        <Banner kind="warn" title={t.wordpress.attention}>
          <ul className="small" style={{ paddingLeft: 16, margin: "4px 0 0" }}>
            {r.warnings!.slice(0, 8).map((w, i) => <li key={i}>{w}</li>)}
            {r.warnings!.length > 8 && <li>{t.wordpress.andMore(r.warnings!.length - 8)}</li>}
          </ul>
        </Banner>
      )}
      {(r.failedImages?.length ?? 0) > 0 && (
        <Banner kind="warn" icon={<AlertTriangle size={18} color="var(--warning)" />} title={t.wordpress.failedImages(r.failedImages!.length)}>
          <span className="small">{t.wordpress.failedImagesBody}</span>
        </Banner>
      )}
      {(r.failedFiles?.length ?? 0) > 0 && (
        <Banner kind="warn" icon={<AlertTriangle size={18} color="var(--warning)" />} title={t.wordpress.failedFiles(r.failedFiles!.length)}>
          <span className="small">{t.wordpress.failedFilesBody}</span>
        </Banner>
      )}
    </Dialog>
  );
}
