import assert from "node:assert/strict";
import { access, readFile } from "node:fs/promises";
import test from "node:test";

async function render() {
  const workerUrl = new URL("../dist/server/index.js", import.meta.url);
  workerUrl.searchParams.set("test", `${process.pid}-${Date.now()}`);
  const { default: worker } = await import(workerUrl.href);

  return worker.fetch(
    new Request("https://tinkercloud.test/", {
      headers: { accept: "text/html" },
    }),
    {
      ASSETS: {
        fetch: async () => new Response("Not found", { status: 404 }),
      },
    },
    {
      waitUntil() {},
      passThroughOnException() {},
    },
  );
}

test("server-renders the complete Tinkercloud landing page", async () => {
  const response = await render();
  assert.equal(response.status, 200);
  assert.match(response.headers.get("content-type") ?? "", /^text\/html\b/i);

  const html = await response.text();
  assert.match(html, /<title>Tinkercloud — Turn small apps into trusted team tools<\/title>/i);
  assert.match(html, /Turn small apps into/);
  assert.match(html, /trusted team tools\./);
  assert.match(html, /Self-hosted/);
  assert.match(html, /Private by default/);
  assert.match(html, /Agent-ready/);
  assert.match(html, /coding agents create dashboards, prototypes, reports, and utilities/i);
  assert.match(html, /From folder to secure webapp\./);
  assert.match(html, /The small essentials\./);
  assert.match(html, /Live sockets/);
  assert.match(html, /@tinkercloud\/sdk/);
  assert.match(html, /tinker\.user\.current/);
  assert.match(html, /tinker\.live\.onKvChange/);
  assert.match(
    html,
    /aria-label="tinker deploy \. --allow &#x27;\*@acme\.com&#x27;"/,
  );
  assert.match(html, /one VPS you control/i);
  assert.match(html, /See one deploy/);
  assert.match(html, /Setup on VPS/);
  assert.match(html, /terminal-toolbar/);
  assert.match(html, /terminal-lights/);
  assert.match(html, /Runs on your VPS/);
  assert.match(html, /Open source on GitHub/);
  assert.match(html, /View on GitHub/);
  assert.match(html, /How to host it/);
  assert.match(html, /One VPS\. One Tinkercloud\./);
  assert.match(html, /Read the README/);
  assert.match(html, /background-highlights/);
  assert.match(html, /aria-hidden="true"/);
  assert.match(html, /AI-ready/);
  assert.doesNotMatch(html, /LLM capability|post-V1|when enabled/);
  assert.match(
    html,
    /href="https:\/\/github\.com\/ChrisMarxDev\/tinkercloud#readme"/,
  );
  assert.equal(
    html.match(/href="https:\/\/github\.com\/ChrisMarxDev\/tinkercloud"/g)?.length,
    2,
  );
  assert.doesNotMatch(html, /Abode|Bloom|Lulu|flower|mascot/i);
  assert.doesNotMatch(html, /--allow \*@acme\.com/);
  assert.doesNotMatch(html, /Release activated|Anonymous access denied|Allowed viewers/);
  assert.doesNotMatch(html, /launch-map\.apps\.acme\.com/);
  assert.doesNotMatch(html, /Copy deploy command|Copy private URL/);
  assert.doesNotMatch(html, /Deploying to edge/i);
  assert.doesNotMatch(html, /codex-preview|react-loading-skeleton/i);
});

test("keeps the static landing page narrow and private by design", async () => {
  const [page, layout, styles, packageJson] = await Promise.all([
    readFile(new URL("../app/page.tsx", import.meta.url), "utf8"),
    readFile(new URL("../app/layout.tsx", import.meta.url), "utf8"),
    readFile(new URL("../app/globals.css", import.meta.url), "utf8"),
    readFile(new URL("../package.json", import.meta.url), "utf8"),
  ]);

  assert.doesNotMatch(page, /<form|<input|<script|localStorage|fetch\(/i);
  assert.doesNotMatch(page, /authorized\s*=\s*true/i);
  assert.doesNotMatch(packageJson, /react-loading-skeleton/);
  assert.match(page, /title: "Blob storage"/);
  assert.match(page, /title: "AI-ready"/);
  assert.match(page, /<div className="how" id="how-it-works"/);
  assert.match(styles, /\.how\s*\{[^}]*grid-column: 1 \/ -1;/s);
  assert.match(styles, /\.deploy-command\s*\{[^}]*width: min\(620px/s);
  assert.match(page, /src="\/tinkercloud-mark\.svg"/);
  assert.match(page, /function BackgroundHighlights/);
  assert.match(page, /<DeployCommand \/>/);
  assert.match(styles, /@media \(prefers-reduced-motion: reduce\)/);
  assert.doesNotMatch(styles, /receipt-|deploy-reveal/);
  assert.match(layout, /@fontsource-variable\/fredoka/);
  assert.doesNotMatch(layout, /og\.png|summary_large_image/);
  await access(
    new URL("../public/tinkercloud-mark.svg", import.meta.url),
  );
  await assert.rejects(
    access(new URL("../app/_sites-preview/SkeletonPreview.tsx", import.meta.url)),
  );
});
