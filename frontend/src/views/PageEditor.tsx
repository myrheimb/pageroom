import { useCallback, useEffect, useRef, useState } from "react";
import { CheckCircle2, CloudOff, Eye, EyeOff, FolderTree, History, ImagePlus, Loader2, Monitor, MoreHorizontal, PanelRight, Trash2, ExternalLink, RotateCw } from "lucide-react";
import { api, on, resolveImageRef, siteFileURL, translate, type AppError, type MediaItem, type Page, type TreeNode } from "../api";
import { useStore, flatPages } from "../store";
import { ask, Banner, Dialog, Menu, Spinner, Tip, timeAgo } from "../ui";
import { BlockEditor, type BlockEditorHandle } from "../editor/BlockEditor";
import { PageSettings } from "./PageSettings";
import { MediaPicker } from "./MediaPicker";
import { HistoryDialog } from "./History";
import { PreviewPane } from "./Preview";
import { t } from "../i18n";

type SaveState = "saved" | "dirty" | "saving" | "error";

export function PageEditor({ path, onPathChange, onDeleted }: { path: string; onPathChange: (p: string) => void; onDeleted: () => void }) {
  const { site, pageListHidden, set, refreshChanges, registerFlush, error, setTree } = useStore();
  const [page, setPage] = useState<Page | null>(null);
  const [loadErr, setLoadErr] = useState<AppError | null>(null);
  const [title, setTitle] = useState("");
  const [fields, setFields] = useState<Record<string, any>>({});
  const [state, setState] = useState<SaveState>("saved");
  const [savedAt, setSavedAt] = useState<number | null>(null);
  const [conflict, setConflict] = useState(false);
  const [side, setSide] = useState(true);
  const [preview, setPreview] = useState(false);
  const [history, setHistory] = useState(false);
  const [picker, setPicker] = useState<null | ((r: { ref: string; alt?: string } | null) => void)>(null);
  const [previewTick, setPreviewTick] = useState(0);
  const [version, setVersion] = useState(0); // bump to remount the editor with fresh content

  const editorRef = useRef<BlockEditorHandle>(null);
  const pending = useRef<{ set: Record<string, any>; del: Set<string>; body: boolean }>({ set: {}, del: new Set(), body: false });
  const base = useRef("");
  const timer = useRef<number | undefined>(undefined);
  const saving = useRef<Promise<void> | null>(null);

  const load = useCallback(async () => {
    try {
      const p = await api.getPage(path);
      setPage(p);
      setTitle(String(p.fields.title ?? ""));
      setFields(p.fields);
      base.current = p.hash;
      pending.current = { set: {}, del: new Set(), body: false };
      setState("saved");
      setVersion((v) => v + 1);
    } catch (e) {
      setLoadErr(e as AppError);
    }
  }, [path]);
  useEffect(() => { load(); }, [load]);

  const save = useCallback(async (force = false) => {
    window.clearTimeout(timer.current);
    if (saving.current) await saving.current;
    const p = pending.current;
    if (!force && !p.body && !Object.keys(p.set).length && !p.del.size) return;
    const req = {
      path,
      baseHash: base.current,
      set: { ...p.set },
      delete: Array.from(p.del),
      body: p.body ? editorRef.current?.getMarkdown() : undefined,
      force,
    };
    pending.current = { set: {}, del: new Set(), body: false };
    setState("saving");
    const run = (async () => {
      try {
        const np = await api.savePage(req);
        base.current = np.hash;
        setPage((old) => (old ? { ...np, body: old.body } : np));
        setState(pending.current.body || Object.keys(pending.current.set).length ? "dirty" : "saved");
        setSavedAt(Date.now());
        setPreviewTick((t) => t + 1);
        refreshChanges();
        if ("title" in req.set || "draft" in req.set || "weight" in req.set || "date" in req.set) api.getTree().then(setTree).catch(() => {});
      } catch (e) {
        // put the edits back so nothing is lost
        pending.current.body ||= req.body !== undefined;
        pending.current.set = { ...req.set, ...pending.current.set };
        req.delete.forEach((k) => pending.current.del.add(k));
        if ((e as AppError).code === "changed-on-disk") setConflict(true);
        else { setState("error"); error(e, t.pageEditor.couldntSave); }
      }
    })();
    saving.current = run;
    await run;
    saving.current = null;
  }, [path]);

  const schedule = useCallback(() => {
    setState("dirty");
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => save(), 900);
  }, [save]);

  // Make sure typing is saved before syncing/publishing/switching pages.
  useEffect(() => registerFlush(() => save()), [save]);
  useEffect(() => () => { void save(); }, [save]);
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "s") { e.preventDefault(); save(); }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [save]);

  // A teammate's update arrived for this page.
  useEffect(() => {
    const f = (e: Event) => {
      const paths: string[] = (e as CustomEvent).detail ?? [];
      if (!paths.includes(path)) return;
      if (state === "saved") load();
      else setConflict(true);
    };
    window.addEventListener("pageroom:synced", f);
    return () => window.removeEventListener("pageroom:synced", f);
  }, [path, state, load]);

  const setField = (k: string, v: any) => {
    setFields((f) => ({ ...f, [k]: v }));
    if (v === undefined || v === null || v === "" || (Array.isArray(v) && v.length === 0)) {
      delete pending.current.set[k];
      if (page?.keys?.includes(k) || k in (page?.fields ?? {})) pending.current.del.add(k);
    } else {
      pending.current.del.delete(k);
      pending.current.set[k] = v;
    }
    schedule();
  };

  const tree = site?.tree;
  const resolveImage = useCallback((src: string) => {
    if (!src) return null;
    if (/^(https?:|data:)/.test(src)) return src;
    if (src.startsWith("/") && tree) {
      // an image inside another page's folder: /posts/trip/beach.jpg
      const owner = flatPages(tree).find((p) => p.bundle && src.startsWith(p.url) && p.url !== "/");
      if (owner) return siteFileURL(owner.path.slice(0, owner.path.lastIndexOf("/")) + "/" + src.slice(owner.url.length));
    }
    const rel = resolveImageRef(src, path);
    return rel ? siteFileURL(rel) : null;
  }, [tree, path]);

  const refFor = useCallback((m: MediaItem) => refForItem(m, path, tree), [path, tree]);
  const pickImage = () => new Promise<{ ref: string; alt?: string } | null>((resolve) => setPicker(() => resolve));

  useEffect(() => on("files:dropped", async (paths: string[]) => {
    const imgs = paths.filter((p) => /\.(jpe?g|png|gif|webp|svg)$/i.test(p));
    if (!imgs.length) return;
    try {
      const out = await api.importMedia(imgs, path);
      out.imported?.forEach((r) => editorRef.current?.insertImage(r.page === path || !r.page ? r.ref : refFor(r)));
      out.failed?.forEach((f) => error(translate({ code: f.code ?? "invalid", message: f.message }), t.pageEditor.couldntAdd(f.name)));
      if (out.imported?.some((r) => r.stripped)) useStore.getState().toast({ kind: "info", title: t.pageEditor.imageAdded, body: t.pageEditor.privacyStripped });
    } catch (e) { error(e); }
  }), [path, refFor]);

  const pasteImage = async (file: File) => {
    const buf = await file.arrayBuffer();
    let bin = "";
    new Uint8Array(buf).forEach((b) => (bin += String.fromCharCode(b)));
    try {
      const r = await api.importMediaData(file.name || "pasted-image.png", btoa(bin), path);
      return { ref: r.ref };
    } catch (e) { error(e); return null; }
  };

  const isDraft = !!fields.draft;
  const url = page?.url ?? "/";

  if (loadErr) return <div className="page-pad"><Banner kind="danger" title={t.pageEditor.couldntOpen}>{loadErr.message}</Banner></div>;
  if (!page) return <div className="empty"><Spinner /></div>;

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
      <div className="editor-toolbar">
        <Tip label={pageListHidden ? t.pageEditor.showPageList : t.pageEditor.hidePageList}><button className="btn ghost icon sm" onClick={() => set({ pageListHidden: !pageListHidden })} aria-label={t.pageEditor.togglePageList}><FolderTree size={16} /></button></Tip>
        <SaveIndicator state={state} savedAt={savedAt} onRetry={() => save()} />
        <div className="spacer" />
        <Tip label={isDraft ? t.pageEditor.draftTip : t.pageEditor.visibleTip}>
          <button className={"btn sm" + (isDraft ? "" : " ghost")} onClick={() => setField("draft", isDraft ? undefined : true)}>
            {isDraft ? <><EyeOff size={14} /> {t.pageEditor.hiddenDraft}</> : <><Eye size={14} color="var(--success)" /> {t.pageEditor.visibleOnSite}</>}
          </button>
        </Tip>
        <div className="seg">
          <button className={!preview ? "on" : ""} onClick={() => setPreview(false)}>{t.pageEditor.write}</button>
          <button className={preview ? "on" : ""} onClick={async () => { await save(); setPreview(true); }}><Monitor size={13} /> {t.pageEditor.preview}</button>
        </div>
        <Tip label={t.pageEditor.pageHistory}><button className="btn ghost icon sm" onClick={async () => { await save(); setHistory(true); }} aria-label={t.pageEditor.history}><History size={16} /></button></Tip>
        <Tip label={side ? t.pageEditor.hideSettings : t.pageEditor.showSettings}><button className={"btn ghost icon sm"} onClick={() => setSide(!side)} aria-label={t.pageEditor.toggleSettings}><PanelRight size={16} /></button></Tip>
        <Menu
          trigger={<button className="btn ghost icon sm" aria-label={t.common.more}><MoreHorizontal size={16} /></button>}
          items={[
            { label: t.pageEditor.reloadFromDisk, icon: <RotateCw size={14} />, run: async () => { if (state === "saved" || (await ask.confirm(t.pageEditor.reloadTitle, t.pageEditor.reloadBody, t.pageEditor.reload, true))) load(); } },
            ...(site?.info.baseURL && !/example\.(org|com)/.test(site.info.baseURL) ? [{ label: t.pageEditor.viewLive, icon: <ExternalLink size={14} />, run: () => api.openExternal(site.info.baseURL.replace(/\/$/, "") + url) }] : []),
            { separator: true },
            {
              label: t.pageEditor.deletePage, icon: <Trash2 size={14} />, danger: true,
              run: async () => {
                if (!(await ask.confirm(title ? t.pageEditor.deleteTitle(title) : t.pageEditor.deleteUntitled, t.pageEditor.deleteBody, t.common.delete, true))) return;
                try { window.clearTimeout(timer.current); pending.current = { set: {}, del: new Set(), body: false }; await api.deletePage(path); onDeleted(); } catch (e) { error(e); }
              },
            },
          ]}
        />
      </div>

      <div className={"editor-layout" + (preview ? " preview" : side ? "" : " no-side")} style={{ flex: 1 }}>
        <div className="editor-scroll">
          {page.parseError ? (
            <RawFixer page={page} onFixed={load} />
          ) : (
            <div className="editor-paper">
              <textarea
                className="title-input"
                rows={1}
                placeholder={page.isSection ? t.pageEditor.sectionTitle : t.pageEditor.pageTitle}
                value={title}
                onChange={(e) => { setTitle(e.target.value); setField("title", e.target.value); autoGrow(e.target); }}
                onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); editorRef.current?.focus(); } }}
                ref={(el) => { if (el) autoGrow(el); }}
              />
              <div className="page-url">
                {page.isSection ? t.pageEditor.sectionIntro : ""}
                <span className="mono">{url}</span>
                {isDraft && <span className="pill draft">{t.common.draft}</span>}
              </div>
              <CoverPicture
                image={typeof fields.image === "string" ? fields.image : ""}
                alt={typeof fields.imageAlt === "string" ? fields.imageAlt : ""}
                resolve={resolveImage}
                pick={pickImage}
                describe={() => ask.prompt(t.editor.heroImageDescribeTitle, t.editor.heroImageDescribeLabel)}
                onChange={(image, alt) => setCoverFields(setField, image, alt)}
              />
              <BlockEditor
                key={version}
                ref={editorRef}
                markdown={page.body}
                resolveImage={resolveImage}
                pickImage={pickImage}
                prompt={(title, label, placeholder) => ask.prompt(title, label, placeholder)}
                addContactForm={async () => { try { await api.addContactForm(); refreshChanges(); return true; } catch (e) { error(e); return false; } }}
                addPageBlock={async (kind) => { try { await api.addPageBlock(kind); refreshChanges(); return true; } catch (e) { error(e); return false; } }}
                pasteImage={pasteImage}
                onChange={() => { pending.current.body = true; schedule(); }}
              />
            </div>
          )}
        </div>
        {preview ? (
          <PreviewPane pageURL={url} tick={previewTick} />
        ) : side ? (
          <PageSettings page={page} fields={fields} setField={setField} resolveImage={resolveImage} pickImage={pickImage} onRenamed={onPathChange} save={save} />
        ) : null}
      </div>

      {picker && (
        <MediaPicker
          page={path}
          refFor={refFor}
          onClose={() => { picker(null); setPicker(null); }}
          onPick={(r) => { picker(r); setPicker(null); }}
        />
      )}
      {history && (
        <HistoryDialog path={path} onClose={() => setHistory(false)} onRestored={() => { setHistory(false); load(); refreshChanges(); }} />
      )}
      {conflict && (
        <Dialog
          open
          dismissable={false}
          onClose={() => {}}
          title={t.pageEditor.conflictTitle}
          description={t.pageEditor.conflictDescription}
          actions={
            <>
              <button className="btn" onClick={() => { setConflict(false); load(); }}>{t.pageEditor.useUpdated}</button>
              <button className="btn primary" onClick={async () => { setConflict(false); pending.current.body = true; await save(true); }}>{t.pageEditor.keepMine}</button>
            </>
          }
        >
          <Banner kind="info"><span className="small">{t.pageEditor.conflictNote}</span></Banner>
        </Dialog>
      )}
    </div>
  );
}

