export const accent = "#B5452C";

const nav = ["Home", "Blog", "About"];

function page(active: string, main: string): string {
  const links = nav.map((name) => `<a href="#"${name === active ? ' class="on"' : ""}>${name}</a>`).join("");
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<style>
  body { margin: 0; background: #faf9f6; color: #1a1a1a; font: 17px/1.6 -apple-system, "Helvetica Neue", Arial, sans-serif; }
  a { color: ${accent}; }
  header { display: flex; align-items: center; justify-content: space-between; padding: 0 24px; height: 64px; border-bottom: 1px solid #e8e6df; }
  header strong { font: 700 22px Georgia, "Times New Roman", serif; }
  nav a { margin-left: 22px; color: inherit; text-decoration: none; }
  nav a.on { color: ${accent}; }
  main { padding: 48px 24px; max-width: 760px; }
  h1, h2, h3 { font-family: Georgia, "Times New Roman", serif; line-height: 1.2; }
  h1 { font-size: 40px; margin: 0 0 12px; }
  .meta { color: #666; margin: 0 0 24px; }
  blockquote { margin: 24px 0; padding-left: 20px; border-left: 3px solid ${accent}; color: #555; }
  .card { border: 1px solid #e8e6df; border-radius: 10px; padding: 18px 22px; background: #fff; }
  .card h3 { margin: 0 0 4px; }
  footer { padding: 24px; border-top: 1px solid #e8e6df; color: #666; }
</style>
</head>
<body>
<header><strong>Hannah's Bakery</strong><nav>${links}</nav></header>
<main>${main}</main>
<footer>&copy; 2026 Hannah's Bakery</footer>
</body>
</html>`;
}

const home = page(
  "Home",
  `<h1>Fresh bread, every morning</h1>
<p>We bake sourdough, rolls and rye in the old mill on Water Street. Come early, it sells out.</p>
<h2>Latest posts</h2>
<div class="card"><h3>Hello, world</h3><div class="meta">Jan 1, 2026</div><p>Our sourdough is baked fresh every morning. Here is how a loaf gets from the starter to your table.</p></div>`,
);

const post = page(
  "Blog",
  `<h1>Hello, world</h1>
<p class="meta">January 1, 2026 &middot; <a href="#">news</a></p>
<p>Our sourdough is baked fresh every morning, and we will share photos of every loaf soon.</p>
<blockquote>Best bread in town, says everyone on our street.</blockquote>
<p>When the next loaf comes out of the oven, we will post it here.</p>`,
);

export function renderSite(url: string): string {
  return url.startsWith("/posts/") ? post : home;
}
