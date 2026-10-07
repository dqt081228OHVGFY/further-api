(function () {
  // oxlint-disable-next-line unicorn/prefer-module -- Also runs inline as a classic head script before paint.
  'use strict';
  if (typeof window === 'undefined' || typeof document === 'undefined') return;
  function initialize() {
    if (window.FurtherAppearance && window.FurtherAppearance.version === 2) {
      window.FurtherAppearance.sync();
      return window.FurtherAppearance;
    }
    const root = document.documentElement;
    const storageKey = 'further-api:theme:v2:palette';
    const legacyKey = 'further-api:theme:v1:further-palette';
    const holidays = new Set(['duanwu', 'qixi', 'mid_autumn', 'chongyang', 'new_year_eve', 'national_day']);
    const colours = new Set(['white', 'blue', 'yellow', ...holidays]);
    const holiday = function (value) { return holidays.has(value) ? value : ''; };
    let season = holiday(root.dataset.seasonHoliday) || holiday(root.dataset.holiday);
    let preference;
    let unsaved = false;
    function readStored() {
      try {
        const stored = JSON.parse(localStorage.getItem(storageKey) || 'null');
        if (stored && ['fixed', 'auto'].includes(stored.mode) && colours.has(stored.fixedPalette)) return { mode: stored.mode, fixedPalette: stored.fixedPalette };
      } catch {}
      return null;
    }
    function persist() {
      try { localStorage.setItem(storageKey, JSON.stringify(preference)); unsaved = false; }
      catch { unsaved = true; }
      try { localStorage.setItem(legacyKey, preference.mode === 'auto' ? 'auto' : preference.fixedPalette); }
      catch { /* Keep the selected colour in this document when writes fail. */ }
    }
    preference = readStored();
    if (!preference) {
      let previous = '';
      try { previous = localStorage.getItem(legacyKey) || ''; } catch {}
      let current = season || 'blue';
      if (colours.has(root.dataset.furtherPalette)) current = root.dataset.furtherPalette;
      if (colours.has(previous)) current = previous;
      preference = { mode: 'fixed', fixedPalette: current };
      persist();
    }
    const effective = function () { return preference.mode === 'auto' ? season || 'blue' : preference.fixedPalette; };
    const snapshot = function () {
      return { mode: preference.mode, fixedPalette: preference.fixedPalette, effectivePalette: effective(), palette: preference.mode === 'auto' ? 'auto' : preference.fixedPalette, season, holiday: holiday(effective()) };
    };
    function apply(notify) {
      const next = snapshot();
      let changed = false;
      const attribute = function (name, value) {
        if (value) {
          if (root.dataset[name] !== value) { root.dataset[name] = value; changed = true; }
        } else if (root.dataset[name] !== undefined) { delete root.dataset[name]; changed = true; }
      };
      attribute('furtherPaletteMode', next.mode);
      attribute('furtherPalette', next.palette);
      attribute('holiday', next.holiday);
      attribute('seasonHoliday', season);
      if (changed || notify) window.dispatchEvent(new CustomEvent('further-ui:palette', { detail: next }));
    }
    const controller = {
      version: 2,
      sync() { const stored = unsaved ? null : readStored(); if (stored) preference = stored; apply(false); },
      applySeason(value) { season = holiday(value); apply(false); },
      select(value) {
        if (value === 'auto') { controller.setMode('auto'); return; }
        if (!colours.has(value)) return;
        preference = { mode: 'fixed', fixedPalette: value };
        persist(); apply(true);
      },
      setMode(value) {
        if (!['fixed', 'auto'].includes(value)) return;
        preference = { mode: value, fixedPalette: value === 'fixed' ? effective() : preference.fixedPalette };
        persist(); apply(true);
      },
      current() { return preference.mode === 'auto' ? 'auto' : preference.fixedPalette; },
      mode() { return preference.mode; },
      season() { return season; },
      state: snapshot
    };
    window.FurtherAppearance = controller;
    // Reconcile stale bootstrap, server attributes and legacy callbacks before paint.
    const observer = new MutationObserver(function () { controller.sync(); });
    observer.observe(root, { attributes: true, attributeFilter: ['data-further-palette', 'data-further-palette-mode', 'data-holiday', 'data-season-holiday'] });
    window.addEventListener('storage', function (event) { if (event.key === storageKey) { unsaved = false; controller.sync(); } });
    apply(false);
    root.dataset.furtherMotion = 'on';
    root.dataset.reducedMotion = 'false';
    try {
      const stored = JSON.parse(localStorage.getItem('further_user_preferences') || '{}');
      const mode = localStorage.getItem('further-api:theme:v1:mode') || stored.theme || 'light';
      root.dataset.theme = mode === 'dark' || (mode === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches) ? 'dark' : 'light';
      localStorage.setItem('further-motion', 'on');
      if (stored.reducedMotion !== false) localStorage.setItem('further_user_preferences', JSON.stringify({ ...stored, reducedMotion: false }));
    } catch {}
    return controller;
  }
  // A single controller serves the inline head and the React entry.
  window.FurtherInitializeAppearance = initialize;
  initialize();
}());
