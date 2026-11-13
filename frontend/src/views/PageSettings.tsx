import { useMemo, useState } from "react";
import { Calendar, ImagePlus, Link2, Pencil, X, Plus, ChevronDown, ChevronRight } from "lucide-react";
import { api, type FieldDef, type Page } from "../api";
import { useStore } from "../store";
import { ask, Switch } from "../ui";
import { t } from "../i18n";
import { scopedFields } from "./scope";

interface Props {
  page: Page;
  fields: Record<string, any>;
  setField: (k: string, v: any) => void;
  resolveImage: (src: string) => string | null;
  pickImage: () => Promise<{ ref: string; alt?: string } | null>;
  onRenamed: (newPath: string) => void;
  save: () => Promise<void>;
}

// Keys with their own controls, hidden from "Other settings".
const handled = new Set(["title", "draft", "date", "description", "summary", "image", "imageAlt", "tags", "categories", "weight", "slug", "url", "aliases", "menus", "menu", "lastmod", "publishDate"]);

export function PageSettings({ page, fields, setField, resolveImage, pickImage, onRenamed, save }: Props) {
  const site = useStore((s) => s.site);
  const error = useStore((s) => s.error);
  const [more, setMore] = useState(false);
  const section = page.path.split("/")[1] ?? "";
  const scoped = useMemo(() => scopedFields(site?.info.prefs.fields ?? [], page.path), [site, page.path]);
  const custom = useMemo(() => scoped.filter((f) => !handled.has(f.key) && f.type !== "repeat"), [scoped]);
  const customKeys = new Set(scoped.map((f) => f.key));
  const isPost = /^(posts?|blog|news|articles)$/i.test(section);
  const others = Object.keys(fields).filter((k) => !handled.has(k) && !customKeys.has(k));

  const descKey = "description" in fields || !("summary" in fields) ? "description" : "summary";

  return (
    <aside className="side-panel" aria-label={t.pageSettings.label}>
      {page.isSection && page.url === "/" && (
        <div className="side-section">
          <div className="field">
            <label htmlFor="ps-title">{t.pageEditor.pageTitle}</label>
            <input id="ps-title" className="input" value={fields.title ?? ""} onChange={(e) => setField("title", e.target.value)} />
            <span className="help">{t.pageSettings.homeTitleNote}</span>
          </div>
        </div>
      )}
      <div className="side-section">
        <h4>{t.pageSettings.publishing}</h4>
        <p className="help side-note">{t.pageSettings.publishingNote}</p>
        <div className="status-card" style={{ marginBottom: 14 }}>
          <Switch on={!fields.draft} onChange={(v) => setField("draft", v ? undefined : true)} label={t.pageSettings.visibleOnSite} />
          <div>
            <div style={{ fontWeight: 600 }}>{fields.draft ? t.pageSettings.hiddenDraft : t.pageSettings.visibleOnSite}</div>
            <div className="dim small">{fields.draft ? t.pageSettings.onlyYou : t.pageSettings.goesLive}</div>
          </div>
        </div>
        {!page.isSection && (
          <div className="field" style={{ marginBottom: 6 }}>
            <label htmlFor="ps-date"><Calendar size={12} /> {t.pageSettings.date}</label>
            <input id="ps-date" className="input" type="datetime-local" value={toLocalInput(fields.date)} onChange={(e) => setField("date", e.target.value ? new Date(e.target.value).toISOString() : undefined)} />
            {fields.date && Date.parse(fields.date) > Date.now() && <span className="help">{t.pageSettings.scheduled}</span>}
          </div>
        )}
      </div>

      <div className="side-section">
        <h4>{t.pageSettings.address}</h4>
        <p className="help side-note">{t.pageSettings.addressNote}</p>
        <div className="row" style={{ gap: 6 }}>
          <Link2 size={14} className="dim" />
          <span className="mono truncate" style={{ flex: 1 }} title={page.url}>{page.url}</span>
          {!page.isSection && page.path.split("/").length > 2 && (
            <button className="btn sm ghost" onClick={async () => {
              const cur = page.url.replace(/\/$/, "").split("/").pop() ?? "";
              const slug = await ask.prompt(t.pageSettings.changeAddressTitle, t.pageSettings.newAddress, "my-page", cur);
              if (!slug || slug === cur) return;
              try { await save(); onRenamed(await api.renameSlug(page.path, slug)); } catch (e) { error(e); }
            }}><Pencil size={12} /> {t.pageSettings.change}</button>
          )}
        </div>
        {Array.isArray(fields.aliases) && fields.aliases.length > 0 && (
          <div className="help" style={{ marginTop: 8 }}>{t.pageSettings.oldAddresses(fields.aliases.join(", "))}</div>
        )}
      </div>

      <div className="side-section">
        <h4>{t.pageSettings.searchSharing}</h4>
        <p className="help side-note">{t.pageSettings.searchSharingNote}</p>
        <div className="field">
          <label htmlFor="ps-desc">{t.pageSettings.shortDescription}</label>
          <textarea id="ps-desc" className="textarea" rows={3} value={fields[descKey] ?? ""} onChange={(e) => setField(descKey, e.target.value)} placeholder={t.pageSettings.descriptionPlaceholder} maxLength={300} />
          <span className="help">{t.pageSettings.descriptionCount((fields[descKey] ?? "").length)}</span>
        </div>
      </div>

      {(isPost || "tags" in fields || "categories" in fields) && (
        <div className="side-section">
          <h4>{t.pageSettings.organize}</h4>
          <p className="help side-note">{t.pageSettings.organizeNote}</p>
          <ListField label={t.pageSettings.categories} value={fields.categories} onChange={(v) => setField("categories", v)} placeholder={t.pageSettings.addCategory} />
          <ListField label={t.pageSettings.tags} value={fields.tags} onChange={(v) => setField("tags", v)} placeholder={t.pageSettings.addTag} />
        </div>
      )}

      {custom.length > 0 && (
        <div className="side-section">
          <h4>{t.pageSettings.details}</h4>
          <p className="help side-note">{t.pageSettings.detailsNote}</p>
          {custom.map((f) => <CustomField key={f.key} def={f} value={fields[f.key]} onChange={(v) => setField(f.key, v)} resolve={resolveImage} pick={pickImage} />)}
        </div>
      )}

      {others.length > 0 && (
        <div className="side-section">
          <button className="btn ghost sm" style={{ marginLeft: -8 }} onClick={() => setMore(!more)}>
            {more ? <ChevronDown size={14} /> : <ChevronRight size={14} />} {t.pageSettings.otherSettings(others.length)}
          </button>
          {more && (
            <div style={{ marginTop: 10 }}>
              <p className="help" style={{ marginTop: 0 }}>{t.pageSettings.otherSettingsHelp}</p>
              {others.map((k) => <RawField key={k} k={k} value={fields[k]} onChange={(v) => setField(k, v)} />)}
            </div>
          )}
        </div>
      )}
    </aside>
  );
}

