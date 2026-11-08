import { lazy, Suspense, useEffect, useRef, useState } from "react";
import { AlertTriangle, ArrowLeft, Check, Code2, ExternalLink, LayoutTemplate, Paintbrush, RotateCcw, Save, Search, Sparkles, Undo2 } from "lucide-react";
import { api, siteFileURL, type AppError, type GalleryTheme, type SiteFont, type Style, type StyleState, type ThemeInfo, type Tokens } from "../api";
import { useStore } from "../store";
import { ask, Banner, Dialog, Spinner } from "../ui";
import { HugoDownloadPrompt, PreviewPane } from "./Preview";
import { language, t } from "../i18n";


const CodeEditor = lazy(() => import("./CodeEditor"));

const fontKeys = ["system", "humanist", "geometric", "classical", "oldstyle", "transitional", "didone", "slab", "rounded", "mono"];
const empty: Tokens = { primary: "", background: "", text: "", heading: "", headingFont: "", bodyFont: "", fontSize: 0, contentWidth: 0, radius: -1 };

export function AppearanceView({ tab }: { tab: "style" | "css" | "themes" }) {
  const go = useStore((s) => s.go);
  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
      <div className="editor-toolbar">
        <div className="seg">
          <button className={tab === "style" ? "on" : ""} onClick={() => go({ kind: "appearance", tab: "style" })}><Paintbrush size={13} /> {t.appearance.style}</button>
          <button className={tab === "css" ? "on" : ""} onClick={() => go({ kind: "appearance", tab: "css" })}><Code2 size={13} /> {t.appearance.customCss}</button>
          <button className={tab === "themes" ? "on" : ""} onClick={() => go({ kind: "appearance", tab: "themes" })}><LayoutTemplate size={13} /> {t.appearance.themes}</button>
        </div>
      </div>
      <div style={{ flex: 1, minHeight: 0, overflow: tab === "themes" ? "auto" : undefined }}>
        {tab === "themes" ? <Themes /> : <StyleEditor mode={tab} />}
      </div>
    </div>
  );
}

