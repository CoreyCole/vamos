import { execFile } from "node:child_process";
import { readFile } from "node:fs/promises";
import { dirname } from "node:path";
import { promisify } from "node:util";
import type { ContextUsage, ExtensionAPI, ExtensionCommandContext } from "@earendil-works/pi-coding-agent";

const execFileAsync = promisify(execFile);
const maxCLIOutputBytes = 1024 * 1024;

type QManagerAction = "start-next" | "continue";

type ParsedArgs = {
	action: QManagerAction;
	passthrough: string[];
};

type QManagerCLIResult = {
	exitCode: number;
	stdout: string;
	stderr: string;
	compaction: QManagerCompactionSignal;
};

type QManagerCompactionSignal = {
	started: boolean;
	handoffPath?: string;
	readyCommand?: string;
};

type ExecError = Error & { code?: unknown; stdout?: unknown; stderr?: unknown };

export default function qManagerParentExtension(pi: ExtensionAPI): void {
	pi.registerCommand("q-manager", {
		description: "Start q-manager from conversation context, or run an explicit start-next/continue operation",
		getArgumentCompletions: (prefix) => {
			const actions = ["start-next", "continue"];
			const matches = actions.filter((action) => action.startsWith(prefix.trim()));
			return matches.length > 0 ? matches.map((action) => ({ value: action, label: action })) : null;
		},
		handler: async (args, ctx) => {
			let parsed: ParsedArgs | undefined;
			try {
				parsed = parseArgs(args);
			} catch (error) {
				ctx.ui.notify(error instanceof Error ? error.message : String(error), "error");
				return;
			}

			if (!parsed) {
				await startManagerAgent(pi, args, ctx);
				return;
			}

			const usageFlags = usageFlagsFromContext(ctx.getContextUsage());
			const passthrough = parsed.passthrough.map((arg) => expandShellVariables(arg, ctx.cwd));
			const result = await runQManagerCLI(parsed.action, passthrough, usageFlags, ctx.cwd);
			publishCLIResult(ctx, result);

			if (result.exitCode !== 0) {
				ctx.ui.notify("q-manager CLI failed; parent compaction skipped", "error");
				return;
			}
			if (result.compaction.started) {
				compactParent(ctx, result.compaction);
			}
		},
	});
}

function parseArgs(args: string): ParsedArgs | undefined {
	const trimmed = args.trim();
	const firstToken = trimmed.match(/^\S+/)?.[0];
	if (firstToken !== "start-next" && firstToken !== "continue") return undefined;

	const parts = splitArgs(trimmed);
	const action = parts.shift() as QManagerAction;
	return { action, passthrough: parts };
}

async function startManagerAgent(pi: ExtensionAPI, args: string, ctx: ExtensionCommandContext): Promise<void> {
	const skill = pi.getCommands().find((command) => command.source === "skill" && command.name === "skill:q-manager");
	if (!skill) throw new Error("q-manager skill is not loaded");

	const skillText = await readFile(skill.sourceInfo.path, "utf8");
	const skillBody = skillText.replace(/^---\r?\n[\s\S]*?\r?\n---\r?\n?/, "").trim();
	const startupInput =
		args.trim() ||
		"Start or resume q-manager from the current conversation. Infer the next graph-safe action from the latest available QRSPI result or manager wake.";
	const prompt = `<skill name="q-manager" location="${skill.sourceInfo.path}">\nReferences are relative to ${dirname(skill.sourceInfo.path)}.\n\n${skillBody}\n</skill>\n\nManager startup input:\n${startupInput}`;

	if (ctx.isIdle()) {
		pi.sendUserMessage(prompt);
	} else {
		pi.sendUserMessage(prompt, { deliverAs: "steer" });
	}
}

function splitArgs(input: string): string[] {
	const args: string[] = [];
	let current = "";
	let quote: '"' | "'" | undefined;
	let escaping = false;

	for (const char of input) {
		if (escaping) {
			current += char;
			escaping = false;
			continue;
		}
		if (char === "\\") {
			escaping = true;
			continue;
		}
		if (quote) {
			if (char === quote) {
				quote = undefined;
			} else {
				current += char;
			}
			continue;
		}
		if (char === '"' || char === "'") {
			quote = char;
			continue;
		}
		if (/\s/.test(char)) {
			if (current !== "") {
				args.push(current);
				current = "";
			}
			continue;
		}
		current += char;
	}
	if (escaping) current += "\\";
	if (quote) throw new Error("unterminated quoted argument");
	if (current !== "") args.push(current);
	return args;
}

