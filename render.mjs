import fs from 'fs/promises';
import path from 'path';
import process from 'process';
import { fileURLToPath } from 'url';

import matter from 'gray-matter';
import fg from 'fast-glob';
import slugify from 'slugify';
import { Resvg } from '@resvg/resvg-js';
import satori from 'satori';
import { html as parseHtml } from 'satori-html';
import { marked } from 'marked';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const ROOT_DIR = __dirname;
const QUOTES_DIR = path.join(ROOT_DIR, 'quotes');
const OUTPUT_CARD_DIR = path.join(ROOT_DIR, 'cards');
const OUTPUT_WRAPPER_DIR = path.join(ROOT_DIR, 'q');
const OUTPUT_SOURCES_DIR = path.join(ROOT_DIR, 'sources');
const TEMPLATE_DIR = path.join(ROOT_DIR, 'templates');
const FONT_DIR = path.join(ROOT_DIR, 'assets', 'fonts');

const CARD_WIDTH = 1200;
const CARD_HEIGHT = 630;

const BASE_PATH = normalizeBasePath(process.env.BASE_PATH || '');
const SITE_ORIGIN = normalizeOrigin(process.env.SITE_ORIGIN || '');

marked.setOptions({ mangle: false, headerIds: false });

async function main() {
  const args = parseArgs(process.argv.slice(2));

  const { quotes, warnings, errors } = await loadQuotes();

  if (warnings.length) {
    warnings.forEach((msg) => console.warn(`⚠️  ${msg}`));
  }

  if (errors.length) {
    errors.forEach((msg) => console.error(`❌ ${msg}`));
    if (args.check) {
      process.exitCode = 1;
      return;
    }
    throw new Error('Aborting due to validation errors.');
  }

  if (args.check) {
    console.log(`✅ ${quotes.length} quote(s) validated.`);
    return;
  }

  if (!quotes.length) {
    console.log('No quotes found. Exiting without generating assets.');
    return;
  }

  const [wrapperTemplate, sourceTemplate, fonts] = await Promise.all([
    fs.readFile(path.join(TEMPLATE_DIR, 'wrapper.html'), 'utf8'),
    fs.readFile(path.join(TEMPLATE_DIR, 'source.html'), 'utf8'),
    loadFonts(),
  ]);

  await cleanOutputs();
  await Promise.all([
    fs.mkdir(OUTPUT_CARD_DIR, { recursive: true }),
    fs.mkdir(OUTPUT_WRAPPER_DIR, { recursive: true }),
    fs.mkdir(OUTPUT_SOURCES_DIR, { recursive: true }),
  ]);

  const sourceGroups = new Map();

  for (const quote of quotes) {
    const svg = await renderQuoteSvg(quote, fonts);
    const resvg = new Resvg(svg, {
      fitTo: {
        mode: 'width',
        value: CARD_WIDTH,
      },
    });
    const pngBuffer = resvg.render().asPng();

    const cardPath = path.join(OUTPUT_CARD_DIR, `${quote.id}.png`);
    await fs.writeFile(cardPath, pngBuffer);

    const wrapperHtml = applyTemplate(wrapperTemplate, buildWrapperPayload(quote));
    const wrapperDir = path.join(OUTPUT_WRAPPER_DIR, quote.id);
    await fs.mkdir(wrapperDir, { recursive: true });
    await fs.writeFile(path.join(wrapperDir, 'index.html'), wrapperHtml, 'utf8');

    const groupKey = `${quote.sourceDomain}__${quote.articleSlug}`;
    if (!sourceGroups.has(groupKey)) {
      sourceGroups.set(groupKey, {
        domain: quote.sourceDomain,
        slug: quote.articleSlug,
        sourceUrl: quote.normalizedUrl,
        articleTitle: quote.articleTitle,
        quotes: [],
      });
    }
    const entry = sourceGroups.get(groupKey);
    if (!entry.articleTitle && quote.articleTitle) {
      entry.articleTitle = quote.articleTitle;
    }
    entry.quotes.push(quote);
  }

  for (const group of sourceGroups.values()) {
    const getTime = (item) => (item.createdAt ? item.createdAt.getTime() : 0);
    group.quotes.sort((a, b) => getTime(b) - getTime(a));

    const quoteItems = group.quotes
      .map((quote) => buildSourceQuoteHtml(quote))
      .join('\n\n');

    const pageTitle = group.articleTitle
      ? `${group.articleTitle} — ${group.domain}`
      : `Quotes from ${group.domain}`;

    const sourceHtml = applyTemplate(sourceTemplate, {
      page_title: escapeHtml(pageTitle),
      source_domain: escapeHtml(group.domain),
      source_url: group.sourceUrl,
      quote_items: quoteItems,
    });

    const outputDir = path.join(OUTPUT_SOURCES_DIR, group.domain, group.slug);
    await fs.mkdir(outputDir, { recursive: true });
    await fs.writeFile(path.join(outputDir, 'index.html'), sourceHtml, 'utf8');
  }

  console.log(`✨ Rendered ${quotes.length} quote(s) across ${sourceGroups.size} source page(s).`);
}

