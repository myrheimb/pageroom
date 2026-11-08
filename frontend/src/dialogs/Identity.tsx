import { useEffect, useState } from "react";
import { api } from "../api";
import { useStore } from "../store";
import { Dialog } from "../ui";
import { t } from "../i18n";

export function IdentityDialog({ onClose, onSaved }: { onClose: () => void; onSaved: () => void }) {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const error = useStore((s) => s.error);
  useEffect(() => {
    api.getWelcome().then((w) => { setName(w.name); setEmail(w.email); }).catch(() => {});
  }, []);
  const save = async () => {
    try {
      await api.saveIdentity(name, email);
      onSaved();
    } catch (e) {
      error(e);
    }
  };
  return (
    <Dialog
      open
      onClose={onClose}
      title={t.identity.title}
      description={t.identity.description}
      actions={
        <>
          <button className="btn" onClick={onClose}>{t.common.cancel}</button>
          <button className="btn primary" disabled={!name.trim()} onClick={save}>{t.common.save}</button>
        </>
      }
    >
      <div className="field">
        <label htmlFor="id-name">{t.identity.yourName}</label>
        <input id="id-name" className="input" autoFocus value={name} onChange={(e) => setName(e.target.value)} />
      </div>
      <div className="field">
        <label htmlFor="id-email">{t.identity.email} <span className="dim">{t.common.optional}</span></label>
        <input id="id-email" className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
        <span className="help">{t.identity.emailHelp}</span>
      </div>
    </Dialog>
  );
}
