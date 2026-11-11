import type * as app from "../bindings/github.com/myrheimb/pageroom/internal/app/models";
import type * as gitsync from "../bindings/github.com/myrheimb/pageroom/internal/gitsync/models";
import type * as site from "../bindings/github.com/myrheimb/pageroom/internal/site/models";
import type { Handler } from "./types";
import { accent } from "./site";

declare global {
  interface Window {
    mockRemote?: boolean;
  }
}

const unused = () => Promise.reject(new Error("The screenshot mock does not implement this call."));
const nothing = async () => {};

const postPath = "content/posts/welcome/index.md";

const body = [
  "Our sourdough is baked fresh every morning, and we will share photos of every loaf soon.",
  "",
  "> Best bread in town, says everyone on our street.",
  "",
  "When the next loaf comes out of the oven, we will post it here.",
  "",
].join("\n");

const pages: Record<string, site.Page> = {
  [postPath]: {
    path: postPath,
    hash: "1",
    format: "yaml",
    keys: ["title", "date", "tags"],
    fields: { title: "Hello, world", date: "2026-01-01T09:00:00Z", tags: ["news"] },
    body,
    url: "/posts/welcome/",
    bundle: true,
    isSection: false,
  },
};

const tree: site.Node = {
  path: "content",
  kind: "section",
  title: "Home",
  draft: false,
  weight: 0,
  bundle: false,
  url: "/",
  indexFile: "content/_index.md",
  children: [
    {
      path: "content/posts",
      kind: "section",
      title: "Blog posts",
      draft: false,
      weight: 1,
      bundle: false,
      url: "/posts/",
      indexFile: "content/posts/_index.md",
      children: [
        { path: postPath, kind: "page", title: "Hello, world", draft: false, date: "2026-01-01T09:00:00Z", weight: 0, bundle: true, url: "/posts/welcome/" },
      ],
    },
    { path: "content/about.md", kind: "page", title: "About", draft: false, date: "2025-12-20T09:00:00Z", weight: 2, bundle: false, url: "/about/" },
  ],
};

const siteState = (): app.SiteState => ({
  info: {
    root: "/Users/hannah/Sites/bakery",
    title: "Hannah's Bakery",
    baseURL: "https://example.org/",
    theme: "starter",
    language: "en",
    languages: ["en"],
    config: "hugo.toml",
    prefs: { version: 1, hugo: { version: "" }, sections: null, fields: null, menus: null, media: { maxWidth: 1600, folder: "" }, appearance: { headHook: "" } },
    sections: { posts: { label: "Blog posts", icon: "", hidden: false } },
  },
  tree,
  settings: { title: "Hannah's Bakery", baseURL: "https://example.org/", language: "en", description: "", copyright: "", theme: "starter", formAddress: "", contactEmail: "" },
  git: window.mockRemote
    ? { isRepo: true, remote: "https://github.com/hannah/bakery.git", host: "github.com", branch: "main", signedIn: true, published: true }
    : { isRepo: true, remote: "", host: "", branch: "main", signedIn: false, published: false },
  styleHook: { kind: "head-hook", target: "layouts/partials/head.html", supported: true },
});

const conflicts: gitsync.Conflict[] = [
  {
    path: postPath,
    binary: false,
    oursDeleted: false,
    theirsDeleted: false,
    localInWay: false,
    theirAuthor: "Ben",
    hunks: [
      {
        conflict: true,
        ours: ["Our sourdough is baked fresh every morning, **before the shop opens**."],
        theirs: ["Our sourdough is baked fresh every morning, **starting at four**."],
      },
      { conflict: false, lines: ["When the next loaf comes out of the oven, we will post it here."] },
    ],
  },
];

