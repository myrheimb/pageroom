# Site configuration

Pageroom works with any Hugo site as it is. To make the editor friendlier for the people who write in it, commit a `.pageroom.yaml` at the site root. Every key is optional.

```yaml
version: 1

# Which Hugo builds the preview. Without this, Pageroom looks for
# HUGO_VERSION in netlify.toml / wrangler.toml / GitHub Actions workflows,
# then .hugo-version and .tool-versions, then uses its default.
hugo:
  version: "0.168.0"

# Friendlier names for content sections in the page tree.
sections:
  posts:
    label: Blog posts
  recipes:
    label: Recipes

# Menus the Menus screen should offer even when they're still empty.
menus: [main, footer]

# Extra front matter fields shown under "Details" in Page settings. Keys with
# their own controls (title, description, image, imageAlt, tags, categories, …) are skipped.
# Custom fields are for metadata. Text and pictures that visitors read belong in
# the page body as blocks, because editors can see and place those on the canvas.
fields:
  - key: subtitle
    label: Subtitle
    type: text            # text | textarea | number | boolean | date | image | list | select | repeat
    help: Shown under the title.
  - key: hero
    label: Big header image
    type: image
    sections: [posts]     # only for pages in these sections
  - key: tagline
    label: Tagline
    type: text
    pages: [content/solution.md]   # only for this page and its language twins, such as content/solution.ja.md
  - key: layout
    label: Layout
    type: select
    options: [default, wide, landing]
  - key: steps
    label: Steps
    type: repeat          # a list of objects, shown as cards below the page body
    pages: [content/solution.md]
    fields:               # the keys of each item, which can hold another repeat
      - key: title
        label: Title
        type: text
      - key: slides
        label: Slides
        type: repeat
        fields:
          - key: before
            label: Before
            type: image
          - key: after
            label: After
            type: image

# A repeat field never shows in Page settings. Each item is a card with the
# controls of its `fields`. A key in an item that `fields` doesn't name shows
# read-only and is kept on save. A new item's keys are saved in the order of
# `fields`, but only in YAML front matter. TOML and JSON pages sort them.

media:
  maxWidth: 2400          # larger photos are scaled down on import
  folder: static/images   # where images for non-bundle pages go

appearance:
  # A partial your theme includes inside <head>. Pageroom detects common
  # ones (extend_head.html, head/custom.html, hooks/head-end.html, …).
  # Set this if yours is different.
  headHook: partials/my-head-extras.html
```

## How Pageroom maps to Hugo files

| In Pageroom | In the site |
|---|---|
| Page | `content/…/name/index.md` (new pages are bundles, so their images live next to them) or `name.md` |
| Section | A folder in `content/`. Its intro page is `_index.md` |
| Order in the tree | `weight` front matter (written as 10, 20, 30… to leave room) |
| Visible / Hidden draft | `draft` front matter |
| Change web address | Renames the file or folder and adds the old URL to `aliases` |
| Move to section | Moves the file (with its images, for bundles) and adds `aliases` |
| Menus | `menus` in the site config (or `config/_default/menus.*`). Entries declared in a page's own front matter move there when a menu is saved |
| Style panel | `assets/css/pageroom.css`, linked from the theme's head hook. Themes that load `assets/css/custom.css` get a marked block in that file instead. |
| Theme gallery | A new folder in `themes/` and `theme` in the config file. Going back deletes the folder and restores the config file. |
| Blocks | Banner (`pr-hero`), Cards, Columns and Button are the paired shortcodes `pr-hero`, `pr-cards` with `pr-card` items, `pr-columns` with two or three `pr-column` items, and `pr-button` in the page body. Inserting one copies its template to `layouts/shortcodes/` unless the site already has a file with that name. Each template carries its own CSS, uses `--pr-primary` when set, and renders its inner content as Markdown. Banner, Cards and Button take `align="center"` or `align="right"`. `pr-columns` takes `layout="2-1"`, `layout="1-2"` or `layout="1-1-1"` for its widths, with three items for `1-1-1`, and `valign="center"` or `valign="bottom"` for where shorter columns sit. When a site opens, each `pr-*` file in `layouts/shortcodes/` that is byte for byte a version Pageroom shipped earlier is replaced with the current template, and shows in the change list. A file you edited is left alone, and a missing file is not created, so a site with its own `pr-columns.html` needs that file updated by hand before these take effect. A Button inside a column keeps its own `align`. Hugo drops a nested Button when a column renders its contents as Markdown, so `pr-align`, `pr-button` and `pr-video` wrap their HTML in comments when their parent is `pr-column`, and `pr-column` leaves what is between those comments alone. A video is `pr-video` with an `id` for YouTube, a `vimeo` id, or a `src` for a video file (a file in the page folder or a full address). Hugo's built-in `youtube` shortcode can't be used inside a column. This needs no change to your Hugo settings. A single paragraph, heading, quote or picture, on the page or in a column, is aligned by wrapping it in `pr-align` with `to="center"` or `to="right"`. |
| Cover picture | `image` and `imageAlt` front matter. The starter themes use `imageAlt` as the alt text. Other themes can print it with `{{ .Params.imageAlt }}`, and some themes don't show the picture at all. |
| Contact form | `{{< contact-form >}}` in the page, and `layouts/shortcodes/contact-form.html` unless the site already has one. The form posts to a form service such as Formspree, and shows the contact email until there is one. The preview doesn't send the form. |
| Languages | Site settings, Site languages. `defaultContentLanguage`, chosen once while the site has no languages, and a `[languages.<code>]` table per language with `languageName` and `weight`, in `config/_default/languages.*` when the site has that file and in the main config otherwise. The default language stays first. A save rewrites the languages table, or each changed entry in `languages.*`, so comments inside it are dropped and comments elsewhere stay. A language that pages still use can't be removed |
| Site settings | `title`, `baseURL`, `locale`/`languageCode`, `copyright`, `theme`, `params.description` and `params.contactForm` (form address and contact email) in the config file, edited in place, keeping comments |

## What Pageroom never touches

- Existing theme files. The gallery only adds theme folders.
- Front matter keys you didn't change, or their order, comments and formatting.
- Page bodies you didn't edit.
- Shortcodes and raw HTML, which show as locked blocks and are saved as written.
