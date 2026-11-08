import { Dialog as RDialog, DropdownMenu, Tooltip as RTooltip } from "radix-ui";
import { create } from "zustand";
import { useEffect, useState, type ReactNode } from "react";
import { AlertTriangle, CheckCircle2, Info, Loader2, X, XCircle } from "lucide-react";
import { useStore } from "./store";
import { language, t } from "./i18n";


export function Dialog(props: {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  description?: ReactNode;
  wide?: boolean;
  children?: ReactNode;
  actions?: ReactNode;
  dismissable?: boolean;
}) {
  return (
    <RDialog.Root open={props.open} onOpenChange={(o) => !o && props.dismissable !== false && props.onClose()}>
      <RDialog.Portal>
        <RDialog.Overlay className="overlay" />
        <RDialog.Content className={"dialog" + (props.wide ? " wide" : "")} aria-describedby={undefined} onEscapeKeyDown={(e) => props.dismissable === false && e.preventDefault()}>
          <div className="row" style={{ alignItems: "flex-start" }}>
            <RDialog.Title asChild><h2 style={{ flex: 1 }}>{props.title}</h2></RDialog.Title>
            {props.dismissable !== false && (
              <RDialog.Close className="btn ghost icon sm" aria-label={t.common.close}><X size={16} /></RDialog.Close>
            )}
          </div>
          {props.description && <p className="desc">{props.description}</p>}
          {props.children}
          {props.actions && <div className="dialog-actions">{props.actions}</div>}
        </RDialog.Content>
      </RDialog.Portal>
    </RDialog.Root>
  );
}


interface Ask {
  kind: "prompt" | "confirm";
  title: string;
  label?: string;
  body?: ReactNode;
  placeholder?: string;
  initial?: string;
  confirm?: string;
  danger?: boolean;
  resolve: (v: string | boolean | null) => void;
}

const useAsk = create<{ current: Ask | null; set: (a: Ask | null) => void }>((set) => ({ current: null, set: (current) => set({ current }) }));

export const ask = {
  prompt: (title: string, label: string, placeholder?: string, initial?: string): Promise<string | null> =>
    new Promise((resolve) => useAsk.getState().set({ kind: "prompt", title, label, placeholder, initial, resolve: resolve as Ask["resolve"] })),
  confirm: (title: string, body: ReactNode, confirm = t.common.continue, danger = false): Promise<boolean> =>
    new Promise((resolve) => useAsk.getState().set({ kind: "confirm", title, body, confirm, danger, resolve: (v) => resolve(!!v) })),
};

export function AskHost() {
  const { current, set } = useAsk();
  const [value, setValue] = useState("");
  useEffect(() => setValue(current?.initial ?? ""), [current]);
  if (!current) return null;
  const done = (v: string | boolean | null) => {
    current.resolve(v);
    set(null);
  };
  return (
    <Dialog
      open
      onClose={() => done(current.kind === "confirm" ? false : null)}
      title={current.title}
      actions={
        <>
          <button className="btn" onClick={() => done(current.kind === "confirm" ? false : null)}>{t.common.cancel}</button>
          <button
            className={"btn " + (current.danger ? "danger solid" : "primary")}
            disabled={current.kind === "prompt" && !value.trim()}
            onClick={() => done(current.kind === "prompt" ? value.trim() : true)}
          >
            {current.confirm ?? t.common.ok}
          </button>
        </>
      }
    >
      {current.kind === "prompt" ? (
        <div className="field">
          <label htmlFor="ask-input">{current.label}</label>
          <input
            id="ask-input"
            className="input"
            autoFocus
            value={value}
            placeholder={current.placeholder}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && value.trim() && done(value.trim())}
          />
        </div>
      ) : (
        <div className="muted">{current.body}</div>
      )}
    </Dialog>
  );
}


