import { chromium } from 'playwright';
import fs from 'node:fs/promises';
import path from 'node:path';

function parseArgs(argv) {
	const out = { expectStopped: false };
	for (let i = 0; i < argv.length; i++) {
		const arg = argv[i];
		if (arg === '--expect-stopped') {
			out.expectStopped = true;
			continue;
		}
		if (!arg.startsWith('--')) {
			throw new Error(`unexpected argument ${arg}`);
		}
		const key = arg.slice(2).replace(/-([a-z])/g, (_, c) => c.toUpperCase());
		const value = argv[++i];
		if (!value || value.startsWith('--')) {
			throw new Error(`missing value for ${arg}`);
		}
		out[key] = value;
	}
	for (const key of ['baseUrl', 'domain', 'slug', 'token', 'report']) {
		if (!out[key])
			throw new Error(
				`missing --${key.replace(/[A-Z]/g, (c) => `-${c.toLowerCase()}`)}`,
			);
	}
	return out;
}

function childURL({ domain, slug }) {
	return `https://${slug}.${domain.replace(/^\.+|\.+$/g, '')}/`;
}

async function authenticate(page, { baseUrl, token }) {
	const url = new URL('/internal/playwright-auth', baseUrl);
	url.searchParams.set('token', token);
	url.searchParams.set('redirect', '/workspaces');
	await page.goto(url.toString(), { waitUntil: 'networkidle' });
	if (!page.url().includes('/workspaces')) {
		throw new Error(`auth did not reach /workspaces: ${page.url()}`);
	}
}

async function switchToWorkspace(page, { baseUrl, slug }) {
	const url = new URL(`/workspaces/switch/${slug}`, baseUrl);
	url.searchParams.set('redirect', '/');
	await page.goto(url.toString(), { waitUntil: 'networkidle' });
}

async function assertChildApp(page, { domain, slug }) {
	const expected = childURL({ domain, slug });
	if (!page.url().startsWith(expected)) {
		throw new Error(`expected child URL ${expected}, got ${page.url()}`);
	}
	await page.waitForSelector('body', { timeout: 10000 });
	const text = await page.locator('body').innerText({ timeout: 10000 });
	if (/Workspace unavailable/i.test(text)) {
		throw new Error(
			'child rendered unavailable page while expected running app',
		);
	}
}

async function assertUnavailable(page, { domain, slug }) {
	const expected = childURL({ domain, slug });
	await page.goto(expected, { waitUntil: 'networkidle' });
	const text = await page.locator('body').innerText({ timeout: 10000 });
	if (!/Workspace unavailable/i.test(text)) {
		throw new Error(
			`expected workspace unavailable page after stop at ${expected}`,
		);
	}
}

async function saveArtifacts(page, reportDir, label) {
	const screenshotDir = path.join(reportDir, 'screenshots');
	await fs.mkdir(screenshotDir, { recursive: true });
	await page.screenshot({
		path: path.join(screenshotDir, `${label}.png`),
		fullPage: true,
	});
}

async function main() {
	const cfg = parseArgs(process.argv.slice(2));
	await fs.mkdir(cfg.report, { recursive: true });

	const browser = await chromium.launch();
	const context = await browser.newContext({ ignoreHTTPSErrors: true });
	await context.tracing.start({ screenshots: true, snapshots: true });
	const page = await context.newPage();
	try {
		await authenticate(page, cfg);
		if (cfg.expectStopped) {
			await assertUnavailable(page, cfg);
			await saveArtifacts(page, cfg.report, 'unavailable-after-stop');
			return;
		}
		await page.goto(new URL('/workspaces', cfg.baseUrl).toString(), {
			waitUntil: 'networkidle',
		});
		await saveArtifacts(page, cfg.report, 'manager-workspaces');
		await switchToWorkspace(page, cfg);
		await assertChildApp(page, cfg);
		await saveArtifacts(page, cfg.report, 'child-app');
	} finally {
		await context.tracing.stop({
			path: path.join(cfg.report, 'playwright-trace.zip'),
		});
		await browser.close();
	}
}

main().catch((err) => {
	console.error(err?.stack || err);
	process.exit(1);
});
