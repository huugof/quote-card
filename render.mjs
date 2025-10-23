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
import { encode as encodeJpeg } from 'jpeg-js';

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
const CARD_HEIGHT = 628;
const CARD_PADDING_X = 150;
const CARD_PADDING_Y = 120;
const QUOTE_FONT_MAX = 72;
const QUOTE_FONT_MIN = 36;
const QUOTE_LINE_HEIGHT = 1.32;
const SPACE_WIDTH_RATIO = 0.35;
const CHAR_WIDTH_RATIO = 0.6;
const WIDE_CHAR_BONUS_RATIO = 0.08;

const BASE_PATH = normalizeBasePath(process.env.BASE_PATH || '');
const SITE_ORIGIN = normalizeOrigin(process.env.SITE_ORIGIN || '');
const ENV_CARD_VERSION = normalizeCardVersion(process.env.CARD_VERSION || '');

marked.setOptions({ mangle: false, headerIds: false });

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const cardVersion = args.cardVersion ?? ENV_CARD_VERSION;

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
    const renderResult = resvg.render();
    const jpeg = encodeJpeg(
      {
        data: renderResult.pixels,
        width: renderResult.width,
        height: renderResult.height,
      },
      88
    );

    const cardPath = path.join(OUTPUT_CARD_DIR, `${quote.id}.jpg`);
    await fs.writeFile(cardPath, jpeg.data);

    const wrapperHtml = applyTemplate(wrapperTemplate, buildWrapperPayload(quote, cardVersion));
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
  let check = false;
  let cardVersion = null;

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (!arg) continue;

    if (arg === '--check' || arg === '--dry-run' || arg === '--validate') {
      check = true;
      continue;
    }

    if (arg.startsWith('--card-version=')) {
      const [, value] = arg.split('=', 2);
      cardVersion = normalizeCardVersion(value);
      continue;
    }

    if (arg === '--card-version' || arg === '--image-version') {
      const value = argv[i + 1];
      if (value && !value.startsWith('--')) {
        cardVersion = normalizeCardVersion(value);
        i += 1;
      } else {
        cardVersion = null;
      }
      continue;
    }

    if (arg.startsWith('--image-version=')) {
      const [, value] = arg.split('=', 2);
      cardVersion = normalizeCardVersion(value);
      continue;
    }
  }

  return { check, cardVersion };
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

function buildWrapperPayload(quote, cardVersion) {
  const sourceDomain = quote.sourceDomain || 'original-source';
  const articleTitle = quote.articleTitle || sourceDomain;
  const hasAuthor = Boolean(quote.name);

  let description;
  if (quote.articleTitle) {
    description = hasAuthor
      ? `From ${quote.articleTitle} by ${quote.name}`
      : `From ${quote.articleTitle} on ${sourceDomain}`;
  } else {
    description = hasAuthor
      ? `${quote.name} on ${sourceDomain}`
      : `Collected from ${sourceDomain}`;
  }

  const cardPath = `/cards/${quote.id}.jpg`;
  const versionSuffix = cardVersion ? `?v=${encodeURIComponent(cardVersion)}` : '';
  const ogImage = absoluteUrl(`${cardPath}${versionSuffix}`);

  return {
    page_title: escapeHtml(articleTitle),
    meta_description: escapeHtml(description),
    og_title: escapeHtml(articleTitle),
    og_description: escapeHtml(description),
    og_image: escapeHtml(ogImage),
    canonical_url: quote.url,
    source_url: quote.url,
    quote_text: escapeHtml(quote.quote),
    quote_author: hasAuthor ? escapeHtml(quote.name) : '',
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
    `    <span><a href="${escapeHtml(publicPath(`/cards/${quote.id}.jpg`))}">Download JPG</a></span>`
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

function escapeForSatori(value) {
  if (value === undefined || value === null) return '';
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
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

function normalizeCardVersion(input) {
  if (!input) return null;
  const trimmed = String(input).trim();
  return trimmed.length ? trimmed : null;
}

function calculateQuoteFontSize(text) {
  const sanitized = (text || '').replace(/\s+/g, ' ').trim();
  const availableWidth = CARD_WIDTH - CARD_PADDING_X * 2;
  const availableHeight = CARD_HEIGHT - CARD_PADDING_Y * 2;

  if (!sanitized) {
    return QUOTE_FONT_MAX;
  }

  for (let size = QUOTE_FONT_MAX; size >= QUOTE_FONT_MIN; size -= 2) {
    const lines = estimateLineCount(sanitized, size, availableWidth);
    const quoteHeight = lines * size * QUOTE_LINE_HEIGHT;

    if (quoteHeight <= availableHeight) {
      return size;
    }
  }

  return QUOTE_FONT_MIN;
}

function estimateLineCount(text, fontSize, maxWidth) {
  const words = text.split(' ');
  if (!words.length) return 1;

  const spaceWidth = fontSize * SPACE_WIDTH_RATIO;
  let lineWidth = 0;
  let lines = 1;

  for (const word of words) {
    if (!word) continue;
    const measuredWordWidth = Math.min(estimateWordWidth(word, fontSize), maxWidth);

    if (lineWidth === 0) {
      lineWidth = measuredWordWidth;
      continue;
    }

    if (lineWidth + spaceWidth + measuredWordWidth > maxWidth) {
      lines += 1;
      lineWidth = measuredWordWidth;
    } else {
      lineWidth += spaceWidth + measuredWordWidth;
    }
  }

  return lines;
}

function estimateWordWidth(word, fontSize) {
  const length = word.length;
  if (!length) return 0;

  const wideCharacters = (word.match(/[MW@#&$%]/g) || []).length;
  const narrowCharacters = (word.match(/[il1']/g) || []).length;
  const baseWidth = length * CHAR_WIDTH_RATIO;
  const widthAdjust = wideCharacters * WIDE_CHAR_BONUS_RATIO - narrowCharacters * 0.04;
  const estimated = Math.max(0.4, baseWidth + widthAdjust);
  return estimated * fontSize;
}

async function renderQuoteSvg(quote, fonts) {
  const quoteFontSize = calculateQuoteFontSize(quote.quote);

  const body = `
    <div style="display:flex;width:${CARD_WIDTH}px;height:${CARD_HEIGHT}px;background:#f7f4ec;color:#26211a;padding:${CARD_PADDING_Y}px ${CARD_PADDING_X}px;box-sizing:border-box;font-family:'Atkinson Hyperlegible';align-items:center;justify-content:center;">
      <div style="font-size:${quoteFontSize}px;line-height:${QUOTE_LINE_HEIGHT};font-weight:400;text-align:center;white-space:pre-wrap;word-break:break-word;max-width:100%;">“${escapeForSatori(
        quote.quote
      )}”</div>
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
