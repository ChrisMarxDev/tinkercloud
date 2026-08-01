import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import test from "node:test";

const config = JSON.parse(readFileSync(new URL("../wrangler.jsonc", import.meta.url), "utf8"));
const workflow = readFileSync(
  new URL("../../.github/workflows/deploy-landing.yml", import.meta.url),
  "utf8",
);

test("defines the typed Cloudflare Worker deployment boundary", () => {
  assert.equal(config.name, "tinkercloud-landing");
  assert.equal(config.main, "./worker/index.ts");
  assert.equal(config.compatibility_date, "2026-07-31");
  assert.ok(config.compatibility_flags.includes("nodejs_compat"));
  assert.equal(config.workers_dev, false);
  assert.equal(config.preview_urls, false);
  assert.deepEqual(config.routes, [
    {
      pattern: "tinkercloud.fun",
      custom_domain: true,
    },
  ]);
  assert.equal(config.assets.binding, "ASSETS");
  assert.equal(config.images.binding, "IMAGES");
  assert.equal(config.observability.enabled, true);
  assert.equal(config.observability.logs.enabled, true);
  assert.equal(config.observability.traces.enabled, true);
});

test("denies legacy hosts, implicit or additional routes, and embedded credentials", () => {
  assert.equal(config.pages_build_output_dir, undefined);
  assert.equal(config.route, undefined);
  assert.equal(config.routes.length, 1);
  assert.doesNotMatch(config.routes[0].pattern, /[/*]/);
  assert.equal(existsSync(new URL("../.openai/hosting.json", import.meta.url)), false);

  const serialized = JSON.stringify(config);
  assert.doesNotMatch(serialized, /(?:api[_-]?key|password|secret|token)/i);
});

test("deploys from main with environment-scoped credentials and public verification", () => {
  assert.match(workflow, /branches:\s*\n\s*- main/);
  assert.match(workflow, /environment:\s*\n\s*name: landing-production/);
  assert.match(workflow, /permissions:\s*\n\s*contents: read/);
  assert.match(
    workflow,
    /apiToken: \$\{\{ secrets\.CLOUDFLARE_API_TOKEN \}\}/,
  );
  assert.match(
    workflow,
    /accountId: \$\{\{ secrets\.CLOUDFLARE_ACCOUNT_ID \}\}/,
  );
  assert.match(workflow, /https:\/\/tinkercloud\.fun\//);
  assert.match(
    workflow,
    /<title>Tinkercloud — Your small apps, securely shared<\/title>/,
  );
  assert.match(workflow, /Open source\. Your VPS\. Your Tinkercloud\./);
  assert.doesNotMatch(workflow, /pull_request:/);
});
