import { Fragment, useEffect } from "react";
import { useStore } from "./store";
import { api, on } from "./api";
import { Welcome } from "./views/Welcome";
import { Workspace } from "./views/Workspace";
import { AskHost, Toasts } from "./ui";

export function App() {
  const site = useStore((s) => s.site);
  const language = useStore((s) => s.language);
  // With no site open there's nothing to save: close right away.
  useEffect(() => (site ? undefined : on("app:closing", () => api.confirmClose())), [site]);
  // Remount everything so text picked outside render follows the new language.
  return (
    <Fragment key={language}>
      {site ? <Workspace /> : <Welcome />}
      <AskHost />
      <Toasts />
    </Fragment>
  );
}