function StyleEditor({ mode }: { mode: "style" | "css" }) {
  const { error, refreshChanges } = useStore();
  const [st, setSt] = useState<StyleState | null>(null);
  const [style, setStyle] = useState<Style | null>(null);
  const [saved, setSaved] = useState<Style | null>(null);
  const [tick, setTick] = useState(0);
  const [saving, setSaving] = useState(false);
  const timer = useRef<number | undefined>(undefined);

  useEffect(() => {
    api.getStyle().then((s) => { setSt(s); setStyle(s.style); setSaved(s.style); }).catch(error);
  }, []);

  // Changes apply to the preview right away (saved to disk, not yet published).
  const persist = async (s: Style) => {
    setSaving(true);
    try {
      await api.saveStyle(s);
      setSaved(s);
      setTick((t) => t + 1);
      refreshChanges();
    } catch (e) { error(e, t.appearance.couldntApply); } finally { setSaving(false); }
  };
  const update = (s: Style, delay = 500) => {
    setStyle(s);
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => persist(s), delay);
  };
  const tok = (k: keyof Tokens, v: any) => style && update({ ...style, tokens: { ...style.tokens, [k]: v } });

  if (!st || !style) return <div className="page-pad"><Spinner /></div>;
  const tokens = style.tokens;
  const dirty = JSON.stringify(style) !== JSON.stringify(saved);

  return (
    <div className="appearance-layout" style={mode === "css" ? { gridTemplateColumns: "minmax(380px, 1fr) minmax(380px, 1fr)" } : undefined}>
      <div className="appearance-controls" style={mode === "css" ? { padding: 0, display: "flex", flexDirection: "column" } : undefined}>
        {!st.hook.supported && (
          <div style={mode === "css" ? { padding: 16 } : undefined}>
            <Banner kind="warn" title={t.appearance.noHook}>
              <div className="small">{st.hook.suggestion}</div>
            </Banner>
          </div>
        )}
        {mode === "style" ? (
          <>
            <h3 style={{ fontSize: 15, marginBottom: 4 }}>{t.appearance.colors}</h3>
            <p className="dim small" style={{ marginTop: 0 }}>{t.appearance.colorsHelp}</p>
            <ColorRow label={t.appearance.mainColor} hint={t.appearance.mainColorHint} value={tokens.primary} onChange={(v) => tok("primary", v)} />
            <ColorRow label={t.appearance.background} value={tokens.background} onChange={(v) => tok("background", v)} />
            <ColorRow label={t.appearance.text} value={tokens.text} onChange={(v) => tok("text", v)} />
            <ColorRow label={t.appearance.headings} value={tokens.heading} onChange={(v) => tok("heading", v)} />

            <h3 style={{ fontSize: 15, margin: "22px 0 10px" }}>{t.appearance.fontsTitle}</h3>
            <div className="label" style={{ marginBottom: 6 }}>{t.appearance.headings}</div>
            <FontPicker fonts={st.fonts} siteFonts={tokens.fonts ?? []} value={tokens.headingFont} onChange={(v) => tok("headingFont", v)} />
            <div className="label" style={{ margin: "14px 0 6px" }}>{t.appearance.bodyText}</div>
            <FontPicker fonts={st.fonts} siteFonts={tokens.fonts ?? []} value={tokens.bodyFont} onChange={(v) => tok("bodyFont", v)} />
            <p className="dim small">{t.appearance.fontsHelp}</p>

            <h3 style={{ fontSize: 15, margin: "22px 0 10px" }}>{t.appearance.layout}</h3>
            <RangeRow label={t.appearance.textSize} unit="px" min={14} max={22} value={tokens.fontSize || 0} onChange={(v) => tok("fontSize", v)} />
            <RangeRow label={t.appearance.contentWidth} unit="px" min={560} max={1400} step={20} value={tokens.contentWidth || 0} onChange={(v) => tok("contentWidth", v)} />
            <RangeRow label={t.appearance.roundedCorners} unit="px" min={0} max={24} value={tokens.radius} unsetValue={-1} onChange={(v) => tok("radius", v)} />

            <div className="row" style={{ marginTop: 22 }}>
              <button className="btn" onClick={async () => { if (await ask.confirm(t.appearance.resetTitle, t.appearance.resetBody, t.common.reset, true)) update({ ...style, tokens: { ...empty } }, 0); }}><RotateCcw size={14} /> {t.appearance.resetToDefaults}</button>
              <div className="spacer" />
              <span className="dim small">{saving ? <><Spinner size={12} /> {t.appearance.applying}</> : dirty ? "…" : <><Check size={12} /> {t.appearance.applied}</>}</span>
            </div>
          </>
        ) : (
          <>
            <div className="row" style={{ padding: "12px 16px", borderBottom: "1px solid var(--line)" }}>
              <div style={{ flex: 1 }}>
                <div style={{ fontWeight: 600 }}>{t.appearance.customCss}</div>
                <div className="dim small">{t.appearance.cssHelp}</div>
              </div>
              <button className="btn primary sm" disabled={!dirty || saving} onClick={() => persist(style)}>{saving ? <Spinner size={13} /> : <Save size={13} />} {t.appearance.apply}</button>
            </div>
            <div style={{ flex: 1, minHeight: 0, overflow: "auto" }}>
              <Suspense fallback={<div className="page-pad"><Spinner /></div>}>
                <CodeEditor value={style.customCSS} onChange={(v) => setStyle({ ...style, customCSS: v })} onSave={() => persist(style)} />
              </Suspense>
            </div>
          </>
        )}
      </div>
      <PreviewPane pageURL="/" tick={tick} />
    </div>
  );
}

function ColorRow({ label, hint, value, onChange }: { label: string; hint?: string; value: string; onChange: (v: string) => void }) {
  return (
    <div className="color-row">
      <label className={"swatch" + (value ? "" : " unset")} style={value ? { background: value } : undefined} aria-label={t.appearance.colorLabel(label)}>
        <input type="color" value={value || "#2f5bd3"} onChange={(e) => onChange(e.target.value)} style={{ opacity: 0 }} />
      </label>
      <div style={{ flex: 1 }}>
        <div style={{ fontWeight: 550 }}>{label}</div>
        <div className="dim small">{value ? value.toUpperCase() : hint ? t.appearance.hintDefault(hint) : t.appearance.themeDefault}</div>
      </div>
      {value && <button className="btn ghost sm" onClick={() => onChange("")}>{t.common.reset}</button>}
    </div>
  );
}

export function loadSiteFonts(siteFonts: SiteFont[]) {
  const faces = siteFonts.flatMap((font) => font.files.map((file) =>
    new FontFace(font.family, `url("${siteFileURL("static/" + file.path)}")`, { weight: file.weight, style: file.style, unicodeRange: file.unicodeRange })));
  faces.forEach((face) => {
    document.fonts.add(face);
    face.load().catch(() => {});
  });
  return () => faces.forEach((face) => document.fonts.delete(face));
}

