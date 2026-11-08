import { useEffect, useState } from "react";
import { ArrowRight, Download, FilePlus2, FileText, ImagePlus, Newspaper, Palette, PenLine, Globe } from "lucide-react";
import { api, type TreeNode, type UpdateRelease } from "../api";
import { countPages, flatPages, useStore } from "../store";
import { ask, Banner, formatDate } from "../ui";
import { t } from "../i18n";

export function UpdateBanner() {
  const [release, setRelease] = useState<UpdateRelease | null>(null);
  useEffect(() => { api.checkForUpdate().then(setRelease).catch(() => {}); }, []);
  if (!release) return null;
  return (
    <Banner kind="info" title={t.dashboard.updateAvailable(release.version)}>
      <div className="small">{t.dashboard.updateHint}</div>
      <button className="btn sm" style={{ marginTop: 8 }} onClick={() => api.openExternal(release.url).catch(() => {})}><Download size={13} /> {t.dashboard.downloadUpdate}</button>
    </Banner>
  );
}

export function Dashboard() {
  const { site, go, changes, error } = useStore();
  const [name, setName] = useState("");
  useEffect(() => { api.getWelcome().then((w) => setName(w.name.split(" ")[0] ?? "")).catch(() => {}); }, []);
  if (!site) return null;
  const counts = countPages(site.tree);
  const recent = flatPages(site.tree)
    .filter((p) => p.date)
    .sort((a, b) => (b.date ?? "").localeCompare(a.date ?? ""))
    .slice(0, 6);
  const blog = findBlogSection(site.tree);

  const newPost = async () => {
    const title = await ask.prompt(t.dashboard.newPost, t.dashboard.newPostLabel, t.dashboard.newPostPlaceholder);
    if (!title) return;
    try {
      const path = await api.createPage({ section: blog?.path ?? site.tree.path, title, bundle: true });
      await useStore.getState().refresh();
      go({ kind: "pages", path });
    } catch (e) { error(e); }
  };

  const hour = new Date().getHours();
  const greet = hour < 12 ? t.dashboard.goodMorning : hour < 18 ? t.dashboard.goodAfternoon : t.dashboard.goodEvening;

  return (
    <div className="page-pad">
      <h1 className="page-title">{t.dashboard.greeting(greet, name)}</h1>
      <p className="page-sub">{t.dashboard.whatsHappening(site.info.title || t.dashboard.yourSite)}</p>

      <UpdateBanner />

      {!site.git.remote && !site.git.direct && (
        <Banner kind="info" title={t.dashboard.onlyOnComputer}>
          <div className="small">{t.dashboard.goOnline}</div>
          <button className="btn sm" style={{ marginTop: 8 }} onClick={() => go({ kind: "settings", tab: "publishing" })}><Globe size={13} /> {t.dashboard.setUpPublishing}</button>
        </Banner>
      )}

      <div className="grid cols-3" style={{ marginBottom: 20 }}>
        <button className="start-card" onClick={newPost}>
          <div className="ic"><PenLine size={19} /></div>
          <h3>{t.dashboard.writePost}</h3>
          <p>{t.dashboard.writePostHint}</p>
        </button>
        <button className="start-card" onClick={async () => {
          const title = await ask.prompt(t.dashboard.newPage, t.dashboard.newPageLabel, t.dashboard.newPagePlaceholder);
          if (!title) return;
          try {
            const path = await api.createPage({ section: site.tree.path, title, bundle: true });
            await useStore.getState().refresh();
            go({ kind: "pages", path });
          } catch (e) { error(e); }
        }}>
          <div className="ic"><FilePlus2 size={19} /></div>
          <h3>{t.dashboard.addPage}</h3>
          <p>{t.dashboard.addPageHint}</p>
        </button>
        <button className="start-card" onClick={() => go({ kind: "media" })}>
          <div className="ic"><ImagePlus size={19} /></div>
          <h3>{t.dashboard.uploadImages}</h3>
          <p>{t.dashboard.uploadImagesHint}</p>
        </button>
      </div>

      <div className="grid cols-2">
        <div className="card">
          <div className="card-pad row">
            <div style={{ flex: 1 }}>
              <h3>{t.dashboard.recentWriting}</h3>
              <div className="dim small">{t.dashboard.recentWritingHint}</div>
            </div>
            <button className="btn sm ghost" onClick={() => go({ kind: "pages" })}>{t.dashboard.allPages} <ArrowRight size={13} /></button>
          </div>
          <div className="list">
            {recent.length === 0 && <div className="list-item dim">{t.dashboard.nothingWritten}</div>}
            {recent.map((p) => (
              <div key={p.path} className="list-item clickable" onClick={() => go({ kind: "pages", path: p.path })}>
                <FileText size={15} className="dim" />
                <div style={{ flex: 1, minWidth: 0 }} className="truncate">{p.title}</div>
                {p.draft ? <span className="pill draft">{t.common.draft}</span> : <span className="pill live">{t.dashboard.live}</span>}
                <span className="dim small nowrap">{formatDate(p.date)}</span>
              </div>
            ))}
          </div>
        </div>

        <div className="stack" style={{ gap: 16 }}>
          <div className="card card-pad">
            <div className="row" style={{ gap: 28 }}>
              <div><div className="stat">{counts.pages}</div><div className="stat-label">{t.dashboard.statPages}</div></div>
              <div><div className="stat">{counts.drafts}</div><div className="stat-label">{t.dashboard.statDrafts}</div></div>
              <div><div className="stat">{changes.length}</div><div className="stat-label">{t.dashboard.statUnpublished}</div></div>
            </div>
          </div>
          <div className="card card-pad row">
            <div className="ic" style={{ width: 36, height: 36, borderRadius: 9, background: "var(--accent-soft)", color: "var(--accent)", display: "grid", placeItems: "center" }}><Palette size={18} /></div>
            <div style={{ flex: 1 }}>
              <h3>{t.dashboard.makeItYours}</h3>
              <div className="dim small">{t.dashboard.makeItYoursHint}</div>
            </div>
            <button className="btn sm" onClick={() => go({ kind: "appearance", tab: "style" })}>{t.dashboard.customize}</button>
          </div>
        </div>
      </div>
      {blog && <p className="dim small" style={{ marginTop: 24 }}><Newspaper size={12} /> {t.dashboard.newPostsGoInto(blog.title)}</p>}
    </div>
  );
}

function findBlogSection(root: TreeNode): TreeNode | undefined {
  const sections = (root.children ?? []).filter((c) => c.kind === "section");
  return sections.find((s) => /^(posts?|blog|news|articles)$/i.test(s.path.split("/").pop() ?? "")) ?? sections[0];
}
