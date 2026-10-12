// Runs the conformance suite's Basic OP plan against a local panel. The
// suite asks for a browser at each authorization step; this driver opens
// those URLs in Chromium, signs in and approves on the panel, and uploads a
// screenshot wherever a module waits for one.
//
// Usage: NODE_TLS_REJECT_UNAUTHORIZED=0 node driver.mjs config.json [results.json] [module...]
// Environment: SUITE (default https://localhost.emobix.co.uk:8443),
// PANEL (default http://127.0.0.1:3001), PANEL_EMAIL, PANEL_PASSWORD.
import { readFile, writeFile } from "node:fs/promises";
import { chromium } from "playwright";

const SUITE = process.env.SUITE ?? "https://localhost.emobix.co.uk:8443";
const PANEL = new URL(process.env.PANEL ?? "http://127.0.0.1:3001").origin;
const EMAIL = process.env.PANEL_EMAIL ?? "admin@example.test";
const PASSWORD = process.env.PANEL_PASSWORD ?? "Password12345";
const PLAN = "oidcc-basic-certification-test-plan";
const VARIANT = { server_metadata: "discovery", client_registration: "static_client" };
// This module checks the error for a browser with no panel session.
const SIGNED_OUT_MODULES = new Set(["oidcc-prompt-none-not-logged-in"]);
const MODULE_TIMEOUT_MS = 4 * 60_000;

const [configPath, resultsPath = "results.json", ...onlyModules] = process.argv.slice(2);
if (!configPath) {
  console.error("usage: node driver.mjs config.json [results.json] [module...]");
  process.exit(2);
}

async function api(method, path, body) {
  const res = await fetch(SUITE + path, {
    method,
    headers: body === undefined ? {} : { "content-type": "application/json" },
    body: body === undefined ? undefined : typeof body === "string" ? body : JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`${method} ${path}: ${res.status} ${await res.text()}`);
  const text = await res.text();
  return text ? JSON.parse(text) : null;
}

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function isVisible(locator) {
  return locator.isVisible().catch(() => false);
}

async function signIn(page) {
  await page.waitForLoadState("networkidle").catch(() => {});
  const email = page.locator('input[type="email"]');
  await email.fill(EMAIL);
  await page.locator('input[type="password"]').fill(PASSWORD);
  if ((await email.inputValue()) !== EMAIL) return;
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await page.waitForURL((url) => url.pathname !== "/login", { timeout: 15_000 }).catch(() => {});
}

// Modules that wait for a screenshot want the panel page that asked the User
// to sign in again or showed the error, not the suite page reached afterwards.
const panelShots = new WeakMap();

async function rememberPanelPage(page) {
  await page.waitForLoadState("networkidle").catch(() => {});
  panelShots.set(page, await page.screenshot({ fullPage: true }));
}

// drive follows one front-channel URL until the browser is back at the suite
// or stops on a page that is not part of the sign-in flow.
async function drive(page, url) {
  await page.goto(url, { waitUntil: "domcontentloaded" }).catch(() => {});
  const deadline = Date.now() + 60_000;
  while (Date.now() < deadline) {
    const current = new URL(page.url());
    if (current.origin === new URL(SUITE).origin) {
      await page.waitForLoadState("networkidle").catch(() => {});
      return "returned";
    }
    if (current.origin !== PANEL) return `stopped at ${current.origin}${current.pathname}`;
    if (current.pathname === "/oidc/error") {
      await rememberPanelPage(page);
      return "panel error page";
    }
    if (current.pathname === "/login" && (await isVisible(page.locator('input[type="email"]')))) {
      await rememberPanelPage(page);
      await signIn(page);
      continue;
    }
    for (const name of ["Allow", "Sign in again"]) {
      const button = page.getByRole("button", { name, exact: true });
      if (await isVisible(button)) {
        if (name === "Sign in again") await rememberPanelPage(page);
        await page.waitForLoadState("networkidle").catch(() => {});
        await button.click().catch(() => {});
        await page.waitForTimeout(1000);
        break;
      }
    }
    await page.waitForTimeout(500);
  }
  return "timeout";
}

async function fillPlaceholders(id, page) {
  const images = await api("GET", `/api/log/${id}/images`);
  const open = images.filter((entry) => entry.upload && !entry.img);
  if (open.length === 0) return false;
  const shot = (panelShots.get(page) ?? (await page.screenshot({ fullPage: true }))).toString("base64");
  for (const entry of open) {
    await api("POST", `/api/log/${id}/images/${entry.upload}`, `data:image/png;base64,${shot}`);
  }
  return true;
}

async function runModule(browser, signedInContext, planId, entry) {
  const variant = entry.variant ? `&variant=${encodeURIComponent(JSON.stringify(entry.variant))}` : "";
  const { id } = await api("POST", `/api/runner?test=${entry.testModule}&plan=${planId}${variant}`);
  const context = SIGNED_OUT_MODULES.has(entry.testModule) ? await browser.newContext({ ignoreHTTPSErrors: true }) : signedInContext;
  const page = await context.newPage();
  const visited = new Set();
  const notes = [];
  const deadline = Date.now() + MODULE_TIMEOUT_MS;
  let info;
  try {
    while (Date.now() < deadline) {
      info = await api("GET", `/api/info/${id}`);
      if (info.status === "FINISHED" || info.status === "INTERRUPTED") break;
      if (info.status === "WAITING") {
        const control = await api("GET", `/api/runner/browser/${id}`).catch(() => null);
        const pending = (control?.urls ?? []).filter((url) => !visited.has(url));
        for (const url of pending) {
          visited.add(url);
          await api("POST", `/api/runner/browser/${id}/visit?url=${encodeURIComponent(url)}`);
          notes.push(await drive(page, url));
        }
        if (pending.length === 0 && (await fillPlaceholders(id, page))) notes.push("uploaded screenshot");
      }
      await sleep(1000);
    }
  } finally {
    await page.close();
    if (context !== signedInContext) await context.close();
  }
  if (info?.status !== "FINISHED" && info?.status !== "INTERRUPTED") {
    await api("DELETE", `/api/runner/${id}`).catch(() => {});
    notes.push("driver timed out");
  }
  const log = await api("GET", `/api/log/${id}`);
  const problems = log
    .filter((line) => line.result === "FAILURE" || line.result === "WARNING")
    .map((line) => `${line.result} ${line.src}: ${line.msg}`);
  return { module: entry.testModule, id, status: info?.status, result: info?.result, notes, problems };
}

const config = await readFile(configPath, "utf8");
const plan = await api("POST", `/api/plan?planName=${PLAN}&variant=${encodeURIComponent(JSON.stringify(VARIANT))}`, config);
console.log(`plan ${plan.id}: ${SUITE}/plan-detail.html?plan=${plan.id}`);

const browser = await chromium.launch();
const signedInContext = await browser.newContext({ ignoreHTTPSErrors: true });
const results = [];
for (const entry of plan.modules) {
  if (onlyModules.length > 0 && !onlyModules.includes(entry.testModule)) continue;
  const outcome = await runModule(browser, signedInContext, plan.id, entry);
  console.log(`${outcome.result ?? outcome.status}\t${outcome.module}\t${outcome.notes.join("; ")}`);
  for (const problem of outcome.problems) console.log(`    ${problem}`);
  results.push(outcome);
}
await browser.close();
await writeFile(resultsPath, JSON.stringify({ plan: plan.id, results }, null, 2));
