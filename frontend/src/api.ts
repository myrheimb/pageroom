// Typed access to the Go backend. Every call goes through `call` so errors
// arrive as { code, message } objects written for people.
import { Events } from "@wailsio/runtime";
import * as Go from "../bindings/github.com/myrheimb/pageroom/internal/app/app";
import { t } from "./i18n";

export interface AppError {
  code: string;
  message: string;
  path?: string;
  details?: Record<string, string>;
}

export function toAppError(e: unknown): AppError {
  // The backend's AppError arrives as the cause of the rejected call.
  if (e instanceof Error) return toAppError(e.cause ?? e.message);
  if (e && typeof e === "object" && "message" in e) {
    const o = e as Partial<AppError>;
    return { code: o.code ?? "error", message: String(o.message), path: o.path, details: o.details };
  }
  if (typeof e === "string") {
    try {
      const o = JSON.parse(e);
      if (o && o.message) return o;
    } catch {
      /* plain string */
    }
    return { code: "error", message: e };
  }
  return { code: "error", message: t.common.unknownError };
}

export function translate(error: AppError): AppError {
  const message = t.errors[error.code];
  return message ? { ...error, message: message(error.details ?? {}) } : error;
}

async function call<T>(p: Promise<T>): Promise<T> {
  try {
    return await p;
  } catch (e) {
    throw translate(toAppError(e));
  }
}


export interface RecentSite { path: string; title: string; opened: string; remote?: string }
export interface Welcome { recent: RecentSite[]; name: string; email: string; sitesFolder: string; githubSignIn: boolean; platform: string; language: string; version: string }

export interface UpdateRelease { version: string; url: string }

export interface SectionPrefs { label: string; icon: string; hidden: boolean }
export interface FieldDef { key: string; label: string; type: string; options?: string[]; help?: string; sections?: string[] }
export interface Prefs {
  version: number;
  hugo: { version: string };
  sections: Record<string, SectionPrefs> | null;
  fields: FieldDef[] | null;
  menus: string[] | null;
  media: { maxWidth: number; folder: string };
  appearance: { headHook: string };
}
export interface SiteInfo { root: string; title: string; baseURL: string; theme: string; language: string; config: string; prefs: Prefs; sections: Record<string, SectionPrefs> | null }
export interface TreeNode {
  path: string; kind: "section" | "page"; title: string; draft: boolean; date?: string; weight: number;
  bundle: boolean; url: string; indexFile?: string; children?: TreeNode[]; error?: string;
}
export interface Settings { title: string; baseURL: string; language: string; description: string; copyright: string; theme: string; formAddress: string; contactEmail: string }
export interface SiteIcon { image: string; letter: boolean }
export interface GitHubPages { address: string; settings: string; setUp: boolean }
export interface Cloudflare { name: string; setUp: boolean }
export interface DirectUpload { account: string; worker: string; address: string; published: boolean }
export interface CloudflareAccount { id: string; name: string }
export interface GitState { isRepo: boolean; remote: string; host: string; branch: string; signedIn: boolean; published: boolean; pages?: GitHubPages; cloudflare?: Cloudflare; direct?: DirectUpload }
export interface StyleHook { kind: string; target: string; supported: boolean; suggestion?: string }
export interface SiteState { info: SiteInfo; tree: TreeNode; settings: Settings; git: GitState; styleHook: StyleHook }

export interface Page {
  path: string; hash: string; format: string; keys: string[] | null; fields: Record<string, any>; body: string;
  url: string; bundle: boolean; isSection: boolean; parseError?: string;
}
export interface SaveRequest { path: string; baseHash: string; set?: Record<string, any>; delete?: string[]; body?: string; force?: boolean }
export interface NewPage { section: string; title: string; bundle: boolean }

export interface MenuItem { id: string; name: string; pageRef?: string; url?: string; weight: number; parent?: string; source: string }
export interface Menu { name: string; items: MenuItem[] }

export interface MediaItem { path: string; name: string; ref: string; page?: string; width: number; height: number; size: number; modified: string; format: string }
export interface ImportResult extends MediaItem { stripped: boolean; resized: boolean; note?: string }
export interface ImportOutcome { imported: ImportResult[] | null; failed: { name: string; code?: string; message: string }[] | null }

