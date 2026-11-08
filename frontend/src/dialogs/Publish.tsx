import { useEffect, useMemo, useState } from "react";
import { CheckCircle2, FileText, Image, LayoutTemplate, Palette, Settings2, File, Rocket, Undo2, ShieldAlert, Users } from "lucide-react";
import { api, type AppError, type ChangeView, type Choice, type Conflict, type Outcome, type Resolution } from "../api";
import { useStore } from "../store";
import { markdownToHtml, stripUnsafeUrls } from "../editor/markdown";
import { ask, Banner, Dialog, Spinner } from "../ui";
import { SignInDialog } from "./SignIn";
import { IdentityDialog } from "./Identity";
import { t } from "../i18n";

const typeIcon: Record<ChangeView["type"], React.ReactNode> = {
  page: <FileText size={15} />, image: <Image size={15} />, settings: <Settings2 size={15} />, appearance: <Palette size={15} />, theme: <LayoutTemplate size={15} />, file: <File size={15} />,
};

function changeLabel(change: ChangeView): string {
  switch (change.type) {
    case "theme": return t.publish.themeChange(change.path.split("/")[1]);
    case "settings": return t.publish.settingsChange;
    case "appearance": return t.publish.appearanceChange;
    default: return change.label;
  }
}

type Phase = { kind: "review" } | { kind: "working" } | { kind: "done"; outcome: Outcome } | { kind: "conflicts"; conflicts: Conflict[] } | { kind: "blocked"; outcome: Outcome };

