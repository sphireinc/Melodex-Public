#!/usr/bin/env node

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const noticesPath = path.join(root, 'THIRD_PARTY_NOTICES.md');
const requiredFiles = ['LICENSE', 'LEGAL.md', 'THIRD_PARTY_NOTICES.md'];
const errors = [];

function readJSON(relativePath) {
  return JSON.parse(fs.readFileSync(path.join(root, relativePath), 'utf8'));
}

function requireNotice(text, label, needle) {
  if (!text.includes(needle)) {
    errors.push(`${label} is missing from THIRD_PARTY_NOTICES.md: ${needle}`);
  }
}

for (const relativePath of requiredFiles) {
  if (!fs.existsSync(path.join(root, relativePath))) {
    errors.push(`required legal file is missing: ${relativePath}`);
  }
}

const notices = fs.existsSync(noticesPath) ? fs.readFileSync(noticesPath, 'utf8') : '';
const frontendPackage = readJSON('frontend/package.json');
const frontendLock = readJSON('frontend/package-lock.json');
const frontendDependencies = {
  ...(frontendPackage.dependencies ?? {}),
  ...(frontendPackage.devDependencies ?? {}),
};

for (const name of Object.keys(frontendDependencies).sort()) {
  const lockEntry = frontendLock.packages?.[`node_modules/${name}`];
  if (!lockEntry?.version) {
    errors.push(`frontend dependency is missing from package-lock.json: ${name}`);
    continue;
  }
  requireNotice(notices, 'frontend dependency', `${name}@${lockEntry.version}`);
}

const goMod = fs.readFileSync(path.join(root, 'go.mod'), 'utf8');
const directGoModules = [];
let inRequireBlock = false;
for (const rawLine of goMod.split('\n')) {
  const line = rawLine.trim();
  if (line === 'require (') {
    inRequireBlock = true;
    continue;
  }
  if (inRequireBlock && line === ')') {
    inRequireBlock = false;
    continue;
  }
  const candidate = inRequireBlock ? line : line.startsWith('require ') ? line.slice('require '.length) : '';
  if (!candidate || candidate.startsWith('//') || candidate.includes('// indirect')) continue;
  const match = candidate.match(/^(\S+)\s+(\S+)/);
  if (match) directGoModules.push(`${match[1]}@${match[2]}`);
}

for (const moduleVersion of directGoModules.sort()) {
  requireNotice(notices, 'Go dependency', moduleVersion);
}

for (const tool of ['yt-dlp', 'ffmpeg']) {
  requireNotice(notices, 'optional external tool', tool);
}

if (errors.length > 0) {
  console.error('Third-party notice check failed:');
  for (const error of errors) console.error(`- ${error}`);
  process.exit(1);
}

console.log(`Third-party notice check passed for ${Object.keys(frontendDependencies).length} frontend declarations and ${directGoModules.length} direct Go modules.`);
