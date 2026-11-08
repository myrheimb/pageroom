import { useEffect, useState } from "react";
import { ArrowRight, ArrowRightLeft, CloudDownload, FileText, FolderOpen, Globe, Sparkles, Trash2 } from "lucide-react";
import { api, type Welcome as WelcomeData, type AppError } from "../api";
import { useStore } from "../store";
import { Banner, Dialog, Menu, Spinner, timeAgo } from "../ui";
import { SignInDialog } from "../dialogs/SignIn";
import { WordPressImport } from "../dialogs/WordPress";
import { t } from "../i18n";

export function Welcome() {
  const [data, setData] = useState<WelcomeData | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [dialog, setDialog] = useState<"clone" | "create" | "wordpress" | null>(null);
  const { setSite, error } = useStore();

  const load = () => api.getWelcome().then(setData).catch(error);
  useEffect(() => { load(); }, []);

  const open = async (path: string) => {
    setBusy(t.welcome.opening);
    try {
      setSite(await api.openSite(path));
    } catch (e) {
      error(e, t.welcome.couldntOpen);
    } finally {
      setBusy(null);
    }
  };

  const chooseFolder = async () => {
    const p = await api.chooseFolder(t.welcome.chooseFolder);
    if (p) open(p);
  };

  return (
    <div className="welcome">
      <div className="welcome-inner">
        <div className="brand">
          <div className="mark"><FileText size={24} /></div>
          <div>
            <h1>{t.welcome.title}</h1>
            <p className="lead">{t.welcome.lead}</p>
          </div>
        </div>

        <div className="start-cards">
          <button className="start-card" onClick={chooseFolder}>
            <div className="ic"><FolderOpen size={20} /></div>
            <h3>{t.welcome.openLocal}</h3>
            <p>{t.welcome.openLocalHint}</p>
          </button>
          <button className="start-card" onClick={() => setDialog("clone")}>
            <div className="ic"><CloudDownload size={20} /></div>
            <h3>{t.welcome.clone}</h3>
            <p>{t.welcome.cloneHint}</p>
          </button>
          <button className="start-card" onClick={() => setDialog("create")}>
            <div className="ic"><Sparkles size={20} /></div>
            <h3>{t.welcome.create}</h3>
            <p>{t.welcome.createHint}</p>
          </button>
          <button className="start-card" onClick={() => setDialog("wordpress")}>
            <div className="ic"><ArrowRightLeft size={20} /></div>
            <h3>{t.welcome.wordpress}</h3>
            <p>{t.welcome.wordpressHint}</p>
          </button>
        </div>

        {data && data.recent?.length > 0 && (
          <>
            <h3 style={{ fontSize: 13, color: "var(--text-2)", margin: "0 0 10px", fontWeight: 600 }}>{t.welcome.recent}</h3>
            <div className="card list">
              {data.recent.map((r) => (
                <div key={r.path} className="list-item clickable" onClick={() => open(r.path)}>
                  <div className="site-switch" style={{ margin: 0, padding: 0, background: "none" }}>
                    <div className="logo">{(r.title || "S").slice(0, 1).toUpperCase()}</div>
                  </div>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontWeight: 600 }}>{r.title || r.path.split(/[\\/]/).pop()}</div>
                    <div className="dim small truncate">{r.path}</div>
                  </div>
                  <span className="dim small nowrap">{timeAgo(r.opened)}</span>
                  <div onClick={(e) => e.stopPropagation()}>
                    <Menu
                      trigger={<button className="btn ghost icon sm" aria-label={t.common.more}>⋯</button>}
                      items={[{ label: t.welcome.removeFromList, icon: <Trash2 size={14} />, run: () => api.forgetSite(r.path).then(load) }]}
                    />
                  </div>
                  <ArrowRight size={16} className="dim" />
                </div>
              ))}
            </div>
          </>
        )}
      </div>

      {busy && (
        <Dialog open onClose={() => {}} dismissable={false} title={busy}>
          <div className="row"><Spinner /> <span className="muted">{t.welcome.bigSites}</span></div>
        </Dialog>
      )}
      {dialog === "clone" && <CloneDialog onClose={() => setDialog(null)} githubSignIn={!!data?.githubSignIn} />}
      {dialog === "create" && <CreateDialog onClose={() => setDialog(null)} />}
      {dialog === "wordpress" && (
        <WordPressImport
          mode="new"
          onClose={() => setDialog(null)}
          onSite={(site, report) => { setSite(site); useStore.getState().set({ importReport: report }); }}
        />
      )}
    </div>
  );
}

