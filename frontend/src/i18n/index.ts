import { en, type Messages } from "./en";
import { ja } from "./ja";

const messages = { en, ja } satisfies Record<string, Messages>;

export type Language = keyof typeof messages;

export const languageNames: Record<Language, string> = { en: "English", ja: "日本語" };

export const systemLanguage: Language =
  navigator.languages.map((tag) => tag.slice(0, 2)).find((code): code is Language => code in messages) ?? "en";

export let language: Language = systemLanguage;
export let t: Messages = messages[systemLanguage];

export function setLanguage(saved: string): Language {
  language = saved in messages ? (saved as Language) : systemLanguage;
  t = messages[language];
  document.documentElement.lang = language;
  return language;
}
