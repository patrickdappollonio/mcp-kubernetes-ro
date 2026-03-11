#!/usr/bin/env node

import fs from "node:fs/promises";
import path from "node:path";
import os from "node:os";
import { fileURLToPath } from "node:url";
import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const repoRoot = path.join(__dirname, "..", "..");

const releaseVersion = process.env.RELEASE_VERSION;
if (!releaseVersion) {
  console.error("RELEASE_VERSION is required");
  process.exit(1);
}

const distDir = path.join(repoRoot, "dist");
const artifactsDir = process.env.ARTIFACTS_DIR ?? distDir;
const npmDistDir = path.join(distDir, "npm");
const stagingDir = path.join(npmDistDir, "package");
const tarballName = `mcp-kubernetes-ro-npm-${releaseVersion}.tgz`;
const tarballPath = path.join(npmDistDir, tarballName);

const TARGETS = [
  { key: "darwin-arm64", goos: "darwin", goarch: "arm64", platform: "darwin", arch: "arm64" },
  { key: "darwin-x64", goos: "darwin", goarch: "amd64", platform: "darwin", arch: "x64" },
  { key: "linux-arm64", goos: "linux", goarch: "arm64", platform: "linux", arch: "arm64" },
  { key: "linux-x64", goos: "linux", goarch: "amd64", platform: "linux", arch: "x64" },
  { key: "win32-arm64", goos: "windows", goarch: "arm64", platform: "win32", arch: "arm64" },
  { key: "win32-x64", goos: "windows", goarch: "amd64", platform: "win32", arch: "x64" }
];

async function main() {
  await fs.rm(stagingDir, { recursive: true, force: true });
  await fs.mkdir(stagingDir, { recursive: true });
  await fs.mkdir(npmDistDir, { recursive: true });

  await writePackageJson();
  await copyStaticAssets();

  for (const target of TARGETS) {
    const archiveBase = archiveName(target.goos, target.goarch);
    const archivePath = await resolveArchivePath(archiveBase);
    if (!archivePath) {
      console.warn(`Skipping ${target.key}; archive ${archiveBase} not found in ${artifactsDir}`);
      continue;
    }
    await stageBinary(target, archivePath);
  }

  await packTarball();
  console.log(`Created ${tarballPath}`);
}

async function writePackageJson() {
  const pkgPath = path.join(repoRoot, "npm", "package.json");
  const pkgRaw = await fs.readFile(pkgPath, "utf8");
  const pkg = JSON.parse(pkgRaw);
  pkg.version = releaseVersion;
  const destPath = path.join(stagingDir, "package.json");
  await fs.writeFile(destPath, `${JSON.stringify(pkg, null, 2)}\n`, "utf8");
}

async function copyStaticAssets() {
  await fs.cp(path.join(repoRoot, "npm", "bin"), path.join(stagingDir, "bin"), {
    recursive: true
  });
  await fs.chmod(path.join(stagingDir, "bin", "mcp-kubernetes-ro.js"), 0o755);
  await copyDocAsset("README.md");
  await copyDocAssetOptional("LICENSE");
  await copyDocAssetOptional("kubernetes-ro.png");
}

function archiveName(goos, goarch) {
  const archSuffix = goarch === "amd64" ? "x86_64" : goarch === "386" ? "i386" : goarch;
  return `mcp-kubernetes-ro_${goos}_${archSuffix}`;
}

async function resolveArchivePath(baseName) {
  const candidates = [
    path.join(artifactsDir, `${baseName}.tar.gz`),
    path.join(artifactsDir, `${baseName}.zip`)
  ];
  for (const candidate of candidates) {
    try {
      await fs.access(candidate);
      return candidate;
    } catch {
      // ignore
    }
  }
  return null;
}

async function stageBinary(target, archivePath) {
  const tmpDir = await fs.mkdtemp(path.join(os.tmpdir(), "mcp-npm-"));
  try {
    await extractArchive(archivePath, tmpDir);
    const binaryName = target.platform === "win32" ? "mcp-kubernetes-ro.exe" : "mcp-kubernetes-ro";
    const binarySrc = await findBinary(tmpDir, binaryName);
    if (!binarySrc) {
      throw new Error(`Unable to find ${binaryName} inside ${archivePath}`);
    }
    const destDir = path.join(stagingDir, "vendor", target.key);
    await fs.mkdir(destDir, { recursive: true });
    const destPath = path.join(destDir, binaryName);
    await fs.copyFile(binarySrc, destPath);
    if (target.platform !== "win32") {
      await fs.chmod(destPath, 0o755);
    }
    console.log(`Staged binary for ${target.key}`);
  } finally {
    await fs.rm(tmpDir, { recursive: true, force: true });
  }
}

async function extractArchive(archivePath, destDir) {
  if (archivePath.endsWith(".zip")) {
    await execFileAsync("unzip", ["-o", "-q", archivePath, "-d", destDir]);
  } else if (archivePath.endsWith(".tar.gz")) {
    await execFileAsync("tar", ["-xzf", archivePath, "-C", destDir]);
  } else {
    throw new Error(`Unsupported archive format: ${archivePath}`);
  }
}

async function findBinary(rootDir, binaryName) {
  const stack = [rootDir];
  while (stack.length > 0) {
    const dir = stack.pop();
    const entries = await fs.readdir(dir, { withFileTypes: true });
    for (const entry of entries) {
      const fullPath = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        stack.push(fullPath);
      } else if (entry.isFile() && entry.name === binaryName) {
        return fullPath;
      }
    }
  }
  return null;
}

async function packTarball() {
  const { stdout } = await execFileAsync(
    "npm",
    ["pack", "--ignore-scripts", "--json", "--pack-destination", npmDistDir],
    { cwd: stagingDir }
  );
  const packInfo = JSON.parse(stdout.trim());
  const filename = packInfo.at(-1)?.filename;
  if (!filename) {
    throw new Error("npm pack did not return a filename");
  }
  const packedPath = path.join(npmDistDir, filename);
  await fs.rename(packedPath, tarballPath);
}

async function copyDocAsset(relativePath) {
  const customPath = path.join(repoRoot, "npm", relativePath);
  const fallbackPath = path.join(repoRoot, relativePath);
  let source;
  if (await fileExists(customPath)) {
    source = customPath;
  } else if (await fileExists(fallbackPath)) {
    source = fallbackPath;
  } else {
    throw new Error(`Unable to find ${relativePath} in npm/ or repository root`);
  }
  await fs.copyFile(source, path.join(stagingDir, relativePath));
}

async function copyDocAssetOptional(relativePath) {
  const customPath = path.join(repoRoot, "npm", relativePath);
  const fallbackPath = path.join(repoRoot, relativePath);
  let source;
  if (await fileExists(customPath)) {
    source = customPath;
  } else if (await fileExists(fallbackPath)) {
    source = fallbackPath;
  } else {
    console.warn(`Skipping ${relativePath}; not found in npm/ or repository root`);
    return;
  }
  await fs.copyFile(source, path.join(stagingDir, relativePath));
}

async function fileExists(filePath) {
  try {
    await fs.access(filePath);
    return true;
  } catch {
    return false;
  }
}

await main();