export function PublishDialog({ onClose }: { onClose: () => void }) {
  const { changes, refreshChanges, refresh, site, flushAll, set, error, toast } = useStore();
  const [message, setMessage] = useState("");
  const [retry, setRetry] = useState(false);
  const [phase, setPhase] = useState<Phase>({ kind: "review" });
  const [need, setNeed] = useState<"signin" | "identity" | null>(null);

  useEffect(() => {
    (async () => {
      await flushAll();
      await refreshChanges();
      const pending = await api.pendingPublish().catch(() => "");
      setRetry(!!pending);
      setMessage(pending || (await api.defaultPublishMessage().catch(() => "")));
    })();
  }, []);

  const handle = async (p: Promise<Outcome>) => {
    setPhase({ kind: "working" });
    try {
      const out = await p;
      if (out.state === "conflicts") setPhase({ kind: "conflicts", conflicts: out.conflicts ?? [] });
      else if (out.state === "blocked") setPhase({ kind: "blocked", outcome: out });
      else setPhase({ kind: "done", outcome: out });
      await refresh();
    } catch (e) {
      const err = e as AppError;
      setPhase({ kind: "review" });
      if (err.code === "auth") setNeed("signin");
      else if (err.code === "need-identity") setNeed("identity");
      else if (err.code === "offline") set({ offline: true }), error(err, t.publish.offline);
      else if (err.code === "workflow-permission") toast({ kind: "error", title: t.publish.didntFinish, body: err.message, action: { label: t.publish.openTokenSettings, run: () => api.openExternal("https://github.com/settings/personal-access-tokens") } });
      else error(err, t.publish.didntFinish);
    }
  };
  const publish = async () => { await flushAll(); handle(api.publish(message)); };

  // Several files can make up one "Appearance", "Site settings" or theme
  // change. Show them as one row, so undoing it undoes all of them.
  const groups = useMemo(() => {
    const g: Record<string, (ChangeView & { paths: string[] })[]> = {};
    for (const c of changes) {
      const list = (g[c.type] ??= []);
      const same = (c.type === "appearance" || c.type === "settings" || c.type === "theme") && list.find((x) => x.label === c.label);
      if (same) same.paths.push(c.path);
      else list.push({ ...c, paths: [c.path] });
    }
    return g;
  }, [changes]);

  if (need === "signin") return <SignInDialog remote={site?.git.remote ?? ""} onClose={() => setNeed(null)} onDone={() => { setNeed(null); publish(); }} />;
  if (need === "identity") return <IdentityDialog onClose={() => setNeed(null)} onSaved={() => { setNeed(null); publish(); }} />;

  if (phase.kind === "conflicts") {
    return <ConflictResolver conflicts={phase.conflicts} onCancel={onClose} onResolve={(choices) => handle(api.resolve(choices))} />;
  }

  if (phase.kind === "blocked") {
    return (
      <Dialog open onClose={onClose} title={t.publish.blockedTitle} actions={<button className="btn primary" onClick={onClose}>{t.common.ok}</button>}>
        <Banner kind="danger" icon={<ShieldAlert size={18} color="var(--danger)" />} title={t.publish.blockedBanner}>
          <span className="small">{t.publish.blockedBody}</span>
        </Banner>
        <div className="card list">
          {phase.outcome.secrets?.map((f, i) => (
            <div key={i} className="list-item">
              <ShieldAlert size={15} color="var(--danger)" />
              <div style={{ flex: 1 }}>
                <div className="mono">{f.path}{f.line ? t.publish.atLine(f.line) : ""}</div>
                <div className="dim small">{t.publish.looksLike(t.publish.secretKinds[f.kind] ?? f.kind)}</div>
              </div>
              <button className="btn sm" onClick={async () => {
                if (await ask.confirm(t.publish.undoFileTitle, t.publish.undoFileBody(f.path), t.publish.undoChanges, true)) {
                  await api.discard(f.path).catch(error);
                  await refreshChanges();
                }
              }}><Undo2 size={13} /> {t.publish.undoFile}</button>
            </div>
          ))}
        </div>
        <p className="dim small">{t.publish.blockedHelp}</p>
      </Dialog>
    );
  }

  if (phase.kind === "done") {
    const o = phase.outcome;
    return (
      <Dialog open onClose={onClose} title={o.state === "published" ? t.publish.published : t.publish.nothingNew} actions={<button className="btn primary" onClick={onClose}>{t.publish.done}</button>}>
        <div className="row" style={{ alignItems: "flex-start", gap: 14 }}>
          <CheckCircle2 size={36} color="var(--success)" />
          <div className="muted">
            {o.state === "published" && site?.git.direct ? (
              <>{t.settings.liveAt(<span className="mono">{site.git.direct.address}</span>)}</>
            ) : o.state === "published" ? (
              <>{t.publish.onTheirWay}</>
            ) : (
              <>{o.message ?? t.publish.alreadyPublished}</>
            )}
            {o.incoming && o.incoming.length > 0 && (
              <p className="small" style={{ marginBottom: 0 }}><Users size={13} /> {t.publish.broughtIn(o.incoming.length)}</p>
            )}
          </div>
        </div>
      </Dialog>
    );
  }

  const working = phase.kind === "working";
  const firstPublish = (!!site?.git.remote && !site.git.published) || (!!site?.git.direct && !site.git.direct.published);
  return (
    <Dialog
      open
      onClose={() => !working && onClose()}
      dismissable={!working}
      title={t.publish.title}
      description={changes.length ? t.publish.willGoLive(changes.length) : firstPublish ? (site?.git.direct ? t.publish.firstOnline : t.publish.firstUpload) : t.publish.nothingToPublish}
      actions={
        <>
          <button className="btn" disabled={working} onClick={onClose}>{t.common.notNow}</button>
          <button className="btn primary" disabled={working || (!changes.length && !firstPublish)} onClick={publish}>
            {working ? <Spinner /> : <Rocket size={16} />} {working ? t.publish.publishing : retry ? t.common.tryAgain : t.publish.publishNow}
          </button>
        </>
      }
    >
      {retry && (
        <div style={{ marginBottom: 12 }}>
          <Banner kind="warn" title={t.publish.lastDidntFinish}>
            <span className="small">{t.publish.lastDidntFinishBody}</span>
          </Banner>
        </div>
      )}
      {Object.entries(groups).map(([type, items]) => (
        <div key={type} className="card list" style={{ marginBottom: 12 }}>
          {items.map((c) => (
            <div key={c.path} className="list-item">
              <span className="dim">{typeIcon[c.type]}</span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div className="truncate" style={{ fontWeight: 550 }}>{changeLabel(c)}</div>
                {c.type === "file" || c.type === "page" ? <div className="dim small truncate">{c.path}</div> : null}
              </div>
              <span className={"pill " + (c.kind === "deleted" ? "danger" : c.kind === "added" ? "live" : "")}>{t.publish[c.kind]}</span>
              <button
                className="btn ghost icon sm"
                title={t.publish.undoChangeTip}
                disabled={working}
                onClick={async () => {
                  if (await ask.confirm(t.publish.undoChangeTitle, t.publish.undoChangeBody(changeLabel(c)), t.publish.undoChange, true)) {
                    for (const p of c.paths) await api.discard(p).catch(error);
                    await refresh();
                  }
                }}
              >
                <Undo2 size={14} />
              </button>
            </div>
          ))}
        </div>
      ))}
      {changes.length > 0 && (
        <div className="field" style={{ marginTop: 16, marginBottom: 0 }}>
          <label htmlFor="pub-msg">{t.publish.describe} <span className="dim">{t.common.optional}</span></label>
          <input id="pub-msg" className="input" value={message} onChange={(e) => setMessage(e.target.value)} disabled={working || retry} />
          <span className="help">{retry ? t.publish.messageLocked : t.publish.messageHelp}</span>
        </div>
      )}
    </Dialog>
  );
}