export interface ChangeView { path: string; kind: "added" | "modified" | "deleted"; type: "page" | "image" | "settings" | "appearance" | "theme" | "file"; label: string }
export interface Hunk { conflict: boolean; lines?: string[]; base?: string[]; ours?: string[]; theirs?: string[] }
export interface Conflict { path: string; binary: boolean; oursDeleted: boolean; theirsDeleted: boolean; localInWay: boolean; hunks?: Hunk[]; theirAuthor?: string }
export interface Finding { path: string; line: number; kind: string }
export interface Outcome {
  state: "up-to-date" | "updated" | "published" | "conflicts" | "blocked" | "offline";
  incoming?: string[]; conflicts?: Conflict[]; secrets?: Finding[]; commit?: string; message?: string;
}
export type Resolution = "ours" | "theirs" | "both";
export interface Choice { whole?: Resolution; hunks?: Resolution[] }
export interface Revision { commit: string; author: string; when: string; message: string }

export interface SiteFont { family: string; fallback: string; files: { path: string; weight: string; style: string; unicodeRange?: string }[] }
export interface Tokens { primary: string; background: string; text: string; heading: string; headingFont: string; bodyFont: string; fontSize: number; contentWidth: number; radius: number; fonts?: SiteFont[] }
export interface Style { tokens: Tokens; customCSS: string }
export interface StyleState { style: Style; hook: StyleHook; fonts: Record<string, string> }
export interface ThemeInfo { name: string; active: boolean; screenshot?: string; description?: string }
export interface GalleryTheme { name: string; title: string; description?: string; author?: string; tags?: string[]; license: string; demo?: string; updated: string; thumbnail: string; screenshot?: string }
export interface PreviewState { url: string; running: boolean; errors?: string[]; note?: string }
export interface AuthStatus { host: string; signedIn: boolean; kind?: string; username?: string; expires?: string }
export interface WPSkippedKind { type: string; count: number }
export interface WPSummary { title: string; link: string; posts: number; pages: number; drafts: number; images: number; comments: number; menuItems: number; skipped: WPSkippedKind[] }
export interface WPReport { posts: number; pages: number; drafts: number; images: number; files: number; menuItems: number; comments: number; failedImages?: string[]; failedFiles?: string[]; warnings?: string[] }
export interface DeviceCode { userCode: string; verificationUri: string; expiresIn: number; interval: number }


const g = Go as any;