export function Toasts() {
  const { toasts, dismiss } = useStore();
  return (
    <div className="toasts" aria-live="polite">
      {toasts.map((toast) => (
        <div key={toast.id} className={"toast " + toast.kind}>
          {toast.kind === "success" ? <CheckCircle2 size={18} /> : toast.kind === "error" ? <XCircle size={18} /> : <Info size={18} />}
          <div style={{ flex: 1 }}>
            <div className="t-title">{toast.title}</div>
            {toast.body && <div className="t-body">{toast.body}</div>}
            {toast.action && (
              <button className="btn sm" style={{ marginTop: 8 }} onClick={() => { toast.action!.run(); dismiss(toast.id); }}>{toast.action.label}</button>
            )}
          </div>
          <button className="btn ghost icon sm" style={{ color: "#aaa" }} onClick={() => dismiss(toast.id)} aria-label={t.common.dismiss}><X size={14} /></button>
        </div>
      ))}
    </div>
  );
}


export const Spinner = ({ size = 16 }: { size?: number }) => <Loader2 size={size} className="spin" />;

export function Tip({ label, children }: { label: string; children: ReactNode }) {
  return (
    <RTooltip.Provider delayDuration={400}>
      <RTooltip.Root>
        <RTooltip.Trigger asChild>{children}</RTooltip.Trigger>
        <RTooltip.Portal>
          <RTooltip.Content className="tip" sideOffset={6}>{label}</RTooltip.Content>
        </RTooltip.Portal>
      </RTooltip.Root>
    </RTooltip.Provider>
  );
}

export interface MenuEntry {
  label?: string;
  icon?: ReactNode;
  danger?: boolean;
  separator?: boolean;
  disabled?: boolean;
  run?: () => void;
}

export function Menu({ trigger, items, align = "end" }: { trigger: ReactNode; items: MenuEntry[]; align?: "start" | "end" }) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>{trigger}</DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content className="menu-pop" align={align} sideOffset={4}>
          {items.map((it, i) =>
            it.separator ? (
              <DropdownMenu.Separator key={i} className="sep" />
            ) : (
              <DropdownMenu.Item key={i} className={"mi" + (it.danger ? " danger" : "")} disabled={it.disabled} onSelect={() => it.run?.()}>
                {it.icon}
                {it.label}
              </DropdownMenu.Item>
            ),
          )}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  );
}

export function Banner({ kind, title, children, icon }: { kind: "warn" | "info" | "danger" | "ok"; title?: ReactNode; children?: ReactNode; icon?: ReactNode }) {
  return (
    <div className={"banner " + kind} role={kind === "danger" ? "alert" : "status"}>
      {icon ?? (kind === "ok" ? <CheckCircle2 size={18} color="var(--success)" /> : kind === "info" ? <Info size={18} color="var(--accent)" /> : <AlertTriangle size={18} color={kind === "danger" ? "var(--danger)" : "var(--warning)"} />)}
      <div style={{ flex: 1 }}>
        {title && <strong>{title}</strong>}
        {children}
      </div>
    </div>
  );
}

export function Switch({ on, onChange, label }: { on: boolean; onChange: (v: boolean) => void; label: string }) {
  return <button className="switch" role="switch" aria-checked={on} aria-label={label} data-on={on} onClick={() => onChange(!on)} />;
}

export function timeAgo(iso: string | number | undefined): string {
  if (!iso) return "";
  const time = typeof iso === "number" ? iso : Date.parse(iso);
  if (!time) return "";
  const s = Math.round((Date.now() - time) / 1000);
  if (s < 45) return t.time.justNow;
  const m = Math.round(s / 60);
  if (m < 60) return t.time.minutesAgo(m);
  const h = Math.round(m / 60);
  if (h < 24) return t.time.hoursAgo(h);
  const d = Math.round(h / 24);
  if (d < 30) return t.time.daysAgo(d);
  return formatDate(iso);
}

export function formatDate(iso?: string | number): string {
  if (!iso) return "";
  const time = typeof iso === "number" ? iso : Date.parse(iso);
  if (!time) return String(iso);
  return new Date(time).toLocaleDateString(language, { year: "numeric", month: "short", day: "numeric" });
}

export function bytes(n: number): string {
  if (n > 1 << 20) return (n / (1 << 20)).toFixed(1) + " MB";
  if (n > 1 << 10) return Math.round(n / 1024) + " KB";
  return t.time.bytes(n);
}