function FontPicker({ fonts, siteFonts, value, onChange }: { fonts: Record<string, string>; siteFonts: SiteFont[]; value: string; onChange: (v: string) => void }) {
  useEffect(() => loadSiteFonts(siteFonts), [siteFonts]);
  return (
    <div className="font-options">
      <button className={"font-option" + (!value ? " on" : "")} onClick={() => onChange("")}>
        <div className="fo-sample">Aa</div><div className="fo-name">{t.appearance.themeDefault}</div>
      </button>
      {fontKeys.filter((k) => fonts[k]).map((k) => (
        <button key={k} className={"font-option" + (value === k ? " on" : "")} onClick={() => onChange(k)}>
          <div className="fo-sample" style={{ fontFamily: fonts[k] }}>Aa</div><div className="fo-name">{t.appearance.fonts[k]}</div>
        </button>
      ))}
      {siteFonts.map((f) => (
        <button key={f.family} className={"font-option" + (value === f.family ? " on" : "")} onClick={() => onChange(f.family)}>
          <div className="fo-sample" style={{ fontFamily: `"${f.family}", ${fonts[f.fallback] ?? fonts.system}` }}>Aa</div><div className="fo-name">{f.family}</div>
        </button>
      ))}
    </div>
  );
}

export function RangeRow({ label, unit, min, max, step = 1, value, onChange, unsetValue = 0 }: { label: string; unit: string; min: number; max: number; step?: number; value: number; onChange: (v: number) => void; unsetValue?: number }) {
  const set = value !== unsetValue;
  return (
    <div className="field" style={{ marginBottom: 12 }}>
      <div className="row"><span className="label" style={{ flex: 1 }}>{label}</span><span className="dim small">{set ? `${value}${unit}` : t.appearance.themeDefault}</span>{set && <button className="btn ghost sm" onClick={() => onChange(unsetValue)}>{t.common.reset}</button>}</div>
      <div className="range-row"><input type="range" min={min} max={max} step={step} value={set ? value : Math.round((min + max) / 2 / step) * step} onChange={(e) => onChange(Number(e.target.value))} aria-label={label} /></div>
    </div>
  );
}

const galleryFilters = {
  blog: ["blog"],
  portfolio: ["portfolio", "gallery", "photography"],
  business: ["business", "company", "corporate", "landing", "landing page", "startup"],
  documentation: ["docs", "documentation"],
  personal: ["personal", "resume", "cv"],
};
type GalleryFilter = keyof typeof galleryFilters;

