'use strict';

// Run with Node: node .agent-work/generate-relayfor-lunar-schedule.cjs [--check] [--refresh]
// Intl generates the candidate schedule. HKO conversion tables supply the
// canonical UTC+8 dates where ICU's astronomical calendar differs by a day.
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const path = require('node:path');
const vm = require('node:vm');

const root = path.resolve(__dirname, '..');
const cacheDirectory = path.join(__dirname, 'lunar-reference');
const minYear = 2025;
const maxYear = 2050;
const dayMilliseconds = 86400000;
const festivalDays = new Map([
  ['5/5', 'duanwu'],
  ['7/7', 'qixi'],
  ['8/15', 'mid_autumn'],
  ['9/9', 'chongyang'],
]);
const keys = [...festivalDays.values(), 'new_year_eve'];
const lunarFormatter = new Intl.DateTimeFormat('en-u-ca-chinese', {
  timeZone: 'Asia/Shanghai', year: 'numeric', month: 'numeric', day: 'numeric',
});
assert.equal(lunarFormatter.resolvedOptions().calendar, 'chinese');

function isoDate(timestamp) {
  return new Date(timestamp).toISOString().slice(0, 10);
}

function lunarDate(timestamp) {
  const parts = Object.fromEntries(lunarFormatter.formatToParts(new Date(timestamp)).map(part => [part.type, part.value]));
  return {month: Number.parseInt(parts.month, 10), day: Number(parts.day), leap: !/^\d+$/.test(parts.month)};
}

function checkYearDates(year, dates) {
  assert.deepEqual(Object.keys(dates).sort(), keys.slice().sort(), `${year} must contain exactly five festivals`);
  for (const key of keys) {
    assert.match(dates[key], new RegExp(`^${year}-\\d{2}-\\d{2}$`), `${year}/${key} must belong to its Gregorian year`);
    assert.equal(isoDate(Date.parse(dates[key] + 'T04:00:00Z')), dates[key]);
  }
  return Object.fromEntries(keys.map(key => [key, dates[key]]));
}

function intlDates(year) {
  const dates = {};
  // 04:00 UTC is noon in Asia/Shanghai: calendar reads never straddle midnight.
  for (let timestamp = Date.UTC(year, 0, 1, 4); timestamp < Date.UTC(year + 1, 0, 1, 4); timestamp += dayMilliseconds) {
    const current = lunarDate(timestamp);
    const next = lunarDate(timestamp + dayMilliseconds);
    const key = !current.leap && festivalDays.get(`${current.month}/${current.day}`);
    if (key) {
      assert.equal(dates[key], undefined, `duplicate non-leap ${key} in ${year}`);
      dates[key] = isoDate(timestamp);
    }
    // Eve is the actual last day before New Year, whether lunar day 29 or 30.
    if (!next.leap && next.month === 1 && next.day === 1) {
      assert.equal(current.month, 12);
      assert.ok(current.day === 29 || current.day === 30);
      assert.equal(dates.new_year_eve, undefined);
      dates.new_year_eve = isoDate(timestamp);
    }
  }
  return checkYearDates(year, dates);
}

function hkoDates(year, table) {
  const dates = {};
  let month = null;
  let leap = false;
  let previousMonth = null;
  let previousTimestamp = null;
  let dayCount = 0;
  for (const line of table.split(/\r?\n/)) {
    const match = /^(\d{4})\/(\d{1,2})\/(\d{1,2})\s+(\d+(?:st|nd|rd|th) Lunar Month|\d+)\s+/.exec(line);
    if (!match) continue;
    assert.equal(Number(match[1]), year);
    const timestamp = Date.UTC(year, Number(match[2]) - 1, Number(match[3]), 4);
    if (previousTimestamp !== null) assert.equal(timestamp - previousTimestamp, dayMilliseconds, `${year} HKO table must be contiguous`);
    const iso = isoDate(timestamp);
    let day;
    if (match[4].includes('Lunar Month')) {
      month = Number.parseInt(match[4], 10);
      // A repeated lunar month label is the intercalary month. HKO's English
      // plain-text table omits the word "leap", so retain transition state.
      leap = month === previousMonth;
      previousMonth = month;
      day = 1;
      if (month === 1 && !leap) dates.new_year_eve = isoDate(timestamp - dayMilliseconds);
    } else {
      day = Number(match[4]);
    }
    const key = !leap && festivalDays.get(`${month}/${day}`);
    if (key) {
      assert.equal(dates[key], undefined, `duplicate HKO ${key} in ${year}`);
      dates[key] = iso;
    }
    previousTimestamp = timestamp;
    dayCount++;
  }
  assert.equal(dayCount, (Date.UTC(year + 1, 0, 1) - Date.UTC(year, 0, 1)) / dayMilliseconds);
  return checkYearDates(year, dates);
}

