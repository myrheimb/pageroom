import { useEffect, useState } from "react";
import { Copy, ExternalLink, KeyRound, ShieldCheck } from "lucide-react";
import { api, type AppError, type DeviceCode } from "../api";
import { useStore } from "../store";
import { Banner, Dialog, Spinner } from "../ui";
import { t } from "../i18n";

/**
 * Sign in to the site's git host. GitHub uses the device flow, so no password
 * is ever typed into Pageroom. Other hosts use an access token. Either way the
 * secret goes straight to the OS keychain.
 */
export function SignInDialog({ remote, onClose, onDone }: { remote: string; onClose: () => void; onDone: () => void }) {
  const isGitHub = !remote || /github\.com/i.test(remote);
  const [mode, setMode] = useState<"github" | "token">(isGitHub ? "github" : "token");
  const [canDevice, setCanDevice] = useState(true);
  useEffect(() => {
    api.getWelcome().then((w) => { if (!w.githubSignIn) { setCanDevice(false); setMode("token"); } }).catch(() => {});
  }, []);
  return (
    <Dialog open onClose={() => { api.cancelSignIn(); onClose(); }} title={isGitHub ? t.signIn.titleGitHub : t.signIn.titleOther}>
      {isGitHub && canDevice && (
        <div className="seg" style={{ marginBottom: 16 }}>
          <button className={mode === "github" ? "on" : ""} onClick={() => setMode("github")}><ShieldCheck size={14} /> {t.signIn.withGitHub}</button>
          <button className={mode === "token" ? "on" : ""} onClick={() => setMode("token")}><KeyRound size={14} /> {t.signIn.useToken}</button>
        </div>
      )}
      {mode === "github" ? <DeviceFlow onDone={onDone} /> : <TokenForm remote={remote} isGitHub={isGitHub} onDone={onDone} />}
    </Dialog>
  );
}

function DeviceFlow({ onDone }: { onDone: () => void }) {
  const [code, setCode] = useState<DeviceCode | null>(null);
  const [err, setErr] = useState<AppError | null>(null);
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const dc = await api.startGitHubSignIn();
        if (!alive) return;
        setCode(dc);
        await api.waitGitHubSignIn();
        if (alive) onDone();
      } catch (e) {
        if (alive && (e as AppError).code !== "cancelled") setErr(e as AppError);
      }
    })();
    return () => { alive = false; api.cancelSignIn(); };
  }, []);
  if (err) return <Banner kind="danger" title={t.signIn.didntWork}>{err.message}</Banner>;
  if (!code) return <div className="row"><Spinner /> <span className="muted">{t.signIn.contacting}</span></div>;
  return (
    <div className="stack">
      <p className="muted" style={{ margin: 0 }}>{t.signIn.enterCode}</p>
      <div className="device-code">{code.userCode}</div>
      <div className="row">
        <button className="btn sm" onClick={() => { navigator.clipboard?.writeText(code.userCode); setCopied(true); }}><Copy size={13} /> {copied ? t.common.copied : t.signIn.copyCode}</button>
        <button className="btn sm ghost" onClick={() => api.openExternal(code.verificationUri)}><ExternalLink size={13} /> {t.signIn.openAgain}</button>
        <div className="spacer" />
        <Spinner size={14} /> <span className="dim small">{t.signIn.waiting}</span>
      </div>
      <p className="dim small" style={{ margin: 0 }}>{t.signIn.youChoose}</p>
    </div>
  );
}

function TokenForm({ remote, isGitHub, onDone }: { remote: string; isGitHub: boolean; onDone: () => void }) {
  const [user, setUser] = useState("");
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<AppError | null>(null);
  const githubPages = useStore((state) => !!state.site?.git.pages?.setUp);
  const save = async () => {
    setBusy(true);
    setErr(null);
    try {
      await api.saveAccessToken(remote, user, token);
      setToken("");
      onDone();
    } catch (e) {
      setErr(e as AppError);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div>
      <p className="muted" style={{ marginTop: 0 }}>
        {isGitHub ? (
          t.signIn.tokenGitHub((text) => <a href="#" onClick={(e) => { e.preventDefault(); api.openExternal("https://github.com/settings/personal-access-tokens/new"); }}>{text}</a>, githubPages)
        ) : (
          t.signIn.tokenOther
        )}
      </p>
      {!isGitHub && (
        <div className="field">
          <label htmlFor="tok-user">{t.signIn.username}</label>
          <input id="tok-user" className="input" value={user} onChange={(e) => setUser(e.target.value)} autoComplete="username" />
        </div>
      )}
      <div className="field">
        <label htmlFor="tok">{t.signIn.accessToken}</label>
        <input id="tok" className="input mono" type="password" autoComplete="off" value={token} onChange={(e) => setToken(e.target.value)} placeholder={isGitHub ? "github_pat_…" : ""} />
        <span className="help">{t.signIn.tokenHelp}</span>
      </div>
      {err && <Banner kind="danger">{err.message}</Banner>}
      <div className="dialog-actions">
        <button className="btn primary" disabled={token.trim().length < 8 || busy} onClick={save}>{busy ? <Spinner /> : <KeyRound size={15} />} {t.signIn.saveAndContinue}</button>
      </div>
    </div>
  );
}
