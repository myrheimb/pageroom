import { useEffect, useState } from "react";
import { ArrowDown, ArrowUp, CheckCircle2, ExternalLink, Globe, ImagePlus, KeyRound, LogOut, Plus, Save, X, ShieldCheck, User, Rocket, Download } from "lucide-react";
import { api, type AuthStatus, type Cloudflare, type CloudflareAccount, type DirectUpload, type GitHubPages, type Language as SiteLanguage, type LanguageSettings, type Settings as SiteSettings, type SiteIcon } from "../api";
import { useStore } from "../store";
import { ask, Banner, Spinner } from "../ui";
import { SignInDialog } from "../dialogs/SignIn";
import { WordPressImport } from "../dialogs/WordPress";
import { language as uiLanguage, languageNames, setLanguage, systemLanguage, t, type Language } from "../i18n";

// Hugo needs the scheme and a trailing slash in baseURL.
export function webAddress(typed: string): string {
  const address = typed.trim();
  if (!address) return "";
  const withScheme = /^https?:\/\//i.test(address) ? address : `https://${address}`;
  return withScheme.replace(/\/?$/, "/");
}

export function SettingsView({ tab }: { tab: "general" | "account" | "publishing" }) {
  const go = useStore((s) => s.go);
  return (
    <div className="page-pad" style={{ maxWidth: 760 }}>
      <h1 className="page-title">{t.settings.title}</h1>
      <div className="seg" style={{ margin: "10px 0 22px" }}>
        <button className={tab === "general" ? "on" : ""} onClick={() => go({ kind: "settings", tab: "general" })}><Globe size={13} /> {t.settings.site}</button>
        <button className={tab === "publishing" ? "on" : ""} onClick={() => go({ kind: "settings", tab: "publishing" })}><Rocket size={13} /> {t.settings.publishing}</button>
        <button className={tab === "account" ? "on" : ""} onClick={() => go({ kind: "settings", tab: "account" })}><User size={13} /> {t.settings.you}</button>
      </div>
      {tab === "general" && <div className="stack" style={{ gap: 16 }}><General /><Languages /></div>}
      {tab === "publishing" && <Publishing />}
      {tab === "account" && <Account />}
    </div>
  );
}

function General() {
  const { site, refresh, toast, error, refreshChanges } = useStore();
  const [s, setS] = useState<SiteSettings | null>(site?.settings ?? null);
  const [saving, setSaving] = useState(false);
  if (!s || !site) return null;
  const dirty = JSON.stringify(s) !== JSON.stringify(site.settings);
  const f = (k: keyof SiteSettings) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => setS({ ...s, [k]: e.target.value });
  return (
    <div className="card card-pad">
      <div className="field"><label htmlFor="st-title">{t.settings.siteTitle}</label><input id="st-title" className="input" value={s.title} onChange={f("title")} /></div>
      <SiteIconField />
      <div className="field">
        <label htmlFor="st-desc">{t.settings.tagline}</label>
        <textarea id="st-desc" className="textarea" rows={2} value={s.description} onChange={f("description")} />
        <span className="help">{t.settings.taglineHelp}</span>
      </div>
      <div className="field">
        <label htmlFor="st-url">{t.settings.webAddress}</label>
        <input id="st-url" className="input mono" value={s.baseURL} onChange={f("baseURL")} placeholder="https://www.example.com/" />
        <span className="help">{t.settings.webAddressHelp}</span>
      </div>
      <div className="row" style={{ gap: 16 }}>
        <div className="field" style={{ flex: 1 }}><label htmlFor="st-lang">{t.settings.language}</label><input id="st-lang" className="input" value={s.language} onChange={f("language")} placeholder="en-US" /></div>
        <div className="field" style={{ flex: 2 }}><label htmlFor="st-copy">{t.settings.copyright}</label><input id="st-copy" className="input" value={s.copyright} onChange={f("copyright")} placeholder={`© ${new Date().getFullYear()} ${s.title}`} /></div>
      </div>
      <div className="field">
        <label htmlFor="st-form">{t.settings.formAddress}</label>
        <input id="st-form" className="input mono" value={s.formAddress} onChange={f("formAddress")} placeholder="https://formspree.io/f/…" />
        <span className="help">{t.settings.formAddressHelp}</span>
      </div>
      <div className="field">
        <label htmlFor="st-email">{t.settings.contactEmail}</label>
        <input id="st-email" className="input" type="email" value={s.contactEmail} onChange={f("contactEmail")} />
        <span className="help">{t.settings.contactEmailHelp}</span>
      </div>
      <div className="row">
        <div className="spacer" />
        <button className="btn primary" disabled={!dirty || saving} onClick={async () => {
          setSaving(true);
          try { const saved = { ...s, baseURL: webAddress(s.baseURL) }; await api.saveSettings(saved); setS(saved); await refresh(); refreshChanges(); toast({ kind: "success", title: t.settings.saved, body: t.common.publishToUpdate }); } catch (e) { error(e); } finally { setSaving(false); }
        }}>{saving ? <Spinner /> : <Save size={15} />} {t.settings.saveSettings}</button>
      </div>
    </div>
  );
}