export function setCoverFields(setField: (key: string, value: any) => void, image: string | undefined, alt: string | undefined) {
  setField("image", image);
  setField("imageAlt", alt);
}

export function CoverPicture({ image, alt, resolve, pick, describe, onChange }: {
  image: string;
  alt: string;
  resolve: (src: string) => string | null;
  pick: () => Promise<{ ref: string; alt?: string } | null>;
  describe: () => Promise<string | null | undefined>;
  onChange: (image: string | undefined, alt: string | undefined) => void;
}) {
  const choose = async () => {
    const picked = await pick();
    if (!picked) return;
    const description = picked.alt || (await describe()) || "";
    onChange(picked.ref, description || undefined);
  };
  const url = image ? resolve(image) : null;
  return (
    <div className="cover-picture">
      {image ? (
        <>
          {url ? <img src={url} alt={alt} /> : <div className="cover-missing small">{image}</div>}
          <div className="cover-actions">
            <button className="btn sm" onClick={choose}>{t.editor.coverChange}</button>
            <button className="btn sm ghost danger" onClick={() => onChange(undefined, undefined)}>{t.editor.coverRemove}</button>
          </div>
        </>
      ) : (
        <button className="btn ghost sm" onClick={choose}><ImagePlus size={14} /> {t.editor.coverAdd}</button>
      )}
      <span className="help">{t.editor.coverNote}</span>
    </div>
  );
}

