import { useCallback, useEffect, useRef, useState } from "react";
import {
  ChevronRight, CloudOff, ExternalLink, FileText, FolderOpen, Home, Image, LayoutDashboard, ListTree, LogOut, Monitor, Palette, PanelLeft, RefreshCw, Rocket, Settings, CheckCircle2, CircleDot,
} from "lucide-react";
import { api, on, type Conflict } from "../api";
import { countPages, useStore, type View } from "../store";
import { t } from "../i18n";
import { Menu, Spinner, Tip, timeAgo } from "../ui";
import { PublishDialog, ConflictResolver } from "../dialogs/Publish";
import { ImportReport } from "../dialogs/WordPress";
import { Dashboard } from "./Dashboard";
import { PagesView } from "./Pages";
import { MediaView } from "./Media";
import { SitePreview } from "./Preview";
import { MenusView } from "./Menus";
import { AppearanceView } from "./Appearance";
import { SettingsView } from "./Settings";

const SYNC_EVERY_MS = 5 * 60 * 1000;

export function Workspace() {
  const { site, view, go, sidebarHidden, changes, publishOpen, set, refresh, refreshChanges, flushAll, toast, error, setTree } = useStore();
  const [syncConflicts, setSyncConflicts] = useState<Conflict[] | null>(null);
  const [sitePreview, setSitePreview] = useState(false);
  const syncing = useStore((s) => s.syncing);
  const lastSync = useStore((s) => s.lastSync);
  const offline = useStore((s) => s.offline);
  const importReport = useStore((s) => s.importReport);
  const syncLock = useRef(false);

  const sync = useCallback(async (quiet: boolean) => {
    const st = useStore.getState().site;
    if (!st?.git.remote || syncLock.current) return;
    syncLock.current = true;
    set({ syncing: true });
    try {
      await flushAll();
      const out = await api.sync();
      set({ lastSync: Date.now(), offline: false });
      if (out.state === "conflicts") setSyncConflicts(out.conflicts ?? []);
      else if (out.state === "updated" && out.incoming?.length) {
        toast({ kind: "info", title: t.workspace.gotLatest, body: t.workspace.changesBroughtIn(out.incoming.length) });
        await refresh();
        window.dispatchEvent(new CustomEvent("pageroom:synced", { detail: out.incoming }));
      } else if (!quiet) toast({ kind: "success", title: t.workspace.upToDate });
    } catch (e: any) {
      if (e?.code === "offline") set({ offline: true });
      else if (!quiet) error(e, t.workspace.couldntGetLatest);
    } finally {
      syncLock.current = false;
      set({ syncing: false });
    }
  }, []);

  useEffect(() => {
    refreshChanges();
    sync(true);
    const t = setInterval(() => sync(true), SYNC_EVERY_MS);
    const onFocus = () => { refreshChanges(); };
    window.addEventListener("focus", onFocus);
    const offFiles = on("site:files-changed", async () => {
      try { setTree(await api.getTree()); } catch { /* ignore */ }
      refreshChanges();
    });
    const offClose = on("app:closing", async () => { await flushAll(); api.confirmClose(); });
    return () => { clearInterval(t); window.removeEventListener("focus", onFocus); offFiles(); offClose(); };
  }, []);

  if (!site) return null;
  const isMac = /mac/i.test(navigator.platform);
  const liveURL = site.info.baseURL && !/example\.(org|com)/.test(site.info.baseURL) ? site.info.baseURL : "";
  const counts = countPages(site.tree);
  const nav: { v: View; label: string; icon: React.ReactNode; count?: number }[] = [
    { v: { kind: "dashboard" }, label: t.workspace.dashboard, icon: <LayoutDashboard size={17} /> },
    { v: { kind: "pages" }, label: t.workspace.pages, icon: <FileText size={17} />, count: counts.pages },
    { v: { kind: "media" }, label: t.workspace.images, icon: <Image size={17} /> },
    { v: { kind: "menus" }, label: t.workspace.menus, icon: <ListTree size={17} /> },
  ];
  const look: { v: View; label: string; icon: React.ReactNode }[] = [
    { v: { kind: "appearance", tab: "style" }, label: t.workspace.appearance, icon: <Palette size={17} /> },
    { v: { kind: "settings", tab: "general" }, label: t.workspace.settings, icon: <Settings size={17} /> },
  ];


  return (
    <div className={"shell" + (sidebarHidden ? " no-sidebar" : "")}>
      {!sidebarHidden && <aside className={"sidebar" + (isMac ? " mac" : "")}>
        <Menu
          align="start"
          trigger={
            <button className="site-switch">
              <div className="logo">{(site.info.title || "S").slice(0, 1).toUpperCase()}</div>
              <div style={{ minWidth: 0, flex: 1 }}>
                <div className="name">{site.info.title || t.workspace.untitledSite}</div>
                <div className="url">{liveURL ? liveURL.replace(/^https?:\/\//, "").replace(/\/$/, "") : t.workspace.notOnlineYet}</div>
              </div>
              <ChevronRight size={14} />
            </button>
          }
          items={[
            { label: t.workspace.showSiteFolder, icon: <FolderOpen size={14} />, run: () => api.revealSite() },
            ...(liveURL ? [{ label: t.workspace.visitLiveSite, icon: <ExternalLink size={14} />, run: () => api.openExternal(liveURL) }] : []),
            { separator: true },
            { label: t.workspace.switchSite, icon: <LogOut size={14} />, run: async () => { await flushAll(); await api.stopPreview().catch(() => {}); useStore.getState().setSite(null); } },
          ]}
        />
        <nav className="nav" aria-label={t.workspace.mainNavigation}>
          {nav.map((n) => (
            <button key={n.label} className={"nav-item" + (view.kind === n.v.kind ? " active" : "")} onClick={() => go(n.v)}>
              {n.icon} {n.label} {n.count != null && <span className="count">{n.count}</span>}
            </button>
          ))}
          <div className="nav-group">
            <div className="nav-label">{t.workspace.designAndSetup}</div>
            {look.map((n) => (
              <button key={n.label} className={"nav-item" + (view.kind === n.v.kind ? " active" : "")} onClick={() => go(n.v)}>
                {n.icon} {n.label}
              </button>
            ))}
          </div>
        </nav>
        <div className="sidebar-foot">
          {offline ? <><CloudOff size={14} /> {t.workspace.offline}</> : site.git.remote ? (
            <>{syncing ? <Spinner size={13} /> : <CheckCircle2 size={13} />} {syncing ? t.workspace.checkingForUpdates : lastSync ? t.workspace.upToDateAgo(timeAgo(lastSync)) : t.workspace.connected}</>
          ) : (
            <><CircleDot size={13} /> {t.workspace.onlyOnThisComputer}</>
          )}
          {site.git.remote && (
            <Tip label={t.workspace.getLatestTip}>
              <button className="btn dark-ghost icon sm" onClick={() => sync(false)} disabled={syncing} aria-label={t.workspace.getLatest}><RefreshCw size={14} /></button>
            </Tip>
          )}
        </div>
      </aside>}

      <div className="main">
        <header className={"topbar" + (isMac ? " mac" : "")}>
          <Tip label={sidebarHidden ? t.workspace.showSidebar : t.workspace.hideSidebar}>
            <button className="btn ghost icon sm" onClick={() => set({ sidebarHidden: !sidebarHidden })} aria-label={t.workspace.toggleSidebar}><PanelLeft size={16} /></button>
          </Tip>
          <div className="crumbs">
            <Home size={15} />
            <ChevronRight size={13} />
            <strong>{titleFor(view)}</strong>
          </div>
          {changes.length > 0 && <span className="dim small nowrap"><span className="changed-dot" style={{ display: "inline-block", marginRight: 6 }} />{t.workspace.unpublishedChanges(changes.length)}</span>}
          {liveURL && <button className="btn" onClick={() => api.openExternal(liveURL)}><ExternalLink size={15} /> {t.workspace.liveSite}</button>}
          <button className="btn" onClick={async () => { await flushAll(); setSitePreview(true); }}><Monitor size={15} /> {t.workspace.previewSite}</button>
          {site.git.remote || site.git.direct ? (
            <button className="btn primary" onClick={async () => { await flushAll(); set({ publishOpen: true }); }}>
              <Rocket size={15} /> {t.workspace.publish} {changes.length > 0 && <span className="badge">{changes.length}</span>}
            </button>
          ) : (
            <button className="btn primary" onClick={() => go({ kind: "settings", tab: "publishing" })}><Rocket size={15} /> {t.workspace.goLive}</button>
          )}
        </header>
        <div className="content" style={view.kind === "pages" || view.kind === "appearance" ? { overflow: "hidden" } : undefined}>
          {view.kind === "dashboard" && <Dashboard />}
          {view.kind === "pages" && <PagesView initial={view.path} />}
          {view.kind === "media" && <MediaView />}
          {view.kind === "menus" && <MenusView />}
          {view.kind === "appearance" && <AppearanceView tab={view.tab ?? "style"} />}
          {view.kind === "settings" && <SettingsView tab={view.tab ?? "general"} />}
        </div>
      </div>

      {sitePreview && <SitePreview mac={isMac} onClose={() => setSitePreview(false)} />}
      {publishOpen && <PublishDialog onClose={() => set({ publishOpen: false })} />}
      {importReport && <ImportReport report={importReport} onClose={() => set({ importReport: null })} />}
      {syncConflicts && (
        <ConflictResolver
          conflicts={syncConflicts}
          actionLabel={t.workspace.keepMyChoices}
          onCancel={() => setSyncConflicts(null)}
          onResolve={async (choices) => {
            try {
              const out = await api.resolve(choices);
              if (out.state === "conflicts") setSyncConflicts(out.conflicts ?? []);
              else { setSyncConflicts(null); toast({ kind: "success", title: t.workspace.allSorted, body: t.workspace.allSortedBody }); await refresh(); }
            } catch (e) { error(e); }
          }}
        />
      )}
    </div>
  );
}

function titleFor(v: View): string {
  switch (v.kind) {
    case "dashboard": return t.workspace.dashboard;
    case "pages": return t.workspace.pages;
    case "media": return t.workspace.images;
    case "menus": return t.workspace.menus;
    case "appearance": return v.tab === "css" ? t.workspace.appearanceCss : v.tab === "themes" ? t.workspace.appearanceThemes : t.workspace.appearance;
    case "settings": return t.workspace.settings;
  }
}