async function readHkoTable(year) {
  const cachePath = path.join(cacheDirectory, `T${year}e.txt`);
  if (!process.argv.includes('--refresh')) {
    try { return await fs.readFile(cachePath, 'utf8'); }
    catch (error) { if (error.code !== 'ENOENT') throw error; }
  }
  const url = `https://www.hko.gov.hk/en/gts/time/calendar/text/files/T${year}e.txt`;
  const response = await fetch(url, {signal: AbortSignal.timeout(30000)});
  if (!response.ok) throw new Error(`HKO ${year}: HTTP ${response.status}`);
  const table = await response.text();
  hkoDates(year, table); // Never cache or use an incomplete/error response.
  await fs.mkdir(cacheDirectory, {recursive: true});
  await fs.writeFile(cachePath, table, 'utf8');
  return table;
}

async function main() {
  const tables = {};
  for (let first = minYear; first <= maxYear; first += 4) {
    await Promise.all(Array.from({length: Math.min(4, maxYear - first + 1)}, async (_, offset) => {
      const year = first + offset;
      tables[year] = await readHkoTable(year);
    }));
  }
  const dates = {};
  const corrections = [];
  for (let year = minYear; year <= maxYear; year++) {
    const generated = intlDates(year);
    const official = hkoDates(year, tables[year]);
    dates[year] = official;
    for (const key of keys) if (generated[key] !== official[key]) corrections.push({year, key, intl: generated[key], hko: official[key]});
  }
  assert.deepEqual(dates[2025], {duanwu: '2025-05-31', qixi: '2025-08-29', mid_autumn: '2025-10-06', chongyang: '2025-10-29', new_year_eve: '2025-01-28'});
  assert.deepEqual(dates[2026], {duanwu: '2026-06-19', qixi: '2026-08-19', mid_autumn: '2026-09-25', chongyang: '2026-10-18', new_year_eve: '2026-02-16'});

  const schedule = {
    minYear, maxYear, timeZone: 'Asia/Shanghai',
    source: 'Intl.DateTimeFormat Chinese calendar, non-leap lunar months; all dates cross-checked and corrected against Hong Kong Observatory UTC+8 Gregorian-Lunar Calendar Conversion Tables.',
    references: [
      'https://www.hko.gov.hk/en/gts/time/conversion.htm',
      'https://www.hko.gov.hk/en/gts/time/calendar/text/files/T{year}e.txt',
      'https://www.hko.gov.hk/en/gts/time/calendar/pdf/files/2025e.pdf',
      'https://www.hko.gov.hk/en/gts/time/calendar/pdf/files/2026e.pdf',
      'https://www.hko.gov.hk/en/Press-Releases/110730/Moon-watching-tips-for-Mid-Autumn-Festival-2026',
    ],
    dates,
  };
  const json = JSON.stringify(schedule, null, 2) + '\n';
  const javascript = '(function () {\n  \'use strict\';\n  // Generated with .agent-work/generate-relayfor-lunar-schedule.cjs.\n  const schedule = ' + JSON.stringify(schedule, null, 2) + ';\n  Object.keys(schedule.dates).forEach(function (year) { Object.freeze(schedule.dates[year]); });\n  Object.freeze(schedule.dates);\n  Object.freeze(schedule.references);\n  window.RelayForLunarSchedule = Object.freeze(schedule);\n})();\n';
  const outputs = [
    [path.join(root, 'model', 'relayfor_holiday_calendar.json'), json],
    [path.join(root, 'relayfor-ui', 'assets', 'lunar-schedule.js'), javascript],
  ];
  for (const [filename, content] of outputs) {
    if (process.argv.includes('--check')) assert.equal(await fs.readFile(filename, 'utf8'), content, filename + ' is stale');
    else await fs.writeFile(filename, content, 'utf8');
  }
  const context = {window: {}};
  vm.runInNewContext(javascript, context);
  assert.equal(JSON.stringify(context.window.RelayForLunarSchedule), JSON.stringify(schedule));
  assert.ok(Object.isFrozen(context.window.RelayForLunarSchedule.dates[2026]));
  await fs.writeFile(path.join(__dirname, 'lunar-reference', 'intl-hko-corrections.json'), JSON.stringify(corrections, null, 2) + '\n', 'utf8');
  console.log(`Verified ${maxYear - minYear + 1} years / ${keys.length * (maxYear - minYear + 1)} festival dates against HKO; ${corrections.length} Intl dates corrected.`);
  if (corrections.length) console.log(JSON.stringify(corrections, null, 2));
}

main().catch(error => { console.error(error); process.exitCode = 1; });