function parseArgs(argv) {
  return {
    check: argv.includes('--check') || argv.includes('--dry-run') || argv.includes('--validate'),
  };
}

async function loadQuotes() {
  const entries = await fg(['**/*.md'], {
    cwd: QUOTES_DIR,
    onlyFiles: true,
    dot: false,
  });

  const idSet = new Set();
  const urlSet = new Map();
  const warnings = [];
  const errors = [];
  const quotes = [];

  for (const relativePath of entries) {
    const filePath = path.join(QUOTES_DIR, relativePath);
    const raw = await fs.readFile(filePath, 'utf8');
    const parsed = matter(raw.trim());
    const data = parsed.data ?? {};
    const body = parsed.content?.trim() ?? '';

    const id = stringOrNull(data.id);
    const quote = stringOrNull(data.quote);
    const name = stringOrNull(data.name);
    const url = stringOrNull(data.url);
    const articleTitle = stringOrNull(data.article_title) || null;
    const sourceDomain = stringOrNull(data.source_domain) || null;
    const createdAt = parseDate(data.created_at);
    const tags = Array.isArray(data.tags) ? data.tags.map(String) : [];

    const location = path.relative(ROOT_DIR, filePath);
    const fileErrors = [];

    if (!id) fileErrors.push(`${location}: missing required field "id".`);
    if (!quote) fileErrors.push(`${location}: missing required field "quote".`);
    if (!name) fileErrors.push(`${location}: missing required field "name".`);
    if (!url) fileErrors.push(`${location}: missing required field "url".`);

    if (id) {
      if (idSet.has(id)) {
        fileErrors.push(`${location}: duplicate id "${id}".`);
      } else {
        idSet.add(id);
      }
    }

    let normalizedUrl = null;
    if (url) {
      try {
        const urlObj = new URL(url);
        urlObj.hash = '';
        normalizedUrl = urlObj.toString().replace(/\/$/, '');
      } catch (err) {
        fileErrors.push(`${location}: invalid url "${url}".`);
      }
    }

    const inferredDomain = (() => {
      if (!normalizedUrl) return null;
      const { hostname } = new URL(normalizedUrl);
      return hostname;
    })();

    const domain = sourceDomain || inferredDomain;
    if (!domain) {
      warnings.push(`${location}: could not determine source domain.`);
    }

    const articleSlug = normalizedUrl ? buildArticleSlug(normalizedUrl) : null;
    if (!articleSlug) {
      warnings.push(`${location}: could not determine article slug.`);
    }

    if (normalizedUrl) {
      const bucket = urlSet.get(normalizedUrl) || [];
      bucket.push(id || location);
      urlSet.set(normalizedUrl, bucket);
    }

    const bodyHtml = body ? marked(body) : '';

    if (fileErrors.length) {
      errors.push(...fileErrors);
      continue;
    }

    quotes.push({
      id,
      quote,
      name,
      url,
      normalizedUrl,
      articleTitle,
      sourceDomain: domain || 'unknown-source',
      articleSlug: articleSlug || 'index',
      createdAt,
      tags,
      bodyHtml,
      location,
    });
  }

  for (const [pageUrl, ids] of urlSet.entries()) {
    if (ids.length > 1) {
      warnings.push(`Multiple quotes reference the same url (${pageUrl}): ${ids.join(', ')}`);
    }
  }

  return { quotes, warnings, errors };
}

async function cleanOutputs() {
  await Promise.all([
    rmIfExists(OUTPUT_CARD_DIR),
    rmIfExists(OUTPUT_WRAPPER_DIR),
    rmIfExists(OUTPUT_SOURCES_DIR),
  ]);
}

async function rmIfExists(targetPath) {
  await fs.rm(targetPath, { recursive: true, force: true }).catch(() => {});
}

async function loadFonts() {
  const requiredFonts = [
    { file: 'AtkinsonHyperlegible-Regular.ttf', weight: 400 },
    { file: 'AtkinsonHyperlegible-Bold.ttf', weight: 700 },
  ];

  const loaded = [];

  for (const font of requiredFonts) {
    const fullPath = path.join(FONT_DIR, font.file);
    let fontData;
    try {
      fontData = await fs.readFile(fullPath);
    } catch (err) {
      throw new Error(`Font file missing: ${font.file}. Add it to assets/fonts.`);
    }

    loaded.push({
      name: 'Atkinson Hyperlegible',
      data: fontData,
      weight: font.weight,
      style: 'normal',
    });
  }

  return loaded;
}

