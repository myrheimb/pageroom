import type { FieldDef } from "../api";

// content/solution.ja.md is the language twin of content/solution.md.
const withoutLanguage = (path: string) => path.replace(/\.[a-z]{2,3}(-[A-Za-z]+)?\.md$/, ".md");

// The custom fields from .pageroom.yaml that apply to this page.
export function scopedFields(defs: FieldDef[], path: string): FieldDef[] {
  const section = path.split("/")[1] ?? "";
  return defs.filter((f) => (!f.sections?.length || f.sections.includes(section)) && (!f.pages?.length || f.pages.some((p) => withoutLanguage(p) === withoutLanguage(path))));
}
