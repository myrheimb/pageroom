import { useEffect, useState } from "react";
import { AlertTriangle, Download, ExternalLink, Info, Monitor, RotateCw, ShieldCheck, Smartphone, Tablet, X } from "lucide-react";
import { api, on, type AppError } from "../api";
import { Banner, Spinner } from "../ui";
import { t } from "../i18n";

type Device = "desktop" | "tablet" | "phone";

export function PreviewPane({ pageURL, tick }: { pageURL: string; tick: number }) {
  const [base, setBase] = useState<string | null>(null);
  const [err, setErr] = useState<AppError | null>(null);
  const [note, setNote] = useState("");
  const [problems, setProblems] = useState<string[]>([]);
  const [device, setDevice] = useState<Device>("desktop");
  const [reloads, setReloads] = useState(0);

  const start = () => {
    setErr(null);
    api.startPreview().then((p) => { setBase(p.url); setNote(p.note ?? ""); }).catch((e) => setErr(e));
  };
  useEffect(() => { start(); }, []);

  // After each save, Hugo rebuilds. Check for template problems shortly after.
  useEffect(() => {
    if (!base) return;
    const t = setTimeout(() => api.previewStatus().then((s) => setProblems(s.errors ?? [])).catch(() => {}), 1200);
    return () => clearTimeout(t);
  }, [tick, base]);

  const src = base ? base.replace(/\/$/, "") + pageURL : "";
  return (
    <div className="preview-pane">
      <div className="preview-bar">
        <div className="seg">
          <button className={device === "desktop" ? "on" : ""} onClick={() => setDevice("desktop")} aria-label={t.preview.desktop}><Monitor size={13} /></button>
          <button className={device === "tablet" ? "on" : ""} onClick={() => setDevice("tablet")} aria-label={t.preview.tablet}><Tablet size={13} /></button>
          <button className={device === "phone" ? "on" : ""} onClick={() => setDevice("phone")} aria-label={t.preview.phone}><Smartphone size={13} /></button>
        </div>
        <div className="preview-url">{pageURL}</div>
        <button className="btn ghost icon sm" aria-label={t.preview.reload} onClick={() => setReloads((n) => n + 1)}><RotateCw size={14} /></button>
        {base && <button className="btn ghost icon sm" aria-label={t.preview.openInBrowser} onClick={() => api.openExternal(src)}><ExternalLink size={14} /></button>}
      </div>
      {note && (
        <div className="preview-note">
          <Info size={13} />
          <span style={{ flex: 1 }}>{note}</span>
          <button className="btn ghost icon sm" aria-label={t.common.dismiss} onClick={() => setNote("")}><X size={12} /></button>
        </div>
      )}
      {problems.length > 0 && (
        <div style={{ padding: "10px 10px 0" }}>
          <Banner kind="warn" title={t.preview.problem}>
            <div className="mono small selectable" style={{ whiteSpace: "pre-wrap" }}>{problems.join("\n")}</div>
          </Banner>
        </div>
      )}
      <div className={"preview-frame-wrap device-" + device}>
        {err?.code === "hugo-download" ? (
          <div className="preview-wait"><HugoDownloadPrompt need={err} onReady={start} /></div>
        ) : err ? (
          <div className="preview-wait">
            <div>
              <AlertTriangle size={28} color="var(--warning)" />
              <p><strong>{t.preview.couldntStart}</strong></p>
              <pre className="mono small selectable" style={{ whiteSpace: "pre-wrap", textAlign: "left", maxWidth: 520 }}>{err.message}</pre>
              <button className="btn" onClick={start}><RotateCw size={14} /> {t.common.tryAgain}</button>
            </div>
          </div>
        ) : !base ? (
          <div className="preview-wait">
            <div style={{ width: 280 }}>
              <Spinner size={22} />
              <p>{t.preview.building}</p>
            </div>
          </div>
        ) : (
          <iframe
            key={reloads}
            className="preview-frame"
            src={src}
            title={t.preview.frameTitle}
            // Theme scripts run here, isolated from Pageroom: different origin, and
            // the app only accepts calls from its own page.
            sandbox="allow-scripts allow-same-origin allow-forms allow-popups"
          />
        )}
      </div>
    </div>
  );
}

export function SitePreview({ mac, onClose }: { mac: boolean; onClose: () => void }) {
  useEffect(() => {
    const close = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", close);
    return () => window.removeEventListener("keydown", close);
  }, [onClose]);
  return (
    <div className={"site-preview" + (mac ? " mac" : "")}>
      <header className="site-preview-bar">
        <strong>{t.preview.fullTitle}</strong>
        <span className="dim small">{t.preview.onlyYou}</span>
        <button className="btn" onClick={onClose}><X size={15} /> {t.preview.closePreview}</button>
      </header>
      <PreviewPane pageURL="/" tick={0} />
    </div>
  );
}

/**
 * Asks before Pageroom downloads its own copy of Hugo. Nothing is installed
 * on the computer: the program is kept in Pageroom's cache folder.
 */
export function HugoDownloadPrompt({ need, onReady, onCancel }: { need: AppError; onReady: () => void; onCancel?: () => void }) {
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<AppError | null>(null);
  const [declined, setDeclined] = useState(false);
  useEffect(() => on("hugo:download", (d) => setProgress(d)), []);
  const version = need.details?.version ?? "";
  const installedVersion = need.details?.installedVersion;
  const installed = installedVersion && need.details?.installedWithoutExtended ? t.preview.withoutExtended(installedVersion) : installedVersion;

  const download = async () => {
    setBusy(true);
    setErr(null);
    try {
      await api.downloadHugo();
      onReady();
    } catch (e) {
      setErr(e as AppError);
    } finally {
      setBusy(false);
    }
  };

  if (declined) {
    return (
      <div className="hugo-prompt">
        <p className="muted">{t.preview.declined}</p>
        <button className="btn" onClick={() => setDeclined(false)}>{t.preview.setUpPreview}</button>
      </div>
    );
  }
  return (
    <div className="hugo-prompt">
      <div className="hp-icon"><Download size={22} /></div>
      <h3>{t.preview.getBuilder}</h3>
      <p className="muted">
        {t.preview.needsHugo(version)}
        {installed && t.preview.installedIs(installed)}
      </p>
      <p className="dim small"><ShieldCheck size={12} /> {t.preview.downloadNote}</p>
      {busy && (
        <div style={{ margin: "14px 0" }}>
          <div className="progress"><div style={{ width: `${progress && progress.total > 0 ? (progress.done / progress.total) * 100 : 4}%` }} /></div>
          <div className="dim small" style={{ marginTop: 6 }}>{progress && progress.total > 0 ? t.preview.progress(Math.round(progress.done / 1048576), Math.round(progress.total / 1048576)) : t.preview.starting}</div>
        </div>
      )}
      {err && <Banner kind="danger" title={t.preview.downloadFailed}>{err.message}</Banner>}
      {!busy && (
        <div className="row" style={{ justifyContent: "center", marginTop: 14 }}>
          <button className="btn" onClick={() => (onCancel ? onCancel() : setDeclined(true))}>{t.common.notNow}</button>
          <button className="btn primary" onClick={download}><Download size={15} /> {t.preview.download(version)}</button>
        </div>
      )}
    </div>
  );
}