export const api = {
  getWelcome: () => call<Welcome>(g.GetWelcome()),
  checkForUpdate: () => call<UpdateRelease | null>(g.CheckForUpdate()),
  saveIdentity: (name: string, email: string) => call<void>(g.SaveIdentity(name, email)),
  saveLanguage: (language: string) => call<void>(g.SaveLanguage(language)),
  chooseFolder: (title: string) => call<string>(g.ChooseFolder(title)),
  forgetSite: (path: string) => call<void>(g.ForgetSite(path)),
  openSite: (path: string) => call<SiteState>(g.OpenSite(path)),
  cloneSite: (url: string, folder: string) => call<SiteState>(g.CloneSite(url, folder)),
  createSite: (title: string) => call<SiteState>(g.CreateSite(title)),
  getSite: () => call<SiteState>(g.GetSite()),
  getTree: () => call<TreeNode>(g.GetTree()),
  revealSite: () => call<void>(g.RevealSite()),
  openExternal: (url: string) => call<void>(g.OpenExternal(url)),

  getPage: (path: string) => call<Page>(g.GetPage(path)),
  savePage: (req: SaveRequest) => call<Page>(g.SavePage(req)),
  createPage: (np: NewPage) => call<string>(g.CreatePage(np)),
  createSection: (parent: string, title: string) => call<string>(g.CreateSection(parent, title)),
  movePage: (path: string, to: string) => call<string>(g.MovePage(path, to)),
  renameSlug: (path: string, slug: string) => call<string>(g.RenameSlug(path, slug)),
  deletePage: (path: string) => call<void>(g.DeletePage(path)),
  reorderPages: (paths: string[]) => call<void>(g.ReorderPages(paths)),

  getMenus: () => call<Menu[]>(g.GetMenus()),
  saveMenu: (m: Menu) => call<void>(g.SaveMenu(m)),
  saveSettings: (s: Settings) => call<void>(g.SaveSettings(s)),
  addContactForm: () => call<void>(g.AddContactForm()),
  getSiteIcon: () => call<SiteIcon>(g.GetSiteIcon()),
  chooseSiteIcon: () => call<SiteIcon | null>(g.ChooseSiteIcon()),
  useLetterIcon: () => call<SiteIcon>(g.UseLetterIcon()),

  listThemes: () => call<ThemeInfo[]>(g.ListThemes()),
  checkTheme: (name: string) => call<string>(g.CheckTheme(name)),
  switchTheme: (name: string) => call<void>(g.SwitchTheme(name)),
  galleryThemes: () => call<GalleryTheme[]>(g.GalleryThemes()),
  tryTheme: (name: string) => call<string>(g.TryTheme(name)),
  endThemeTrial: (keep: boolean) => call<void>(g.EndThemeTrial(keep)),
  getStyle: () => call<StyleState>(g.GetStyle()),
  saveStyle: (s: Style) => call<StyleHook>(g.SaveStyle(s)),

  listMedia: () => call<MediaItem[]>(g.ListMedia()),
  pickImages: () => call<string[] | null>(g.PickImages()),
  importMedia: (paths: string[], page: string) => call<ImportOutcome>(g.ImportMedia(paths, page)),
  importMediaData: (name: string, b64: string, page: string) => call<ImportResult>(g.ImportMediaData(name, b64, page)),
  mediaUsage: (path: string) => call<string[] | null>(g.MediaUsage(path)),
  deleteMedia: (path: string, force: boolean) => call<void>(g.DeleteMedia(path, force)),

  getChanges: () => call<ChangeView[] | null>(g.GetChanges()),
  defaultPublishMessage: () => call<string>(g.DefaultPublishMessage()),
  pendingPublish: () => call<string>(g.PendingPublish()),
  sync: () => call<Outcome>(g.Sync()),
  publish: (message: string) => call<Outcome>(g.Publish(message)),
  resolve: (choices: Record<string, Choice>) => call<Outcome>(g.Resolve(choices)),
  discard: (path: string) => call<void>(g.Discard(path)),
  history: (path: string) => call<Revision[] | null>(g.History(path)),
  revisionContent: (path: string, commit: string) => call<string>(g.RevisionContent(path, commit)),
  restoreRevision: (path: string, commit: string) => call<Page>(g.RestoreRevision(path, commit)),
  connectRemote: (url: string) => call<GitState>(g.ConnectRemote(url)),
  disconnectRemote: () => call<void>(g.DisconnectRemote()),
  setUpGitHubPages: () => call<GitHubPages>(g.SetUpGitHubPages()),
  setUpCloudflare: () => call<Cloudflare>(g.SetUpCloudflare()),
  removeHosting: (host: "github-pages" | "cloudflare") => call<void>(g.RemoveHosting(host)),
  connectCloudflare: (token: string, accountID: string) => call<CloudflareAccount[] | null>(g.ConnectCloudflare(token, accountID)),
  stopCloudflare: () => call<void>(g.StopCloudflare()),

  getAuthStatus: (remote: string) => call<AuthStatus>(g.GetAuthStatus(remote)),
  startGitHubSignIn: () => call<DeviceCode>(g.StartGitHubSignIn()),
  waitGitHubSignIn: () => call<AuthStatus>(g.WaitGitHubSignIn()),
  cancelSignIn: () => call<void>(g.CancelSignIn()),
  saveAccessToken: (remote: string, user: string, token: string) => call<AuthStatus>(g.SaveAccessToken(remote, user, token)),
  signOut: (remote: string) => call<void>(g.SignOut(remote)),

  pickWordPressFile: () => call<string>(g.PickWordPressFile()),
  peekWordPress: (path: string) => call<WPSummary>(g.PeekWordPress(path)),
  importWordPress: (path: string, images: boolean) => call<WPReport>(g.ImportWordPress(path, images)),
  createSiteFromWordPress: (path: string, images: boolean) => call<{ site: SiteState; report: WPReport }>(g.CreateSiteFromWordPress(path, images)),

  confirmClose: () => call<void>(g.ConfirmClose()),

  downloadHugo: () => call<void>(g.DownloadHugo()),
  startPreview: () => call<PreviewState>(g.StartPreview()),
  previewStatus: () => call<PreviewState>(g.PreviewStatus()),
  stopPreview: () => call<void>(g.StopPreview()),
};

export function on(event: string, cb: (...data: any[]) => void): () => void {
  try {
    return Events.On(event, (e) => cb(e.data));
  } catch {
    return () => {};
  }
}

export function siteFileURL(rel: string, bust?: string | number): string {
  const enc = rel.split("/").map(encodeURIComponent).join("/");
  return `/site-file/${enc}${bust ? `?v=${bust}` : ""}`;
}

/**
 * Resolve an image reference as written in Markdown (relative to the page
 * bundle, or /images/x.jpg from static/) to a site-relative path.
 */
export function resolveImageRef(ref: string, pagePath: string): string | null {
  if (!ref || /^(https?:|data:|\/\/)/i.test(ref)) return null;
  const clean = decodeURIComponent(ref.split(/[?#]/)[0]);
  if (clean.startsWith("/")) return "static" + clean;
  const dir = pagePath.slice(0, pagePath.lastIndexOf("/"));
  const parts = (dir + "/" + clean).split("/");
  const out: string[] = [];
  for (const p of parts) {
    if (p === "..") out.pop();
    else if (p && p !== ".") out.push(p);
  }
  return out.join("/");
}
