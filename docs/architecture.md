# Architecture

```
Wails window
  frontend/             React and TypeScript
    views/              Dashboard, Pages, Images, Menus, Appearance, Settings
    editor/             TipTap block editor, Markdown bridge
    dialogs/            Publish, conflicts, sign-in, WordPress import
    i18n/               English and Japanese text
        | generated bindings (frontend/bindings)
  main.go               window setup, embedded frontend, own-origin check
  internal/app          the App service the frontend calls, errors in plain words
  internal/site         content tree, pages, menus, settings
  internal/frontmatter  lossless YAML, TOML and JSON editing
  internal/gitsync      Get latest and Publish (go-git)
  internal/merge        three-way merge
  internal/media        image import and cleaning
  internal/appearance   style tokens to CSS, theme hooks
  internal/hugo         pinned Hugo download, preview server
  internal/secrets      keychain, redaction, scanning, OAuth
  internal/update       newer-release check
  internal/wordpress    WXR import
  internal/starter      embedded starter site and themes
  internal/themes       theme gallery index, safe unpacking
  internal/workers      upload a built site to Cloudflare
```

## Decisions

Go and Wails v3 give one small native binary per platform that uses the system webview. go-git means users don't need git installed.

The file system is the source of truth. There is no database. The editor saves to disk as you type, atomically, and "unpublished changes" is `git status` plus unpushed commits. A file watcher refreshes the UI when someone edits the site in another app, and the save hash check catches clashes.

Publish integrates, commits and pushes with compare-and-swap. `gitsync.integrate` treats the working folder as ours, the remote branch as theirs, and their merge base as base. Files only they changed are written, and files both changed go through `merge.Merge`. Any conflict stops everything and goes to the UI. Otherwise the branch and index move to the remote commit, so whatever differs from it is our work. Pageroom commits that and pushes, requiring the remote hash it merged with, and starts over if the push is rejected.

Without a git host, Pageroom builds the site with the preview's Hugo and uploads it to Cloudflare as a Worker. Changes are committed locally only after the upload succeeds.

The editor converts Markdown to HTML (markdown-it) to ProseMirror, and back with Turndown. Shortcodes, raw HTML and `<!--more-->` are lifted out as locked nodes that keep their exact text. Hero, Cards (with Card items) and Button are the shortcodes `pr-hero`, `pr-cards`, `pr-card` and `pr-button`. They load as editable TipTap nodes and write back as the same shortcodes, and anything the editor can't represent stays a locked block. The cover picture sits on the canvas and is saved as `image` and `imageAlt`, so the right panel holds only metadata. A body is only re-serialized if the user edits it. `markdown.test.ts` pins the round trip.

Front matter edits are surgical. YAML goes through `yaml.Node` to keep comments and order, TOML is edited line by line, and JSON keeps key order. Unchanged documents stay byte-identical.

The UI's text is in `frontend/src/i18n`. `en.tsx` is the source and other languages are typed against it, so a missing string fails the type check. Go errors carry a code, and the frontend shows the message in the chosen language.

The preview runs `hugo server --renderToMemory --bind 127.0.0.1`. An installed Hugo of the site's version is used, and otherwise Pageroom asks before downloading a private copy.

## Other static site generators

The Hugo-specific parts are `internal/site`, `internal/hugo` and `internal/appearance`. The editor, front matter, git sync, merge, media and secrets packages don't depend on Hugo. An adapter would provide a content model and a preview runner behind the same App API.