export function ConflictResolver({ conflicts, onResolve, onCancel, actionLabel = t.conflicts.keepAndPublish }: { conflicts: Conflict[]; onResolve: (c: Record<string, Choice>) => void; onCancel: () => void; actionLabel?: string }) {
  const init = () => {
    const c: Record<string, Choice> = {};
    for (const f of conflicts) {
      const n = f.hunks?.filter((h) => h.conflict).length ?? 0;
      c[f.path] = n > 0 && !f.binary ? { hunks: Array(n).fill("") } : { whole: undefined };
    }
    return c;
  };
  const [choices, setChoices] = useState<Record<string, Choice>>(init);
  const author = conflicts[0]?.theirAuthor;
  const who = author ? t.conflicts.person(author) : t.conflicts.aTeammate;
  const complete = conflicts.every((f) => {
    const c = choices[f.path];
    return c.whole || (c.hunks && c.hunks.length > 0 && c.hunks.every(Boolean));
  });
  const setHunk = (path: string, i: number, r: Resolution) =>
    setChoices((c) => ({ ...c, [path]: { hunks: c[path].hunks!.map((x, j) => (j === i ? r : x)) } }));
  const setWhole = (path: string, r: Resolution) => setChoices((c) => ({ ...c, [path]: { whole: r } }));

  return (
    <Dialog
      open
      wide
      onClose={onCancel}
      title={t.conflicts.title(who)}
      description={t.conflicts.description}
      actions={
        <>
          <button className="btn" onClick={onCancel}>{t.conflicts.decideLater}</button>
          <button className="btn primary" disabled={!complete} onClick={() => onResolve(choices)}><Rocket size={16} /> {actionLabel}</button>
        </>
      }
    >
      {conflicts.map((f) => {
        const name = f.path.split("/").slice(-2).join("/");
        if (f.binary || f.oursDeleted || f.theirsDeleted || f.localInWay || !f.hunks?.some((h) => h.conflict)) {
          const what = f.theirsDeleted ? t.conflicts.theyDeleted(who) : f.oursDeleted ? t.conflicts.youDeleted(who) : f.localInWay ? t.conflicts.sameName(who) : t.conflicts.bothChanged;
          const c = choices[f.path];
          return (
            <div key={f.path} className="conflict">
              <div className="conflict-head"><FileText size={15} /><strong>{name}</strong><span className="dim small">{what}</span></div>
              <div className="conflict-foot">
                <button className={"btn sm" + (c.whole === "ours" ? " primary" : "")} onClick={() => setWhole(f.path, "ours")}>{f.oursDeleted ? t.conflicts.keepDeleted : t.conflicts.keepMine}</button>
                <button className={"btn sm" + (c.whole === "theirs" ? " primary" : "")} onClick={() => setWhole(f.path, "theirs")}>{f.theirsDeleted ? t.conflicts.deleteIt : t.conflicts.keepTheirs(who)}</button>
                {!f.oursDeleted && !f.theirsDeleted && <button className={"btn sm" + (c.whole === "both" ? " primary" : "")} onClick={() => setWhole(f.path, "both")}>{t.conflicts.keepBoth}</button>}
              </div>
            </div>
          );
        }
        let ci = -1;
        return (
          <div key={f.path} className="conflict">
            <div className="conflict-head"><FileText size={15} /><strong>{name}</strong></div>
            {f.hunks!.map((h, hi) => {
              if (!h.conflict) {
                const lines = (h.lines ?? []).filter((l) => l.trim() && !isSettingsLine(l));
                const ctx = (hi === 0 ? lines.slice(-2) : lines.slice(0, 2)).join(" ");
                return ctx ? <div key={hi} className="conflict-context">{hi === 0 ? "…" : ""}{plain(ctx)}{hi === 0 ? "" : "…"}</div> : null;
              }
              ci++;
              const idx = ci;
              const sel = choices[f.path].hunks?.[idx];
              return (
                <div key={hi}>
                  <div className="conflict-sides">
                    <div className={"conflict-side" + (sel === "ours" || sel === "both" ? " on" : "")} onClick={() => setHunk(f.path, idx, "ours")}>
                      <div className="who">{t.conflicts.yourVersion}</div>
                      <Rendered lines={h.ours} />
                    </div>
                    <div className={"conflict-side" + (sel === "theirs" || sel === "both" ? " on" : "")} onClick={() => setHunk(f.path, idx, "theirs")}>
                      <div className="who">{t.conflicts.theirVersion(who)}</div>
                      <Rendered lines={h.theirs} />
                    </div>
                  </div>
                  <div className="conflict-foot">
                    <button className={"btn sm" + (sel === "ours" ? " primary" : "")} onClick={() => setHunk(f.path, idx, "ours")}>{t.conflicts.keepMine}</button>
                    <button className={"btn sm" + (sel === "theirs" ? " primary" : "")} onClick={() => setHunk(f.path, idx, "theirs")}>{t.conflicts.keepTheirs(who)}</button>
                    <button className={"btn sm" + (sel === "both" ? " primary" : "")} onClick={() => setHunk(f.path, idx, "both")}>{t.conflicts.keepBoth}</button>
                  </div>
                </div>
              );
            })}
          </div>
        );
      })}
    </Dialog>
  );
}

// Show each version formatted (as on the page), not as Markdown source.
function Rendered({ lines }: { lines?: string[] }) {
  const md = (lines ?? []).join("\n").trim();
  if (!md) return <div className="text"><em className="dim">{t.conflicts.removed}</em></div>;
  if (md.split("\n").every(isSettingsLine)) return <div className="text mono small">{md}</div>;
  const doc = new DOMParser().parseFromString(markdownToHtml(md), "text/html");
  doc.querySelectorAll("script, iframe, img, style").forEach((n) => n.remove());
  stripUnsafeUrls(doc);
  return <div className="text prose-editor" style={{ minHeight: 0, fontSize: 14.5 }} dangerouslySetInnerHTML={{ __html: doc.body.innerHTML }} />;
}

function isSettingsLine(l: string): boolean {
  return /^(\+\+\+|---)\s*$/.test(l) || /^[\w-]+\s*[=:]/.test(l) || /^\s*\[.*\]\s*$/.test(l);
}

function plain(md: string): string {
  return md.replace(/[*_`#>]/g, "").replace(/\[([^\]]*)\]\([^)]*\)/g, "$1");
}
