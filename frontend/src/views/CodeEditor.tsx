import CodeMirror from "@uiw/react-codemirror";
import { css } from "@codemirror/lang-css";
import { keymap, EditorView } from "@codemirror/view";
import { t } from "../i18n";

const dark = typeof window !== "undefined" && window.matchMedia?.("(prefers-color-scheme: dark)").matches;

export default function CodeEditor({ value, onChange, onSave }: { value: string; onChange: (v: string) => void; onSave: () => void }) {
  return (
    <CodeMirror
      value={value}
      height="100%"
      minHeight="420px"
      theme={dark ? "dark" : "light"}
      placeholder={t.appearance.cssPlaceholder}
      extensions={[
        css(),
        EditorView.lineWrapping,
        keymap.of([{ key: "Mod-s", preventDefault: true, run: () => { onSave(); return true; } }]),
      ]}
      onChange={onChange}
      basicSetup={{ lineNumbers: true, foldGutter: false, highlightActiveLine: true }}
    />
  );
}