function CloneDialog({ onClose, githubSignIn }: { onClose: () => void; githubSignIn: boolean }) {
  const [url, setUrl] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<AppError | null>(null);
  const [signIn, setSignIn] = useState(false);
  const setSite = useStore((s) => s.setSite);

  const clone = async () => {
    setBusy(true);
    setErr(null);
    try {
      setSite(await api.cloneSite(url, ""));
    } catch (e) {
      setErr(e as AppError);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Dialog
        open
        onClose={onClose}
        title={t.welcome.clone}
        description={t.welcome.cloneDescription}
        actions={
          <>
            <button className="btn" onClick={onClose}>{t.common.cancel}</button>
            <button className="btn primary" disabled={!url.trim() || busy} onClick={clone}>
              {busy ? <Spinner /> : <CloudDownload size={16} />} {t.welcome.downloadSite}
            </button>
          </>
        }
      >
        <div className="field">
          <label htmlFor="clone-url">{t.welcome.repoAddress}</label>
          <input id="clone-url" className="input" autoFocus placeholder="https://github.com/your-name/your-site" value={url} onChange={(e) => setUrl(e.target.value)} onKeyDown={(e) => e.key === "Enter" && url && clone()} />
        </div>
        {err && (
          <Banner kind={err.code === "auth" || err.code === "not-found" ? "warn" : "danger"} title={err.code === "auth" || err.code === "not-found" ? t.welcome.signInToContinue : t.welcome.couldntDownload}>
            <div className="small">{err.message}</div>
            {(err.code === "auth" || err.code === "not-found") && (
              <button className="btn sm" style={{ marginTop: 8 }} onClick={() => setSignIn(true)}>{githubSignIn ? t.welcome.signInWithGitHub : t.common.signIn}</button>
            )}
          </Banner>
        )}
        <p className="dim small" style={{ margin: 0 }}>{t.welcome.privateSite}</p>
      </Dialog>
      {signIn && <SignInDialog remote={url} onClose={() => setSignIn(false)} onDone={() => { setSignIn(false); clone(); }} />}
    </>
  );
}

function CreateDialog({ onClose }: { onClose: () => void }) {
  const [title, setTitle] = useState("");
  const [busy, setBusy] = useState(false);
  const { setSite, go, error } = useStore();
  const create = async () => {
    setBusy(true);
    try {
      setSite(await api.createSite(title.trim()));
      go({ kind: "appearance", tab: "themes" });
    } catch (e) {
      error(e, t.welcome.couldntCreate);
      setBusy(false);
    }
  };
  return (
    <Dialog
      open
      onClose={onClose}
      title={t.welcome.create}
      description={t.welcome.createDescription}
      actions={
        <>
          <button className="btn" onClick={onClose}>{t.common.cancel}</button>
          <button className="btn primary" disabled={!title.trim() || busy} onClick={create}>{busy ? <Spinner /> : <Sparkles size={16} />} {t.welcome.createSite}</button>
        </>
      }
    >
      <div className="field">
        <label htmlFor="site-title">{t.welcome.siteName}</label>
        <input id="site-title" className="input" autoFocus placeholder={t.welcome.siteNamePlaceholder} value={title} onChange={(e) => setTitle(e.target.value)} onKeyDown={(e) => e.key === "Enter" && title.trim() && create()} />
      </div>
      <p className="dim small" style={{ margin: 0 }}><Globe size={12} /> {t.welcome.goLiveLater}</p>
    </Dialog>
  );
}
