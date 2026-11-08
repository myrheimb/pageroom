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
# their own controls (title, description, image, tags, categories, …) are skipped.
fields:
  - key: subtitle
    label: Subtitle
    type: text            # text | textarea | number | boolean | date | image | list | select
    help: Shown under the title.
  - key: hero
    label: Big header image
    type: image
    sections: [posts]     # only for pages in these sections
  - key: layout
    label: Layout
    type: select
    options: [default, wide, landing]

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
| Contact form | `{{< contact-form >}}` in the page, and `layouts/shortcodes/contact-form.html` unless the site already has one. The form posts to a form service such as Formspree, and shows the contact email until there is one. The preview doesn't send the form. |
| Site settings | `title`, `baseURL`, `locale`/`languageCode`, `copyright`, `theme`, `params.description` and `params.contactForm` (form address and contact email) in the config file, edited in place, keeping comments |

## What Pageroom never touches

- Existing theme files. The gallery only adds theme folders.
- Front matter keys you didn't change, or their order, comments and formatting.
- Page bodies you didn't edit.
- Shortcodes and raw HTML, which show as locked blocks and are saved as written.