export const GetWelcome: Handler<"GetWelcome"> = async () => ({
  recent: [{ path: "/Users/hannah/Sites/bakery", title: "Hannah's Bakery", opened: new Date().toISOString() }],
  name: "Hannah Baker",
  email: "hannah@example.org",
  sitesFolder: "/Users/hannah/Sites",
  githubSignIn: true,
  platform: "linux",
  language: "en",
  version: "0.1.0",
});
export const CheckForUpdate: Handler<"CheckForUpdate"> = async () => null;
export const OpenSite: Handler<"OpenSite"> = async () => siteState();
export const GetSite: Handler<"GetSite"> = async () => siteState();
export const GetTree: Handler<"GetTree"> = async () => tree;
export const GetChanges: Handler<"GetChanges"> = async () => [{ path: postPath, kind: "modified", type: "page", label: "Hello, world" }];
export const GetPage: Handler<"GetPage"> = async (path) => pages[path];
export const SavePage: Handler<"SavePage"> = async (request) => ({ ...pages[request.path], hash: String(Math.random()) });
export const StartPreview: Handler<"StartPreview"> = async () => ({ url: `${location.origin}/mock-site`, running: true });
export const PreviewStatus: Handler<"PreviewStatus"> = async () => ({ url: `${location.origin}/mock-site`, running: true });
export const DefaultPublishMessage: Handler<"DefaultPublishMessage"> = async () => "Update the blog";
export const PendingPublish: Handler<"PendingPublish"> = async () => "";
export const Sync: Handler<"Sync"> = async () => ({ state: "up-to-date" });
export const Publish: Handler<"Publish"> = async () => ({ state: "conflicts", conflicts });
export const GetStyle: Handler<"GetStyle"> = async () => ({
  style: {
    tokens: { primary: accent, background: "", text: "", heading: "", headingFont: "", bodyFont: "", fontSize: 0, contentWidth: 0, radius: -1 },
    customCSS: "",
  },
  hook: { kind: "head-hook", target: "layouts/partials/head.html", supported: true },
  fonts: {
    system: "system-ui, sans-serif",
    humanist: "Optima, Candara, sans-serif",
    geometric: "Avenir, Montserrat, sans-serif",
    classical: "Palatino, Georgia, serif",
    oldstyle: "Iowan Old Style, Palatino, serif",
    transitional: "Baskerville, Georgia, serif",
    didone: "Didot, Bodoni MT, serif",
    slab: "Rockwell, Roboto Slab, serif",
    rounded: "ui-rounded, Nunito, sans-serif",
    mono: "ui-monospace, Menlo, monospace",
  },
});
export const SaveStyle: Handler<"SaveStyle"> = async () => ({ kind: "head-hook", target: "layouts/partials/head.html", supported: true });
export const ConfirmClose: Handler<"ConfirmClose"> = nothing;
export const OpenExternal: Handler<"OpenExternal"> = nothing;
export const StopPreview: Handler<"StopPreview"> = nothing;

export const AddContactForm = unused;
export const AddPageBlock = unused;
export const AddTranslation = unused;
export const CancelSignIn = unused;
export const CheckTheme = unused;
export const ChooseFolder = unused;
export const ChooseSiteIcon = unused;
export const CloneSite = unused;
export const ConnectCloudflare = unused;
export const ConnectRemote = unused;
export const CreatePage = unused;
export const CreateSection = unused;
export const CreateSite = unused;
export const CreateSiteFromWordPress = unused;
export const DeleteMedia = unused;
export const DeletePage = unused;
export const Discard = unused;
export const DisconnectRemote = unused;
export const DownloadHugo = unused;
export const EndThemeTrial = unused;
export const ForgetSite = unused;
export const GalleryThemes = unused;
export const GetAuthStatus = unused;
export const GetMenus = unused;
export const GetSiteIcon = unused;
export const History = unused;
export const ImportMedia = unused;
export const ImportMediaData = unused;
export const ImportWordPress = unused;
export const ListMedia = unused;
export const ListThemes = unused;
export const MediaUsage = unused;
export const MovePage = unused;
export const PeekWordPress = unused;
export const PickImages = unused;
export const PickWordPressFile = unused;
export const RemoveHosting = unused;
export const RenameSlug = unused;
export const ReorderPages = unused;
export const Resolve = unused;
export const RestoreRevision = unused;
export const RevealSite = unused;
export const RevisionContent = unused;
export const SaveAccessToken = unused;
export const SaveIdentity = unused;
export const SaveLanguage = unused;
export const SaveMenu = unused;
export const SaveSettings = unused;
export const SetUpCloudflare = unused;
export const SetUpGitHubPages = unused;
export const SignOut = unused;
export const StartGitHubSignIn = unused;
export const StopCloudflare = unused;
export const SwitchTheme = unused;
export const TryTheme = unused;
export const UseLetterIcon = unused;
export const WaitGitHubSignIn = unused;
