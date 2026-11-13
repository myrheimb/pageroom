import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { markdownToHtml, unb64 } from "./markdown";

const goldenDirectory = join(__dirname, "../../../internal/wordpress/testdata/golden");

// Shortcodes the importer leaves locked in the editor.
const allowedShortcodes = ["audio", "contact-form", "gist", "google-map", "video", "youtube"];

const shortcodeName = (raw: string) => /^\{\{[<%]\s*\/?\s*([\w\-./]+)/.exec(raw)?.[1] ?? raw;
const attributeValues = (html: string, attribute: string) => [...html.matchAll(new RegExp(`${attribute}="([^"]*)"`, "g"))].map((match) => unb64(match[1]));

const goldenFiles = readdirSync(goldenDirectory).filter((name) => name.endsWith(".md"));

describe("imported pages are editable", () => {
  it("has golden files", () => {
    expect(goldenFiles.length).toBeGreaterThan(0);
  });

  it.each(goldenFiles)("%s has no raw HTML and only allowed shortcodes", (name) => {
    const html = markdownToHtml(readFileSync(join(goldenDirectory, name), "utf8"));

    expect(attributeValues(html, "data-raw-inline")).toEqual([]);
    expect(attributeValues(html, "data-raw-html").filter((raw) => raw !== "<!--more-->")).toEqual([]);

    const shortcodes = [...attributeValues(html, "data-shortcode"), ...attributeValues(html, "data-shortcode-inline")];
    const names = shortcodes.map(shortcodeName);
    expect(names.filter((shortcode) => !allowedShortcodes.includes(shortcode))).toEqual([]);
  });
});
