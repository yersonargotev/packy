import { createHash } from "node:crypto";
import { cpSync, existsSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { join, relative } from "node:path";
import { tmpdir } from "node:os";

const scriptsDirectory = import.meta.dir;
const nodeModulesDirectory = join(scriptsDirectory, "node_modules");
const commanderPackagePath = join(
  nodeModulesDirectory,
  "commander",
  "package.json"
);
const installKeyPath = join(
  nodeModulesDirectory,
  ".poteto-mode-tools-install-key"
);

function currentInstallKey(): string {
  return createHash("sha256")
    .update(readFileSync(join(scriptsDirectory, "package.json")))
    .update("\0")
    .update(readFileSync(join(scriptsDirectory, "bun.lock")))
    .digest("hex");
}

export function ensureDependenciesInstalled(): void {
  // Packy owns the installed skill tree. Runtime dependency installation must
  // not add node_modules or lock markers to its receipt-owned closure.
  if (process.env.PSTACK_HELPER_WORKSPACE !== scriptsDirectory) {
    const workspace = mkdtempSync(join(tmpdir(), "pstack-tools-"));
    const staged = join(realpathSync(workspace), "scripts");
    try {
      cpSync(scriptsDirectory, staged, {
        recursive: true,
        filter: (source) => !relative(scriptsDirectory, source).split("/").includes("node_modules"),
      });
      const entry = join(staged, relative(scriptsDirectory, process.argv[1]));
      const child = Bun.spawnSync([process.execPath, entry, ...process.argv.slice(2)], {
        cwd: process.cwd(),
        env: { ...process.env, PSTACK_HELPER_WORKSPACE: staged },
        stdin: "inherit",
        stdout: "inherit",
        stderr: "inherit",
      });
      process.exitCode = child.exitCode;
    } finally {
      rmSync(workspace, { recursive: true, force: true });
    }
    process.exit(process.exitCode ?? 1);
  }

  const installKey = currentInstallKey();
  if (
    existsSync(commanderPackagePath) &&
    existsSync(installKeyPath) &&
    readFileSync(installKeyPath, "utf8").trim() === installKey
  ) {
    return;
  }

  const result = Bun.spawnSync(
    [process.execPath, "install", "--frozen-lockfile"],
    { cwd: scriptsDirectory }
  );
  if (result.exitCode !== 0) {
    process.stdout.write(result.stdout);
    process.stderr.write(result.stderr);
    throw new Error(
      `bun install --frozen-lockfile exited with status ${result.exitCode}`
    );
  }
  if (!existsSync(commanderPackagePath)) {
    throw new Error(
      "bun install --frozen-lockfile completed without installing commander"
    );
  }

  writeFileSync(installKeyPath, `${installKey}\n`);

  const restarted = Bun.spawnSync([process.execPath, ...process.argv.slice(1)], {
    cwd: process.cwd(),
    env: process.env,
    stdin: "inherit",
    stdout: "inherit",
    stderr: "inherit",
  });
  process.exit(restarted.exitCode ?? 1);
}