function buildWrapperPayload(quote) {
  const metaTitle = `“${quote.quote}” — ${quote.name}`;
  const description = quote.articleTitle
    ? `${quote.name} on ${quote.articleTitle}`
    : `${quote.name} on ${quote.sourceDomain}`;

  const cardPath = `/cards/${quote.id}.png`;
  const ogImage = absoluteUrl(cardPath);

  return {
    page_title: escapeHtml(metaTitle),
    meta_description: escapeHtml(description),
    og_title: escapeHtml(metaTitle),
    og_description: escapeHtml(quote.quote),
    og_image: escapeHtml(ogImage),
    canonical_url: quote.url,
    source_url: quote.url,
    quote_text: escapeHtml(quote.quote),
    quote_author: escapeHtml(quote.name),
    article_title: quote.articleTitle ? escapeHtml(quote.articleTitle) : '',
    card_url: escapeHtml(publicPath(cardPath)),
  };
}

function buildSourceQuoteHtml(quote) {
  const parts = [];
  parts.push('<article>');
  parts.push(`  <blockquote>“${escapeHtml(quote.quote)}”</blockquote>`);
  parts.push(`  <cite>${escapeHtml(quote.name)}</cite>`);
  if (quote.bodyHtml) {
    parts.push(`  <div class="body">${quote.bodyHtml}</div>`);
  }
  parts.push('  <div class="meta">');
  parts.push(
    `    <span><a href="${escapeHtml(publicPath(`/q/${quote.id}/`))}">Quote page</a></span>`
  );
  parts.push(
    `    <span><a href="${escapeHtml(publicPath(`/cards/${quote.id}.png`))}">Download PNG</a></span>`
  );
  parts.push('  </div>');
  parts.push('</article>');
  return parts.join('\n');
}

function applyTemplate(template, data) {
  let output = template;

  output = output.replace(/{{#(\w+)}}([\s\S]*?){{\/(\w+)}}/g, (match, key, inner, closingKey) => {
    if (key !== closingKey) return '';
    const value = data[key];
    if (value) {
      return inner;
    }
    return '';
  });

  output = output.replace(/{{(\w+)}}/g, (match, key) => {
    if (Object.prototype.hasOwnProperty.call(data, key)) {
      return data[key];
    }
    return '';
  });

  return output;
}

function buildArticleSlug(urlString) {
  try {
    const url = new URL(urlString);
    const pathSegments = url.pathname.split('/').filter(Boolean);
    if (pathSegments.length === 0) return 'index';
    const raw = pathSegments.join('-');
    const slug = slugify(raw, {
      lower: true,
      strict: true,
      trim: true,
    });
    return slug || 'index';
  } catch (err) {
    return null;
  }
}

function stringOrNull(value) {
  if (value === undefined || value === null) return null;
  const str = String(value).trim();
  return str.length ? str : null;
}

function parseDate(value) {
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  return date;
}

function escapeHtml(value) {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function publicPath(relativePath) {
  const normalized = relativePath.startsWith('/') ? relativePath : `/${relativePath}`;
  return `${BASE_PATH}${normalized}`;
}

function absoluteUrl(relativePath) {
  const pathWithBase = publicPath(relativePath);
  if (!SITE_ORIGIN) {
    return pathWithBase;
  }
  return `${SITE_ORIGIN}${pathWithBase}`;
}

function normalizeBasePath(input) {
  if (!input) return '';
  let result = input.trim();
  if (!result || result === '/') return '';
  if (!result.startsWith('/')) {
    result = `/${result}`;
  }
  return result.replace(/\/+$/, '');
}

function normalizeOrigin(input) {
  if (!input) return '';
  const trimmed = input.trim();
  if (!trimmed) return '';
  return trimmed.replace(/\/$/, '');
}

async function renderQuoteSvg(quote, fonts) {
  const body = `
    <div style="display:flex;width:${CARD_WIDTH}px;height:${CARD_HEIGHT}px;background:#0f172a;color:#f8fafc;padding:80px;box-sizing:border-box;font-family:'Atkinson Hyperlegible';">
      <div style="display:flex;flex-direction:column;justify-content:space-between;width:100%;">
        <div style="display:flex;flex-direction:column;gap:32px;">
          <div style="font-size:80px;line-height:1.05;font-weight:700;">“${escapeHtml(quote.quote)}”</div>
          <div style="font-size:36px;opacity:0.85;">${escapeHtml(quote.name)}</div>
        </div>
        <div style="font-size:28px;opacity:0.65;display:flex;justify-content:space-between;gap:24px;">
          <span>${escapeHtml(quote.sourceDomain)}</span>
          <span>${quote.articleTitle ? escapeHtml(quote.articleTitle) : ''}</span>
        </div>
      </div>
    </div>
  `;

  const svg = await satori(parseHtml(body), {
    width: CARD_WIDTH,
    height: CARD_HEIGHT,
    fonts,
  });

  return svg;
}

main().catch((error) => {
  console.error(error.stack || error.message);
  process.exitCode = 1;
});