function expandShellVariables(arg: string, cwd: string): string {
	return arg.replace(/\$\{([A-Za-z_][A-Za-z0-9_]*)\}|\$([A-Za-z_][A-Za-z0-9_]*)/g, (match, braced, bare) => {
		const name = String(braced ?? bare);
		if (name === "PWD") return cwd;
		return process.env[name] ?? "";
	});
}

function usageFlagsFromContext(usage: ContextUsage | undefined): string[] {
	if (!usage) return [];
	if (usage.percent !== null) {
		return ["--manager-usage-percent", usage.percent.toFixed(1), "--manager-usage-source", "pi-extension-context"];
	}
	if (usage.tokens !== null && usage.contextWindow > 0) {
		return [
			"--manager-usage-tokens",
			String(usage.tokens),
			"--manager-usage-window",
			String(usage.contextWindow),
			"--manager-usage-source",
			"pi-extension-context",
		];
	}
	return [];
}

async function runQManagerCLI(
	action: QManagerAction,
	args: string[],
	usageFlags: string[],
	cwd: string,
): Promise<QManagerCLIResult> {
	const bin = process.env.VAMOS_Q_MANAGER_BIN?.trim() || "vamos";
	const cliArgs = ["qrspi", action, ...args, ...usageFlags];
	try {
		const { stdout, stderr } = await execFileAsync(bin, cliArgs, { cwd, maxBuffer: maxCLIOutputBytes });
		return { exitCode: 0, stdout, stderr, compaction: parseCompactionSignal(stdout) };
	} catch (error) {
		if (isExecError(error)) {
			const stdout = String(error.stdout ?? "");
			const stderr = String(error.stderr ?? "");
			const exitCode = typeof error.code === "number" ? error.code : 1;
			return { exitCode, stdout, stderr, compaction: parseCompactionSignal(stdout) };
		}
		throw error;
	}
}

function isExecError(error: unknown): error is ExecError {
	return error instanceof Error && ("stdout" in error || "stderr" in error || "code" in error);
}

function parseCompactionSignal(stdout: string): QManagerCompactionSignal {
	if (!stdout.split(/\r?\n/).some((line) => line.trim() === "q-manager-parent-compact: started")) {
		return { started: false };
	}
	return {
		started: true,
		handoffPath: findLineValue(stdout, "handoff:"),
		readyCommand: findLineValue(stdout, "ready:"),
	};
}

function findLineValue(stdout: string, prefix: string): string | undefined {
	for (const line of stdout.split(/\r?\n/)) {
		if (line.startsWith(prefix)) return line.slice(prefix.length).trim() || undefined;
	}
	return undefined;
}

function publishCLIResult(ctx: ExtensionCommandContext, result: QManagerCLIResult): void {
	const stdoutLine = firstSignificantLine(result.stdout);
	if (stdoutLine) ctx.ui.notify(`q-manager: ${stdoutLine}`, result.exitCode === 0 ? "info" : "error");

	const stderrLine = firstSignificantLine(result.stderr);
	if (stderrLine) ctx.ui.notify(`q-manager stderr: ${stderrLine}`, result.exitCode === 0 ? "warning" : "error");
}

function firstSignificantLine(text: string): string | undefined {
	return text
		.split(/\r?\n/)
		.map((line) => line.trim())
		.find((line) => line.length > 0);
}

function compactParent(ctx: ExtensionCommandContext, signal: QManagerCompactionSignal): void {
	const ready = signal.readyCommand ?? "vamos qrspi manager-ready --state-file <state> --manager-pane $TMUX_PANE";
	const handoff = signal.handoffPath ?? "the q-manager operational handoff printed above";
	ctx.compact({
		customInstructions: `Read q-manager operational handoff: ${handoff}.\nAfter compaction, run exactly once:\n${ready}\nThen follow any flushed q_manager_child_wake.`,
		onComplete: () => ctx.ui.notify("q-manager parent compaction complete; run manager-ready", "info"),
		onError: (error) => ctx.ui.notify(`q-manager parent compaction failed: ${error.message}`, "error"),
	});
	ctx.ui.notify("q-manager parent compaction started", "info");
}