function SaveIndicator({ state, savedAt, onRetry }: { state: SaveState; savedAt: number | null; onRetry: () => void }) {
  const [, tick] = useState(0);
  useEffect(() => { const t = setInterval(() => tick((n) => n + 1), 30000); return () => clearInterval(t); }, []);
  if (state === "saving") return <span className="save-state"><Loader2 size={13} className="spin" /> {t.pageEditor.saving}</span>;
  if (state === "dirty") return <span className="save-state"><span className="dot orange" /> {t.pageEditor.editing}</span>;
  if (state === "error") return <span className="save-state" style={{ color: "var(--danger)" }}><CloudOff size={13} /> {t.pageEditor.notSaved} <button className="btn sm" onClick={onRetry}>{t.common.tryAgain}</button></span>;
  return <span className="save-state"><CheckCircle2 size={13} color="var(--success)" /> {savedAt ? t.pageEditor.savedAgo(timeAgo(savedAt)) : t.pageEditor.allSaved}</span>;
}

function RawFixer({ page, onFixed }: { page: Page; onFixed: () => void }) {
  return (
    <div className="editor-paper">
      <Banner kind="warn" title={t.pageEditor.formatProblem}>
        <div className="small">{t.pageEditor.formatProblemBody}</div>
        <div className="mono small" style={{ marginTop: 6 }}>{page.parseError}</div>
      </Banner>
      <p className="muted small">{t.pageEditor.askDeveloper}</p>
      <pre className="card card-pad mono selectable" style={{ whiteSpace: "pre-wrap", maxHeight: 400, overflow: "auto" }}>{page.body}</pre>
      <button className="btn" onClick={onFixed}><RotateCw size={14} /> {t.pageEditor.checkAgain}</button>
    </div>
  );
}

function autoGrow(el: HTMLTextAreaElement) {
  el.style.height = "auto";
  el.style.height = el.scrollHeight + "px";
}

/** How to reference a library image from the page being edited. */
export function refForItem(m: MediaItem, pagePath: string, tree?: TreeNode): string {
  if (!m.page) return m.ref; // static/ images: "/images/x.jpg" works everywhere
  if (m.page === pagePath) return m.name;
  const owner = tree && flatPages(tree).find((p) => p.path === m.page);
  if (owner) return owner.url.replace(/\/?$/, "/") + m.name;
  return m.ref;
}