const languageCodes = ["ar", "bg", "ca", "cs", "da", "de", "el", "en", "es", "et", "fi", "fr", "he", "hi", "hr", "hu", "id", "it", "ja", "ko", "lt", "lv", "nl", "no", "pl", "pt", "ro", "ru", "sk", "sl", "sr", "sv", "th", "tr", "uk", "vi", "zh"];

const displayName = (code: string, inLanguage: string) => new Intl.DisplayNames([inLanguage], { type: "language" }).of(code) ?? code;

// languageName in the config is the language's own name, as visitors see it.
const ownLanguage = (code: string): SiteLanguage => ({ code, name: displayName(code, code) });

function Languages() {
  const { refresh, refreshChanges, toast, error } = useStore();
  const [saved, setSaved] = useState<LanguageSettings | null>(null);
  const [list, setList] = useState<SiteLanguage[]>([]);
  const [defaultCode, setDefaultCode] = useState("");
  const [pick, setPick] = useState("");
  const [saving, setSaving] = useState(false);
  const load = () => api.getLanguages().then((loaded) => { setSaved(loaded); setList(loaded.languages); setDefaultCode(loaded.default); }, (problem) => error(problem));
  useEffect(() => { load(); }, []);
  if (!saved) return null;
  const dirty = JSON.stringify([defaultCode, list]) !== JSON.stringify([saved.default, saved.languages]);
  const label = (code: string) => displayName(code, uiLanguage);
  const options = (taken: string[]) => languageCodes.filter((code) => !taken.includes(code)).map((code) => ({ code, label: label(code) })).sort((a, b) => a.label.localeCompare(b.label, uiLanguage));
  const move = (index: number, by: number) => {
    const next = [...list];
    [next[index], next[index + by]] = [next[index + by], next[index]];
    setList(next);
  };
  const chooseDefault = (code: string) => {
    setList(list.map((entry) => (entry.code === defaultCode ? ownLanguage(code) : entry)).filter((entry, index, all) => all.findIndex((other) => other.code === entry.code) === index));
    setDefaultCode(code);
  };
  const save = async () => {
    setSaving(true);
    try {
      await api.saveLanguages(defaultCode, list);
      await refresh();
      await load();
      refreshChanges();
      toast({ kind: "success", title: t.settings.languagesSaved, body: t.common.publishToUpdate });
    } catch (problem) { error(problem, t.settings.couldntSaveLanguages); } finally { setSaving(false); }
  };
  return (
    <div className="card card-pad">
      <h3>{t.settings.siteLanguages}</h3>
      <p className="muted small" style={{ marginTop: 2 }}>{t.settings.siteLanguagesHelp}</p>
      {!saved.fixed && (
        <div className="field">
          <label htmlFor="st-default-lang">{t.settings.defaultLanguage}</label>
          <select id="st-default-lang" className="select" value={defaultCode} onChange={(event) => chooseDefault(event.target.value)}>
            {[{ code: defaultCode, label: label(defaultCode) }, ...options([defaultCode])].map((option) => <option key={option.code} value={option.code}>{option.label}</option>)}
          </select>
          <span className="help">{t.settings.defaultLanguageHelp}</span>
        </div>
      )}
      <div className="stack" style={{ gap: 6 }}>
        {list.map((entry, index) => (
          <div className="row" key={entry.code} style={{ gap: 8 }}>
            <span style={{ flex: 1 }}>{entry.name} <span className="mono dim small">{entry.code}</span></span>
            {entry.code === defaultCode && <span className="pill">{t.settings.defaultLanguageBadge}</span>}
            <button className="btn sm ghost" aria-label={t.settings.moveLanguageUp(entry.name)} disabled={index === 0} onClick={() => move(index, -1)}><ArrowUp size={14} /></button>
            <button className="btn sm ghost" aria-label={t.settings.moveLanguageDown(entry.name)} disabled={index === list.length - 1} onClick={() => move(index, 1)}><ArrowDown size={14} /></button>
            <button className="btn sm ghost danger" aria-label={t.settings.removeLanguage(entry.name)} disabled={entry.code === defaultCode} onClick={() => setList(list.filter((other) => other.code !== entry.code))}><X size={14} /></button>
          </div>
        ))}
      </div>
      <div className="field" style={{ marginTop: 12 }}>
        <label htmlFor="st-add-lang">{t.settings.addLanguage}</label>
        <div className="row" style={{ gap: 6 }}>
          <select id="st-add-lang" className="select" value={pick} onChange={(event) => setPick(event.target.value)}>
            <option value="">{t.settings.chooseLanguage}</option>
            {options(list.map((entry) => entry.code)).map((option) => <option key={option.code} value={option.code}>{option.label}</option>)}
          </select>
          <button className="btn sm" disabled={!pick} onClick={() => { setList([...list, ownLanguage(pick)]); setPick(""); }}><Plus size={14} /> {t.settings.addLanguageButton}</button>
        </div>
      </div>
      <div className="row">
        <div className="spacer" />
        <button className="btn primary" disabled={!dirty || saving} onClick={save}>{saving ? <Spinner /> : <Save size={15} />} {t.settings.saveLanguages}</button>
      </div>
    </div>
  );
}

