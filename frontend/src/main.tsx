import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./styles.css";
import { App } from "./App";
import { api } from "./api";
import { setLanguage } from "./i18n";
import { useStore } from "./store";

async function savedLanguage(): Promise<string> {
  try {
    return (await api.getWelcome()).language;
  } catch {
    return "";
  }
}

useStore.setState({ language: setLanguage(await savedLanguage()) });

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
