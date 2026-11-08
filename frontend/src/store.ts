import { create } from "zustand";
import { api, type ChangeView, type SiteState, type TreeNode, type AppError, type WPReport } from "./api";
import { language, t, type Language } from "./i18n";

export type View =
  | { kind: "dashboard" }
  | { kind: "pages"; path?: string }
  | { kind: "media" }
  | { kind: "menus" }
  | { kind: "appearance"; tab?: "style" | "css" | "themes" }
  | { kind: "settings"; tab?: "general" | "account" | "publishing" };

export interface Toast {
  id: number;
  kind: "success" | "error" | "info";
  title: string;
  body?: string;
  action?: { label: string; run: () => void };
}

type Flusher = () => Promise<void>;

interface State {
  site: SiteState | null;
  language: Language;
  view: View;
  sidebarHidden: boolean;
  pageListHidden: boolean;
  changes: ChangeView[];
  toasts: Toast[];
  syncing: boolean;
  lastSync: number | null;
  offline: boolean;
  publishOpen: boolean;
  signInOpen: boolean;
  identityOpen: boolean;
  importReport: WPReport | null;
  flushers: Set<Flusher>;

  setSite(s: SiteState | null): void;
  setTree(t: TreeNode): void;
  go(v: View): void;
  refresh(): Promise<void>;
  refreshChanges(): Promise<void>;
  toast(t: Omit<Toast, "id">): void;
  dismiss(id: number): void;
  error(e: AppError | unknown, title?: string): void;
  set(p: Partial<State>): void;
  /** Editors register a flush so pending typing is saved before sync/publish. */
  registerFlush(f: Flusher): () => void;
  flushAll(): Promise<void>;
}

let toastId = 1;

export const useStore = create<State>((set, get) => ({
  site: null,
  language,
  view: { kind: "dashboard" },
  sidebarHidden: false,
  pageListHidden: false,
  changes: [],
  toasts: [],
  syncing: false,
  lastSync: null,
  offline: false,
  publishOpen: false,
  signInOpen: false,
  identityOpen: false,
  importReport: null,
  flushers: new Set(),

  setSite: (site) => set({ site, view: { kind: "dashboard" }, changes: [] }),
  setTree: (tree) => {
    const s = get().site;
    if (s) set({ site: { ...s, tree } });
  },
  go: (view) => set({ view }),
  refresh: async () => {
    try {
      const site = await api.getSite();
      set({ site });
    } catch (e) {
      get().error(e);
    }
    await get().refreshChanges();
  },
  refreshChanges: async () => {
    try {
      const changes = (await api.getChanges()) ?? [];
      set({ changes });
    } catch {
      /* changes are informative only */
    }
  },
  toast: (t) => {
    const id = toastId++;
    set({ toasts: [...get().toasts, { ...t, id }] });
    setTimeout(() => get().dismiss(id), t.kind === "error" ? 9000 : 5000);
  },
  dismiss: (id) => set({ toasts: get().toasts.filter((t) => t.id !== id) }),
  error: (e, title) => {
    const err = e as AppError;
    get().toast({ kind: "error", title: title ?? t.common.somethingWentWrong, body: err?.message ?? String(e) });
  },
  set: (p) => set(p),
  registerFlush: (f) => {
    get().flushers.add(f);
    return () => get().flushers.delete(f);
  },
  flushAll: async () => {
    for (const f of Array.from(get().flushers)) {
      try {
        await f();
      } catch {
        /* the editor shows its own save errors */
      }
    }
  },
}));

export function findNode(n: TreeNode | undefined, path: string): TreeNode | undefined {
  if (!n) return undefined;
  if (n.path === path || n.indexFile === path) return n;
  for (const c of n.children ?? []) {
    const f = findNode(c, path);
    if (f) return f;
  }
  return undefined;
}

export function parentOf(n: TreeNode, path: string): TreeNode | undefined {
  for (const c of n.children ?? []) {
    if (c.path === path) return n;
    const f = parentOf(c, path);
    if (f) return f;
  }
  return undefined;
}

export function countPages(n: TreeNode): { pages: number; drafts: number } {
  let pages = 0, drafts = 0;
  const walk = (x: TreeNode) => {
    if (x.kind === "page") {
      pages++;
      if (x.draft) drafts++;
    }
    x.children?.forEach(walk);
  };
  walk(n);
  return { pages, drafts };
}

export function flatPages(n: TreeNode): TreeNode[] {
  const out: TreeNode[] = [];
  const walk = (x: TreeNode) => {
    if (x.kind === "page") out.push(x);
    x.children?.forEach(walk);
  };
  walk(n);
  return out;
}