function toLocalInput(v: any): string {
  if (!v) return "";
  const t = Date.parse(String(v));
  if (!t) return "";
  const d = new Date(t);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function ImageField({ label, value, onChange, resolve, pick }: { label: string; value: any; onChange: (v: any) => void; resolve: (s: string) => string | null; pick: Props["pickImage"] }) {
  const src = typeof value === "string" ? value : "";
  const url = src ? resolve(src) : null;
  return (
    <div className="field">
      <label>{label}</label>
      <div className="thumb-field">
        {src ? (
          <>
            {url ? <img src={url} alt="" /> : <div className="tf-empty small">{src}</div>}
            <div className="row" style={{ padding: 6 }}>
              <button className="btn sm" onClick={async () => { const r = await pick(); if (r) onChange(r.ref); }}>{t.common.replace}</button>
              <button className="btn sm ghost danger" onClick={() => onChange(undefined)}>{t.common.remove}</button>
            </div>
          </>
        ) : (
          <div className="tf-empty" onClick={async () => { const r = await pick(); if (r) onChange(r.ref); }} role="button" tabIndex={0}>
            <div className="row small"><ImagePlus size={16} /> {t.pageSettings.chooseImage}</div>
          </div>
        )}
      </div>
    </div>
  );
}

function ListField({ label, value, onChange, placeholder }: { label: string; value: any; onChange: (v: any) => void; placeholder: string }) {
  const items: string[] = Array.isArray(value) ? value.map(String) : typeof value === "string" && value ? [value] : [];
  const [draft, setDraft] = useState("");
  const add = () => {
    const v = draft.trim();
    if (v && !items.includes(v)) onChange([...items, v]);
    setDraft("");
  };
  return (
    <div className="field">
      <label>{label}</label>
      <div className="chips">
        {items.map((item) => (
          <span key={item} className="chip">{item}<button aria-label={t.pageSettings.removeItem(item)} onClick={() => onChange(items.filter((x) => x !== item))}><X size={11} /></button></span>
        ))}
      </div>
      <div className="row" style={{ gap: 6 }}>
        <input className="input" value={draft} placeholder={placeholder} onChange={(e) => setDraft(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter" || e.key === ",") { e.preventDefault(); add(); } }} />
        <button className="btn icon" onClick={add} aria-label={t.pageSettings.addItem(label)} disabled={!draft.trim()}><Plus size={14} /></button>
      </div>
    </div>
  );
}

export function CustomField({ def, value, onChange, resolve, pick, idPrefix = "cf-" }: { idPrefix?: string; def: FieldDef; value: any; onChange: (v: any) => void; resolve: Props["resolveImage"]; pick: Props["pickImage"] }) {
  const id = idPrefix + def.key;
  const label = def.label || def.key;
  switch (def.type) {
    case "image":
      return <ImageField label={label} value={value} onChange={onChange} resolve={resolve} pick={pick} />;
    case "list":
      return <ListField label={label} value={value} onChange={onChange} placeholder={t.pageSettings.addCustomItem(label)} />;
    case "boolean":
      return (
        <div className="row" style={{ marginBottom: 14 }}>
          <Switch on={!!value} onChange={(v) => onChange(v || undefined)} label={label} />
          <span>{label}</span>
        </div>
      );
    case "textarea":
      return (
        <div className="field">
          <label htmlFor={id}>{label}</label>
          <textarea id={id} className="textarea" value={value ?? ""} onChange={(e) => onChange(e.target.value)} />
          {def.help && <span className="help">{def.help}</span>}
        </div>
      );
    case "select":
      return (
        <div className="field">
          <label htmlFor={id}>{label}</label>
          <select id={id} className="select" value={value ?? ""} onChange={(e) => onChange(e.target.value || undefined)}>
            <option value="">—</option>
            {def.options?.map((o) => <option key={o}>{o}</option>)}
          </select>
          {def.help && <span className="help">{def.help}</span>}
        </div>
      );
    case "number":
      return (
        <div className="field">
          <label htmlFor={id}>{label}</label>
          <input id={id} className="input" type="number" value={value ?? ""} onChange={(e) => onChange(e.target.value === "" ? undefined : Number(e.target.value))} />
          {def.help && <span className="help">{def.help}</span>}
        </div>
      );
    case "date":
      return (
        <div className="field">
          <label htmlFor={id}>{label}</label>
          <input id={id} className="input" type="date" value={String(value ?? "").slice(0, 10)} onChange={(e) => onChange(e.target.value || undefined)} />
        </div>
      );
    default:
      return (
        <div className="field">
          <label htmlFor={id}>{label}</label>
          <input id={id} className="input" value={value ?? ""} onChange={(e) => onChange(e.target.value)} />
          {def.help && <span className="help">{def.help}</span>}
        </div>
      );
  }
}

function RawField({ k, value, onChange }: { k: string; value: any; onChange: (v: any) => void }) {
  const id = "raw-" + k;
  if (typeof value === "boolean") {
    return <div className="row" style={{ marginBottom: 12 }}><Switch on={value} onChange={onChange} label={k} /><span className="mono">{k}</span></div>;
  }
  if (typeof value === "number") {
    return <div className="field"><label htmlFor={id} className="mono">{k}</label><input id={id} className="input" type="number" value={value} onChange={(e) => onChange(Number(e.target.value))} /></div>;
  }
  if (typeof value === "string") {
    return <div className="field"><label htmlFor={id} className="mono">{k}</label><input id={id} className="input" value={value} onChange={(e) => onChange(e.target.value)} /></div>;
  }
  return (
    <div className="field">
      <label className="mono">{k}</label>
      <pre className="mono small dim" style={{ margin: 0, whiteSpace: "pre-wrap" }}>{JSON.stringify(value, null, 1)}</pre>
    </div>
  );
}
