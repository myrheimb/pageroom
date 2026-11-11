import { useEffect, useMemo, useState } from "react";
import { FileText, FolderInput } from "lucide-react";
import { api, type TreeNode } from "../api";
import { useStore } from "../store";
import { Dialog } from "../ui";
import { PageTree } from "./PageTree";
import { PageEditor } from "./PageEditor";
import { t } from "../i18n";

export function PagesView({ initial }: { initial?: string }) {
  const { site, pageListHidden, changes, refresh, go, error } = useStore();
  const [selected, setSelected] = useState<string | undefined>(initial);
  const [moving, setMoving] = useState<string | null>(null);
  useEffect(() => { if (initial) setSelected(initial); }, [initial]);
  useEffect(() => {
    const f = (e: Event) => setMoving((e as CustomEvent).detail);
    window.addEventListener("pageroom:move", f);
    return () => window.removeEventListener("pageroom:move", f);
  }, []);
  const changed = useMemo(() => new Set(changes.map((c) => c.path)), [changes]);
  if (!site) return null;
  const labels = site.info.sections ?? {};
  const labelFor = (s: TreeNode) => labels[s.path.split("/").pop() ?? ""]?.label || s.title;

  const open = (p: string) => { setSelected(p); go({ kind: "pages", path: p }); };
  const afterChange = async (p?: string) => {
    await refresh();
    if (p) open(p);
  };

  return (
    <div className={"pages-layout" + (pageListHidden && selected ? " no-tree" : "")}>
      {!(pageListHidden && selected) && <PageTree tree={site.tree} selected={selected} languages={site.info.languages ?? []} changed={changed} onOpen={open} onChanged={afterChange} labelFor={labelFor} />}
      <div style={{ minWidth: 0, minHeight: 0 }}>
        {selected ? (
          <PageEditor key={selected} path={selected} onPathChange={(p) => afterChange(p)} onDeleted={() => { setSelected(undefined); refresh(); }} />
        ) : (
          <div className="empty" style={{ marginTop: 60 }}>
            <div className="art"><FileText size={28} /></div>
            <h3>{t.pages.chooseTitle}</h3>
            <p>{t.pages.chooseHint}</p>
          </div>
        )}
      </div>
      {moving && (
        <MoveDialog
          tree={site.tree}
          labelFor={labelFor}
          onClose={() => setMoving(null)}
          onPick={async (section) => {
            try {
              const np = await api.movePage(moving, section);
              setMoving(null);
              await afterChange(selected === moving ? np : undefined);
            } catch (e) { error(e); }
          }}
        />
      )}
    </div>
  );
}

function MoveDialog({ tree, onClose, onPick, labelFor }: { tree: TreeNode; onClose: () => void; onPick: (s: string) => void; labelFor: (s: TreeNode) => string }) {
  const sections: { node: TreeNode; depth: number }[] = [];
  const walk = (n: TreeNode, d: number) => { sections.push({ node: n, depth: d }); n.children?.filter((c) => c.kind === "section").forEach((c) => walk(c, d + 1)); };
  walk(tree, 0);
  return (
    <Dialog open onClose={onClose} title={t.pages.moveTitle} description={t.pages.moveDescription}>
      <div className="card list" style={{ maxHeight: 360, overflow: "auto" }}>
        {sections.map(({ node, depth }) => (
          <div key={node.path} className="list-item clickable" style={{ paddingLeft: 14 + depth * 18 }} onClick={() => onPick(node.path)}>
            <FolderInput size={15} className="dim" /> {depth === 0 ? t.pages.topLevel : labelFor(node)}
          </div>
        ))}
      </div>
    </Dialog>
  );
}