function SiteIconField() {
  const { site, toast, error, refreshChanges } = useStore();
  const [icon, setIcon] = useState<SiteIcon | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => { api.getSiteIcon().then(setIcon, (e) => error(e)); }, [site?.settings]);
  const change = async (load: () => Promise<SiteIcon | null>) => {
    setBusy(true);
    try {
      const next = await load();
      if (!next) return;
      setIcon(next);
      refreshChanges();
      toast({ kind: "success", title: t.settings.iconSaved, body: t.common.publishToUpdate });
    } catch (e) { error(e); } finally { setBusy(false); }
  };
  return (
    <div className="field">
      <label>{t.settings.siteIcon}</label>
      <div className="row" style={{ gap: 12 }}>
        {icon && <img src={icon.image} width={32} height={32} alt="" style={{ borderRadius: 4 }} />}
        <button className="btn sm" disabled={busy} onClick={() => change(api.chooseSiteIcon)}>{busy ? <Spinner /> : <ImagePlus size={14} />} {t.settings.chooseIcon}</button>
        {icon && !icon.letter && <button className="btn sm ghost" disabled={busy} onClick={() => change(api.useLetterIcon)}>{t.settings.useLetterIcon}</button>}
      </div>
      <span className="help">{t.settings.siteIconHelp}</span>
    </div>
  );
}

