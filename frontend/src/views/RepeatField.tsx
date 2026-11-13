import { type FieldDef } from "../api";
import { t } from "../i18n";
import { CustomField } from "./PageSettings";

type Item = Record<string, any>;

interface Props {
  def: FieldDef;
  value: any;
  onChange: (v: any) => void;
  resolveImage: (src: string) => string | null;
  pickImage: () => Promise<{ ref: string; alt?: string } | null>;
  idPrefix?: string;
}

export function RepeatField({ def, value, onChange, resolveImage, pickImage, idPrefix = "rf-" }: Props) {
  const items: Item[] = Array.isArray(value) ? value : [];
  const schema = def.fields ?? [];
  const named = new Set(schema.map((f) => f.key));

  const setKey = (index: number, key: string, v: any) => {
    const next = { ...items[index] };
    if (v === undefined) delete next[key];
    else next[key] = v;
    onChange(items.map((item, i) => (i === index ? next : item)));
  };

  return (
    <section className="repeat" aria-labelledby={idPrefix + "label"}>
      <h3 id={idPrefix + "label"} className="repeat-label">{def.label || def.key}</h3>
      {def.help && <p className="help">{def.help}</p>}
      {items.length === 0 && <p className="help">{t.repeat.empty}</p>}
      {items.map((item, index) => {
        const itemPrefix = `${idPrefix}${index}-`;
        return (
          <div key={index} className="repeat-card" role="group" aria-label={`${def.label || def.key} ${t.repeat.item(index + 1)}`}>
            <h4 className="repeat-item">{t.repeat.item(index + 1)}</h4>
            {schema.map((child) =>
              child.type === "repeat" ? (
                <RepeatField key={child.key} def={child} value={item[child.key]} onChange={(v) => setKey(index, child.key, v)} resolveImage={resolveImage} pickImage={pickImage} idPrefix={itemPrefix + child.key + "-"} />
              ) : (
                <CustomField key={child.key} def={child} value={item[child.key]} onChange={(v) => setKey(index, child.key, v)} resolve={resolveImage} pick={pickImage} idPrefix={itemPrefix} />
              ),
            )}
            {Object.keys(item)
              .filter((key) => !named.has(key))
              .map((key) => (
                <div key={key} className="field">
                  <label htmlFor={itemPrefix + "raw-" + key} className="mono">{t.repeat.unnamedKey(key)}</label>
                  <input id={itemPrefix + "raw-" + key} className="input" readOnly value={typeof item[key] === "string" ? item[key] : JSON.stringify(item[key])} />
                </div>
              ))}
          </div>
        );
      })}
    </section>
  );
}