function Themes() {
  const { error, refresh, toast } = useStore();
  const [themes, setThemes] = useState<ThemeInfo[] | null>(null);
  const [browsing, setBrowsing] = useState(false);
  const [trial, setTrial] = useState<GalleryTheme | null>(null);
  const [checking, setChecking] = useState<string | null>(null);
  const [problem, setProblem] = useState<{ theme: string; text: string } | null>(null);
  const [needHugo, setNeedHugo] = useState<{ err: AppError; retry: () => void } | null>(null);
  const problemRef = useRef<HTMLDivElement>(null);
  const load = () => api.listThemes().then((list) => setThemes(list ?? [])).catch(error);
  useEffect(() => { load(); }, []);
  useEffect(() => { problemRef.current?.scrollIntoView({ behavior: "smooth", block: "center" }); }, [problem]);

  const activate = async (name: string) => {
    setChecking(name);
    setProblem(null);
    try {
      const problems = await api.checkTheme(name);
      if (problems) { setProblem({ theme: name, text: problems }); return; }
      const active = themes?.find((theme) => theme.active)?.name;
      if (!(await ask.confirm(t.appearance.switchTitle(name), t.appearance.switchBody(active ?? ""), t.appearance.switchTheme))) return;
      await api.switchTheme(name);
      toast({ kind: "success", title: t.appearance.nowUsing(name), body: t.appearance.previewFirst });
      await load();
      await refresh();
    } catch (e) {
      // The test build needs Hugo. Ask before downloading it, then retry.
      if ((e as AppError).code === "hugo-download") setNeedHugo({ err: e as AppError, retry: () => activate(name) });
      else error(e);
    } finally { setChecking(null); }
  };

  const tryTheme = async (theme: GalleryTheme) => {
    setChecking(theme.name);
    setProblem(null);
    try {
      const problems = await api.tryTheme(theme.name);
      if (problems) { setProblem({ theme: theme.title, text: problems }); return; }
      setTrial(theme);
      await refresh();
    } catch (e) {
      if ((e as AppError).code === "hugo-download") setNeedHugo({ err: e as AppError, retry: () => tryTheme(theme) });
      else error(e, t.appearance.couldntTry);
    } finally { setChecking(null); }
  };

  const endTrial = async (keep: boolean) => {
    if (!trial) return;
    try {
      await api.endThemeTrial(keep);
      toast(keep
        ? { kind: "success", title: t.appearance.nowUsing(trial.title), body: t.appearance.publishWhenReady }
        : { kind: "success", title: t.appearance.backToBefore });
      setTrial(null);
      setBrowsing(false);
      await load();
      await refresh();
    } catch (e) { error(e); }
  };

  if (trial) return <ThemeTrial theme={trial} onEnd={endTrial} />;

  return (
    <div className="page-pad">
      <Banner kind="warn" title={t.appearance.changingWarning}>
        <span className="small">{t.appearance.changingWarningBody}</span>
      </Banner>
      {problem && (
        <div ref={problemRef}>
          <Banner kind="danger" title={t.appearance.doesntBuild(problem.theme)}>
            <div className="small">{t.appearance.doesntBuildBody}</div>
            <pre className="mono small selectable" style={{ whiteSpace: "pre-wrap", maxHeight: 180, overflow: "auto" }}>{problem.text}</pre>
          </Banner>
        </div>
      )}
      {needHugo && (
        <Dialog open onClose={() => setNeedHugo(null)} title={t.appearance.testingNeedsHugo}>
          <HugoDownloadPrompt need={needHugo.err} onCancel={() => setNeedHugo(null)} onReady={() => { const retry = needHugo.retry; setNeedHugo(null); retry(); }} />
        </Dialog>
      )}
      {browsing ? (
        <Gallery installed={themes?.map((theme) => theme.name) ?? []} busy={checking} onTry={tryTheme} onSwitch={activate} onBack={() => setBrowsing(false)} />
      ) : (
        <>
          <div className="row" style={{ margin: "18px 0 12px" }}>
            <h3 style={{ flex: 1, margin: 0, fontSize: 15 }}>{t.appearance.installed}</h3>
            <button className="btn primary" onClick={() => setBrowsing(true)}><Sparkles size={14} /> {t.appearance.getMore}</button>
          </div>
          {!themes ? <Spinner /> : themes.length === 0 ? (
            <div className="card empty"><h3>{t.appearance.noneInstalled}</h3><p>{t.appearance.noneInstalledBody}</p></div>
          ) : (
            <div className="grid cols-3">
              {themes.map((theme) => (
                <div key={theme.name} className="card theme-card">
                  <div className="tc-shot">{theme.screenshot ? <img src={siteFileURL(theme.screenshot)} alt="" /> : <LayoutTemplate size={32} />}</div>
                  <div className="card-pad">
                    <div className="row"><h3 style={{ flex: 1 }}>{theme.name}</h3>{theme.active && <span className="pill live">{t.appearance.active}</span>}</div>
                    {theme.description && <p className="dim small" style={{ margin: "4px 0 10px" }}>{theme.description}</p>}
                    {!theme.active && (
                      <button className="btn sm" onClick={() => activate(theme.name)} disabled={!!checking}>
                        {checking === theme.name ? <><Spinner size={13} /> {t.appearance.testing}</> : <><AlertTriangle size={13} /> {t.appearance.switchToThis}</>}
                      </button>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}

function Gallery({ installed, busy, onTry, onSwitch, onBack }: { installed: string[]; busy: string | null; onTry: (theme: GalleryTheme) => Promise<void>; onSwitch: (name: string) => void; onBack: () => void }) {
  const { error } = useStore();
  const [all, setAll] = useState<GalleryTheme[] | null>(null);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<GalleryFilter | "">("");
  const [open, setOpen] = useState<GalleryTheme | null>(null);
  // Recently updated themes first: they're the most likely to keep working.
  useEffect(() => { api.galleryThemes().then((list) => setAll([...list].sort((left, right) => right.updated.localeCompare(left.updated)))).catch(error); }, []);

  const words = query.trim().toLowerCase();
  const shown = (all ?? []).filter((theme) => {
    const tags = (theme.tags ?? []).map((tag) => tag.toLowerCase());
    const text = [theme.title, theme.description, theme.author, ...tags].join(" ").toLowerCase();
    return (!filter || galleryFilters[filter].some((tag) => tags.includes(tag))) && (!words || text.includes(words));
  });

  return (
    <>
      <div className="row" style={{ margin: "18px 0 14px", flexWrap: "wrap" }}>
        <button className="btn ghost sm" onClick={onBack}><ArrowLeft size={14} /> {t.appearance.installed}</button>
        <div className="seg">
          {(["", ...Object.keys(galleryFilters)] as (GalleryFilter | "")[]).map((name) => (
            <button key={name} className={filter === name ? "on" : ""} onClick={() => setFilter(name)}>{t.appearance.filters[name || "all"]}</button>
          ))}
        </div>
        <div className="spacer" />
        <div className="tree-search" style={{ width: 240, flex: "none" }}>
          <Search size={15} />
          <input className="input" placeholder={t.appearance.search} value={query} onChange={(e) => setQuery(e.target.value)} />
        </div>
      </div>
      {!all ? <Spinner /> : shown.length === 0 ? (
        <div className="card empty"><h3>{t.appearance.noMatch}</h3><p>{t.appearance.noMatchHint}</p></div>
      ) : (
        <div className="grid cols-3">
          {shown.map((theme) => (
            <button key={theme.name} className="card theme-card gallery-card" onClick={() => setOpen(theme)}>
              <div className="tc-shot"><img src={theme.thumbnail} alt="" loading="lazy" /></div>
              <div className="card-pad">
                <div className="row"><h3 style={{ flex: 1 }}>{theme.title}</h3>{installed.includes(theme.name) && <span className="pill">{t.appearance.installedPill}</span>}</div>
                {theme.author && <div className="dim small">{t.appearance.by(theme.author)}</div>}
                {theme.description && <p className="dim small clamp-2" style={{ margin: "6px 0 0" }}>{theme.description}</p>}
              </div>
            </button>
          ))}
        </div>
      )}
      {open && (
        <ThemeDetails
          theme={open}
          installed={installed.includes(open.name)}
          busy={busy}
          onTry={async () => { await onTry(open); setOpen(null); }}
          onSwitch={() => { setOpen(null); onSwitch(open.name); }}
          onClose={() => !busy && setOpen(null)}
        />
      )}
    </>
  );
}

function ThemeDetails({ theme, installed, busy, onTry, onSwitch, onClose }: { theme: GalleryTheme; installed: boolean; busy: string | null; onTry: () => void; onSwitch: () => void; onClose: () => void }) {
  const updated = new Date(theme.updated).toLocaleDateString(language, { month: "long", year: "numeric" });
  return (
    <Dialog
      open
      wide
      onClose={onClose}
      title={theme.title}
      description={theme.author ? t.appearance.by(theme.author) : undefined}
      actions={
        <>
          {theme.demo && <button className="btn" onClick={() => api.openExternal(theme.demo!)}><ExternalLink size={14} /> {t.appearance.seeDemo}</button>}
          {installed ? (
            <button className="btn primary" disabled={!!busy} onClick={onSwitch}>{t.appearance.switchToThis}</button>
          ) : (
            <button className="btn primary" disabled={!!busy} onClick={onTry}>
              {busy === theme.name ? <><Spinner size={13} /> {t.appearance.downloadingTesting}</> : <><Sparkles size={14} /> {t.appearance.tryWithMyPages}</>}
            </button>
          )}
        </>
      }
    >
      <img src={theme.screenshot ?? theme.thumbnail} alt={t.appearance.screenshotAlt(theme.title)} style={{ width: "100%", borderRadius: 8, border: "1px solid var(--line)" }} />
      {theme.description && <p>{theme.description}</p>}
      <p className="dim small">{t.appearance.licenseUpdated(theme.license, updated)}</p>
      {!installed && <p className="dim small">{t.appearance.tryNote}</p>}
    </Dialog>
  );
}

function ThemeTrial({ theme, onEnd }: { theme: GalleryTheme; onEnd: (keep: boolean) => Promise<void> }) {
  const [ending, setEnding] = useState<"keep" | "back" | null>(null);
  const end = async (keep: boolean) => {
    setEnding(keep ? "keep" : "back");
    await onEnd(keep);
    setEnding(null);
  };
  return (
    <div style={{ display: "grid", gridTemplateRows: "auto 1fr", height: "100%" }}>
      <div className="row" style={{ padding: "12px 16px", borderBottom: "1px solid var(--line)" }}>
        <div style={{ flex: 1 }}>
          <div style={{ fontWeight: 600 }}>{t.appearance.trying(theme.title)}</div>
          <div className="dim small">{t.appearance.tryingHelp}</div>
        </div>
        <button className="btn" disabled={!!ending} onClick={() => end(false)}>{ending === "back" ? <Spinner size={13} /> : <Undo2 size={14} />} {t.appearance.goBack}</button>
        <button className="btn primary" disabled={!!ending} onClick={() => end(true)}>{ending === "keep" ? <Spinner size={13} /> : <Check size={14} />} {t.appearance.keepTheme}</button>
      </div>
      <div style={{ display: "grid", minHeight: 0 }}>
        <PreviewPane pageURL="/" tick={0} />
      </div>
    </div>
  );
}