function GitHubPagesCard({ pages }: { pages: GitHubPages }) {
  const { refresh, refreshChanges, error, toast } = useStore();
  const stop = async () => {
    const sure = await ask.confirm(t.settings.stopPagesTitle, t.settings.stopPagesBody(pages.address), t.settings.removePagesSetup, true);
    if (!sure) return;
    try {
      await api.removeHosting("github-pages");
      await refresh();
      toast({ kind: "success", title: t.settings.pagesRemoved, body: t.settings.pagesRemovedBody, action: { label: t.settings.openPagesSettings, run: () => api.openExternal(pages.settings) } });
    } catch (e) { error(e, t.settings.couldntRemovePages); }
  };
  const [busy, setBusy] = useState(false);
  const setUp = async () => {
    setBusy(true);
    try {
      await api.setUpGitHubPages();
      await refresh();
      refreshChanges();
    } catch (e) {
      error(e, t.settings.couldntSetUpPages);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="card card-pad">
      <h3>{t.settings.pagesTitle}</h3>
      {pages.setUp ? (
        <ol className="muted small" style={{ paddingLeft: 18, lineHeight: 1.8, margin: "6px 0 0" }}>
          <li>
            {t.settings.pagesSource}
            <div><button className="btn sm ghost" onClick={() => api.openExternal(pages.settings)}><ExternalLink size={13} /> {t.settings.openPagesSettings}</button></div>
          </li>
          <li>
            {t.settings.pagesPublish(<span className="mono">{pages.address}</span>)}
            <div><button className="btn sm ghost" onClick={() => api.openExternal(pages.address)}><ExternalLink size={13} /> {t.settings.visitSite}</button></div>
          </li>
          <li style={{ listStyle: "none", marginLeft: -18 }}><button className="btn sm ghost danger" onClick={stop}>{t.settings.stopPages}</button></li>
        </ol>
      ) : (
        <div className="row" style={{ marginTop: 8 }}>
          <p className="muted small" style={{ flex: 1, margin: 0 }}>{t.settings.pagesIntro(<span className="mono">{pages.address}</span>)}</p>
          <button className="btn primary sm" disabled={busy} onClick={setUp}>{busy ? <Spinner /> : <Rocket size={14} />} {t.settings.setUpPages}</button>
        </div>
      )}
    </div>
  );
}

function CloudflareCard({ cloudflare, liveURL }: { cloudflare: Cloudflare; liveURL: string }) {
  const { site, refresh, error, toast } = useStore();
  const stop = async () => {
    const sure = await ask.confirm(t.settings.stopCloudflareTitle, t.settings.stopCloudflareBody(cloudflare.name), t.settings.removeCloudflareSetup, true);
    if (!sure) return;
    try {
      await api.removeHosting("cloudflare");
      await refresh();
      toast({ kind: "success", title: t.settings.cloudflareRemoved, body: t.settings.cloudflareRemovedBody, action: { label: t.settings.openCloudflare, run: () => api.openExternal("https://dash.cloudflare.com/?to=/:account/workers-and-pages") } });
    } catch (e) { error(e, t.settings.couldntRemoveCloudflare); }
  };
  const [busy, setBusy] = useState(false);
  const [address, setAddress] = useState("");
  const setUp = async () => {
    setBusy(true);
    try {
      await api.setUpCloudflare();
      await refresh();
    } catch (e) {
      error(e, t.settings.couldntSetUpCloudflare);
    } finally {
      setBusy(false);
    }
  };
  const saveAddress = async () => {
    if (!site) return;
    try {
      await api.saveSettings({ ...site.settings, baseURL: webAddress(address) });
      await refresh();
      toast({ kind: "success", title: t.settings.addressSaved, body: t.settings.addressSavedBody });
    } catch (e) { error(e); }
  };
  return (
    <div className="card card-pad">
      <h3>{t.settings.cloudflareTitle}</h3>
      {cloudflare.setUp ? (
        <ol className="muted small" style={{ paddingLeft: 18, lineHeight: 1.8, margin: "6px 0 0" }}>
          <li>{t.settings.cloudflarePublish}</li>
          <li>
            {t.settings.cloudflareSignIn}
            <div><button className="btn sm ghost" onClick={() => api.openExternal("https://dash.cloudflare.com/?to=/:account/workers-and-pages")}><ExternalLink size={13} /> {t.settings.openCloudflare}</button></div>
          </li>
          <li>{t.settings.cloudflareCompute}</li>
          <li>{t.settings.cloudflareCreate}</li>
          <li>{t.settings.cloudflareConnect}</li>
          <li>{t.settings.cloudflareName(<span className="mono">{cloudflare.name}</span>)}</li>
          <li>{t.settings.cloudflareDeploy}</li>
          <li>
            {liveURL ? (
              <>{t.settings.liveAt(<span className="mono">{liveURL}</span>)}<div><button className="btn sm ghost" onClick={() => api.openExternal(liveURL)}><ExternalLink size={13} /> {t.settings.visitSite}</button></div></>
            ) : (
              <>
                {t.settings.pasteAddress(<span className="mono">.workers.dev</span>)}
                <div className="row" style={{ gap: 6, marginTop: 4 }}>
                  <input className="input mono" placeholder={`https://${cloudflare.name}.you.workers.dev`} value={address} onChange={(e) => setAddress(e.target.value)} />
                  <button className="btn sm" disabled={!/^\S+\.\S+$/.test(address.trim())} onClick={saveAddress}>{t.common.save}</button>
                </div>
              </>
            )}
          </li>
          <li style={{ listStyle: "none", marginLeft: -18 }}><button className="btn sm ghost danger" onClick={stop}>{t.settings.stopCloudflare}</button></li>
        </ol>
      ) : (
        <div className="row" style={{ marginTop: 8 }}>
          <p className="muted small" style={{ flex: 1, margin: 0 }}>{t.settings.cloudflareIntro}</p>
          <button className="btn primary sm" disabled={busy} onClick={setUp}>{busy ? <Spinner /> : <Rocket size={14} />} {t.settings.setUpCloudflare}</button>
        </div>
      )}
    </div>
  );
}

// Cloudflare's token page, filled in with the one permission Pageroom needs.
const cloudflareTokenPage = "https://dash.cloudflare.com/profile/api-tokens?" + new URLSearchParams({
  permissionGroupKeys: JSON.stringify([{ key: "workers_scripts", type: "edit" }]),
  accountId: "*",
  zoneId: "all",
  name: "Pageroom",
});

function CloudflareTokenForm({ onConnected }: { onConnected: () => void }) {
  const error = useStore((state) => state.error);
  const [token, setToken] = useState("");
  const [accounts, setAccounts] = useState<CloudflareAccount[]>([]);
  const [account, setAccount] = useState("");
  const [busy, setBusy] = useState(false);
  const connect = async () => {
    setBusy(true);
    try {
      const choices = await api.connectCloudflare(token, account);
      if (choices?.length) {
        setAccounts(choices);
        setAccount(choices[0].id);
      } else onConnected();
    } catch (problem) {
      error(problem, t.settings.couldntConnectCloudflare);
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      {accounts.length > 0 && (
        <div className="field" style={{ marginTop: 6, marginBottom: 0 }}>
          <label htmlFor="cf-account">{t.settings.chooseAccount}</label>
          <select id="cf-account" className="select" value={account} onChange={(event) => setAccount(event.target.value)}>
            {accounts.map((choice) => <option key={choice.id} value={choice.id}>{choice.name || choice.id}</option>)}
          </select>
        </div>
      )}
      <div className="row" style={{ gap: 6, marginTop: 4 }}>
        <input className="input mono" type="password" autoComplete="off" placeholder={t.settings.tokenPlaceholder} value={token} onChange={(event) => setToken(event.target.value)} />
        <button className="btn primary sm" disabled={busy || !token.trim()} onClick={connect}>{busy && <Spinner />} {t.settings.connect}</button>
      </div>
    </>
  );
}

function GoLiveOnCloudflare() {
  const { refresh, toast } = useStore();
  return (
    <div className="card card-pad">
      <h3>{t.settings.directTitle}</h3>
      <p className="muted small" style={{ marginTop: 2 }}>{t.settings.directIntro}</p>
      <ol className="muted small" style={{ paddingLeft: 18, lineHeight: 1.8, margin: "6px 0 0" }}>
        <li>
          {t.settings.directSignIn}
          <div><button className="btn sm ghost" onClick={() => api.openExternal(cloudflareTokenPage)}><ExternalLink size={13} /> {t.settings.openCloudflare}</button></div>
        </li>
        <li>{t.settings.directCreate}</li>
        <li>
          {t.settings.directPaste}
          <CloudflareTokenForm onConnected={async () => {
            await refresh();
            toast({ kind: "success", title: t.settings.directConnected, body: t.settings.directConnectedBody });
          }} />
        </li>
      </ol>
    </div>
  );
}

function DirectUploadCard({ direct }: { direct: DirectUpload }) {
  const { site, refresh, error, toast } = useStore();
  const [newToken, setNewToken] = useState(false);
  const address = <span className="mono">{direct.address}</span>;
  const stop = async () => {
    const sure = await ask.confirm(t.settings.stopDirectTitle, t.settings.stopDirectBody(direct.address), t.settings.takeOffline, true);
    if (!sure) return;
    try {
      await api.stopCloudflare();
      await refresh();
      toast({ kind: "success", title: t.settings.directStopped });
    } catch (problem) { error(problem, t.settings.couldntStopDirect); }
  };
  return (
    <div className="card card-pad">
      <h3>{t.settings.cloudflareTitle}</h3>
      <p className="muted small" style={{ margin: "6px 0 0" }}>{direct.published ? t.settings.liveAt(address) : t.settings.directFirstPublish(address)}</p>
      {direct.published && <button className="btn sm ghost" onClick={() => api.openExternal(direct.address)}><ExternalLink size={13} /> {t.settings.visitSite}</button>}
      <p className="muted small">{t.settings.directBackup(<span className="mono">{site?.info.root}</span>)}</p>
      {newToken && <CloudflareTokenForm onConnected={() => { setNewToken(false); toast({ kind: "success", title: t.settings.tokenSaved }); }} />}
      <div className="row" style={{ gap: 6, marginTop: 8 }}>
        {!newToken && <button className="btn sm ghost" onClick={() => setNewToken(true)}>{t.settings.replaceToken}</button>}
        <button className="btn sm ghost danger" onClick={stop}>{t.settings.stopCloudflare}</button>
      </div>
    </div>
  );
}

function Publishing() {
  const { site, refresh, error, toast } = useStore();
  const [auth, setAuth] = useState<AuthStatus | null>(null);
  const [signIn, setSignIn] = useState(false);
  const [wp, setWp] = useState(false);
  const [url, setUrl] = useState("");
  const [host, setHost] = useState<"cloudflare" | "github-pages" | "other" | null>(null);
  const remote = site?.git.remote ?? "";
  const direct = site?.git.direct;
  const hostMismatch = host === "github-pages" && !/github\.com[/:]/.test(url) ? t.settings.pagesNeedsGitHub
    : host === "cloudflare" && !/(github|gitlab)\.com[/:]/.test(url) ? t.settings.cloudflareNeedsHost : "";
  const loadAuth = () => api.getAuthStatus(remote).then(setAuth).catch(() => {});
  useEffect(() => { loadAuth(); }, [remote]);
  if (!site) return null;

  if (!site.git.isRepo) {
    return <Banner kind="warn" title={t.settings.notRepoTitle}>{t.settings.notRepoBody}</Banner>;
  }
  return (
    <div className="stack" style={{ gap: 16 }}>
      {direct && <DirectUploadCard direct={direct} />}
      {!remote && !direct && <GoLiveOnCloudflare />}

      {!direct && <div className="card card-pad">
        <h3>{t.settings.repository}</h3>
        {remote ? (
          <>
            <p className="muted small" style={{ marginTop: 2 }}>{t.settings.repositoryHelp}</p>
            <div className="row"><Globe size={15} className="dim" /><span className="mono truncate" style={{ flex: 1 }}>{remote}</span><span className="pill">{site.git.branch}</span>
              <button className="btn sm ghost danger" onClick={async () => {
                const hosted = site.git.pages?.setUp || site.git.cloudflare?.setUp;
                const sure = await ask.confirm(t.settings.disconnectTitle, t.settings.disconnectBody(remote, !!hosted), t.settings.disconnect, true);
                if (!sure) return;
                try {
                  await api.disconnectRemote();
                  await refresh();
                  toast({ kind: "success", title: t.settings.disconnected, body: t.settings.disconnectedBody });
                } catch (e) { error(e, t.settings.couldntDisconnect); }
              }}>{t.settings.disconnect}</button>
            </div>
          </>
        ) : (
          <>
            <p className="muted small" style={{ marginTop: 2 }}>{t.settings.teamsHint}</p>
            <p className="muted small">
              {t.settings.emptyRepoHelp}
            </p>
            <div className="field">
              <label>{t.settings.whereHosted}</label>
              <div className="seg">
                <button className={host === "cloudflare" ? "on" : ""} onClick={() => setHost("cloudflare")}>{t.settings.cloudflareFree}</button>
                <button className={host === "github-pages" ? "on" : ""} onClick={() => setHost("github-pages")}>{t.settings.pagesFree}</button>
                <button className={host === "other" ? "on" : ""} onClick={() => setHost("other")}>{t.settings.anotherHost}</button>
              </div>
              <span className="help">
                {host === "cloudflare" ? t.settings.hostCloudflare
                  : host === "github-pages" ? t.settings.hostPages
                  : host === "other" ? t.settings.hostOther
                  : t.settings.hostNone}
              </span>
            </div>
            <div className="row">
              <input className="input mono" placeholder="https://github.com/you/your-site" value={url} onChange={(e) => setUrl(e.target.value)} />
              <button className="btn primary" disabled={!host || !/^(https:\/\/|git@)/.test(url.trim()) || !!hostMismatch} onClick={async () => {
                try {
                  await api.connectRemote(url);
                  if (host === "github-pages") await api.setUpGitHubPages();
                  if (host === "cloudflare") await api.setUpCloudflare();
                  await refresh();
                  toast({ kind: "success", title: t.settings.connected, body: t.settings.connectedBody });
                } catch (e) { error(e); }
              }}>{t.settings.connect}</button>
            </div>
            {hostMismatch && url.trim() && <span className="help">{hostMismatch}</span>}
          </>
        )}
      </div>}

      {site.git.cloudflare && !site.git.pages?.setUp && <CloudflareCard cloudflare={site.git.cloudflare} liveURL={/example\.(org|com)/.test(site.settings.baseURL) ? "" : site.settings.baseURL} />}
      {site.git.pages && !site.git.cloudflare?.setUp && <GitHubPagesCard pages={site.git.pages} />}

      {!direct && <div className="card card-pad">
        <h3>{t.settings.signInTitle}</h3>
        {!auth ? <Spinner /> : auth.signedIn ? (
          <div className="row" style={{ marginTop: 8 }}>
            <CheckCircle2 size={18} color="var(--success)" />
            <div style={{ flex: 1 }}>
              <div>{t.settings.signedInTo(auth.host, auth.username ?? "")}</div>
              <div className="dim small"><ShieldCheck size={12} /> {t.settings.keptSecurely(auth.kind === "oauth")}</div>
            </div>
            <button className="btn sm" onClick={async () => { if (await ask.confirm(t.settings.signOutTitle, t.settings.signOutBody(auth.host), t.settings.signOut, true)) { await api.signOut(remote); loadAuth(); } }}><LogOut size={13} /> {t.settings.signOut}</button>
          </div>
        ) : (
          <div className="row" style={{ marginTop: 8 }}>
            <KeyRound size={18} className="dim" />
            <div style={{ flex: 1 }} className="muted">{t.settings.notSignedIn(auth.host)}</div>
            <button className="btn primary sm" onClick={() => setSignIn(true)}>{t.common.signIn}</button>
          </div>
        )}
      </div>}

      <div className="card card-pad">
        <h3>{t.settings.safeTitle}</h3>
        <ul className="muted small" style={{ paddingLeft: 18, lineHeight: 1.8, margin: "6px 0 0" }}>
          <li>{t.settings.safeLocal}</li>
          {!direct && <li>{t.settings.safeCombined}</li>}
          {!direct && <li>{t.settings.safeChoose}</li>}
          <li>{t.settings.safeSecrets}</li>
          <li>{t.settings.safeHistory}</li>
        </ul>
      </div>

      <div className="card card-pad row">
        <Download size={18} className="dim" />
        <div style={{ flex: 1 }}>
          <h3>{t.settings.wordpressTitle}</h3>
          <div className="dim small">{t.settings.wordpressHint}</div>
        </div>
        <button className="btn sm" onClick={() => setWp(true)}>{t.settings.importEllipsis}</button>
      </div>
      {wp && <WordPressImport mode="existing" onClose={() => setWp(false)} onImported={() => refresh()} />}
      {signIn && <SignInDialog remote={remote} onClose={() => setSignIn(false)} onDone={() => { setSignIn(false); loadAuth(); refresh(); }} />}
    </div>
  );
}

export function Account() {
  const { toast, error, set } = useStore();
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [savedLanguage, setSavedLanguage] = useState("");
  const [version, setVersion] = useState("");
  const [loaded, setLoaded] = useState(false);
  useEffect(() => { api.getWelcome().then((w) => { setName(w.name); setEmail(w.email); setSavedLanguage(w.language); setVersion(w.version); setLoaded(true); }).catch(() => {}); }, []);
  if (!loaded) return <Spinner />;
  const chooseLanguage = async (choice: string) => {
    try {
      await api.saveLanguage(choice);
      set({ language: setLanguage(choice) });
    } catch (e) { error(e); }
  };
  return (
    <div className="stack" style={{ gap: 16 }}>
      <div className="card card-pad">
        <p className="muted small" style={{ marginTop: 0 }}>{t.settings.accountHelp}</p>
        <div className="field"><label htmlFor="ac-name">{t.settings.yourName}</label><input id="ac-name" className="input" value={name} onChange={(e) => setName(e.target.value)} /></div>
        <div className="field"><label htmlFor="ac-email">{t.settings.email}</label><input id="ac-email" className="input" value={email} onChange={(e) => setEmail(e.target.value)} /></div>
        <div className="row"><div className="spacer" /><button className="btn primary" disabled={!name.trim()} onClick={async () => { try { await api.saveIdentity(name, email); toast({ kind: "success", title: t.common.saved }); } catch (e) { error(e); } }}><Save size={15} /> {t.common.save}</button></div>
      </div>
      <div className="card card-pad">
        <div className="field" style={{ marginBottom: 0 }}>
          <label htmlFor="ac-language">{t.language.label}</label>
          <select id="ac-language" className="select" value={savedLanguage} onChange={(e) => chooseLanguage(e.target.value)}>
            <option value="">{t.language.system(languageNames[systemLanguage])}</option>
            {(Object.keys(languageNames) as Language[]).map((code) => <option key={code} value={code}>{languageNames[code]}</option>)}
          </select>
          <span className="help">{t.language.help}</span>
        </div>
      </div>
      <p className="muted small" style={{ margin: 0 }}>{t.settings.version(version)}</p>
    </div>
  );
}
