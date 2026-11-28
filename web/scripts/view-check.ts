/*
 * The viewer's acceptance on this repository (CLAUDE.md principle 12; `make view-check`). It builds this repository's
 * snapshot, serves it with unicode serve and the built pages, and opens in a private headless Chromium every view kind
 * this repository can open, each with several inputs chosen deterministically, and the entity page of each kind of node,
 * each page at a desktop's width and at 768 px. An input is skipped only when the interface answers that the view does
 * not apply to it (serve.not-applicable). A load that the browser's network cuts off is the sweep's own fault and is
 * tried again. Each page must pass these rules:
 *
 * - renders: the page shows no problem note, logs no console error, throws nothing uncaught, gets no answer of 400 or
 *   more, loads, and a view on a canvas draws its canvas;
 * - opens-on-content: when a view opens, at least one node's box of its canvas lies in the canvas, or, for a view
 *   without a canvas, at least one node of the page shows;
 * - reachable: every control in the window, off the canvas, is the element at the middle of its visible part; every
 *   control on the canvas is, once the canvas is panned to put that middle at the canvas's middle, and shows some part
 *   of itself; and every line that previews its edge has a point that no box drawn above the lines covers and that lies
 *   further than half a click target from every line drawn after it;
 * - marks-clear: no label or badge of a line overlaps a box of the drawing or another label or badge by more than 4 px²;
 * - lines-apart: no two lines of a drawing have axis-aligned stretches on one line overlapping for more than 2 px;
 * - not-nested: no control lies inside another;
 * - linked: every shown text that names a node (its title, its identity, or a design node's identity without its
 *   component) is inside a control, unless it names the node the page is about outside the drawing or names a field or
 *   a value in its row's first cell; and no control holds the names of two nodes.
 *
 * A failure names the page, the rule and the element; the script exits 1 when a page fails any rule. `--serve URL`
 * sweeps a unicode serve already running instead, and `--only PATTERN` opens only the pages whose name matches.
 */
import { execFileSync, spawn } from 'node:child_process';
import { existsSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { parseArgs } from 'node:util';
import { chromium, type BrowserContext, type Page } from 'playwright';

const ROOT = resolve(import.meta.dirname, '../..');
const PAGES = join(ROOT, 'internal/serve/ui/dist');
/** The widths every page is checked at: a desktop's, and 768 px, where the side columns become drawers. */
const VIEWPORTS = [{ width: 1440, height: 900 }, { width: 768, height: 1024 }];

/** How many pages are open at once, and how long one may take to load. */
const WORKERS = 4;
const LOAD_TIMEOUT = 60_000;
const LOAD_ATTEMPTS = 3;

/** The error Chromium gives the requests it cuts off when the host's network changes. */
const NETWORK_CHANGED = 'net::ERR_NETWORK_CHANGED';

/** How many inputs the views with many inputs open, taken evenly from all of them in identity order. */
const CALLS_SAMPLE = 24;
const CODE_SAMPLE = 16;

/** A node as the interface gives it; the sweep reads only these fields. */
interface ApiNode {
  id: string;
  kind: string;
  form: string;
  layer: string;
  title: string;
  salient: boolean;
}

/** A page to open: its name in the report, its address, whether it shows a view or an entity, and whether it draws on a canvas. */
interface Visit {
  name: string;
  path: string;
  view: boolean;
  canvas: boolean;
}

/** A rule a page breaks, and the element that breaks it. */
interface Finding {
  rule: string;
  element: string;
}

/** What the page under inspection needs from the sweep: what it shows, the node names it may show, and its own nodes. */
interface Context {
  view: boolean;
  canvas: boolean;
  names: [string, string][];
  self: string[];
}

/** `count` items spread evenly over the list, in its order. */
function spread<T>(items: T[], count: number): T[] {
  if (items.length <= count) {
    return items;
  }
  return Array.from({ length: count }, (_, index) => items[Math.floor((index * items.length) / count)]).filter((item): item is T => item !== undefined);
}

function query(path: string, input: string): string {
  return `${path}?${new URLSearchParams({ input })}`;
}

/** The pages the sweep opens, from what the snapshot holds. */
function visits(nodes: ApiNode[]): Visit[] {
  const byId = (a: ApiNode, b: ApiNode) => (a.id < b.id ? -1 : 1);
  const salient = nodes.filter(node => node.salient).sort(byId);
  const components = salient.filter(node => node.kind === 'container' && node.layer === 'design' && node.form === 'component');
  const flows = salient.filter(node => node.kind === 'flow');
  const operations = salient.filter(node => node.kind === 'op');
  const files = salient.filter(node => node.kind === 'container' && node.layer === 'code' && node.form === 'file');
  // The property view is a list of properties; every other view is drawn on a canvas.
  const view = (kind: string, node: ApiNode): Visit => ({ name: `${kind} ${node.id}`, path: query(`/view/${kind}`, node.id), view: true, canvas: kind !== 'property' });
  const kinds = Map.groupBy(salient, node => `${node.kind} ${node.layer} ${node.form}`);
  return [
    { name: 'architecture', path: '/view/architecture', view: true, canvas: true },
    { name: 'events', path: '/view/events', view: true, canvas: true },
    ...components.map(node => view('events', node)),
    ...components.map(node => view('class', node)),
    ...components.map(node => ({ ...view('class', node), name: `class ${node.id} code`, path: `${query('/view/class', node.id)}&layer=code` })),
    ...components.map(node => view('property', node)),
    ...flows.map(node => view('sequence', node)),
    ...flows.map(node => view('activity', node)),
    ...spread(operations, CALLS_SAMPLE).map(node => view('calls', node)),
    ...spread(files, CODE_SAMPLE).map(node => view('code', node)),
    ...[...kinds.values()].flatMap(([node]) => (node ? [{ name: `entity ${node.kind} ${node.layer} ${node.form} ${node.id}`, path: `/entity?${new URLSearchParams({ id: node.id })}`, view: false, canvas: false }] : [])),
  ];
}

/** The node-shaped objects in an answer of the interface. */
function nodesIn(value: unknown, found: ApiNode[] = []): ApiNode[] {
  if (Array.isArray(value)) {
    value.forEach(item => nodesIn(item, found));
  } else if (value !== null && typeof value === 'object') {
    const item = value as Record<string, unknown>;
    if (typeof item.id === 'string' && typeof item.title === 'string' && typeof item.kind === 'string' && typeof item.layer === 'string') {
      found.push(item as unknown as ApiNode);
    }
    Object.values(item).forEach(child => nodesIn(child, found));
  }
  return found;
}

/**
 * The names by which a page may show a node: its title, its identity, and a design node's identity without its
 * component, as a card names a type of its own component.
 */
function namesOf(nodes: ApiNode[]): [string, string][] {
  return nodes.flatMap(node => {
    const dot = node.id.indexOf('.');
    const relative: [string, string][] = node.layer === 'design' && dot > 0 && !node.id.includes('#') ? [[node.id.slice(dot + 1), node.id]] : [];
    return [[node.title, node.id], [node.id, node.id], ...relative];
  });
}

/**
 * The checks that run in the page. It runs in the browser, so it uses nothing from outside its own body. The canvas
 * is panned by setting its sheet's transform directly, target by target, and put back at the end.
 */
function inspect({ view, canvas, names: pairs, self }: Context): Finding[] {
  const INTERACTIVE = 'a[href], button, [role="button"], [tabindex="0"]';
  const findings: Finding[] = [];
  const describe = (element: Element) => (element.getAttribute('aria-label') ?? element.textContent ?? '').trim().replace(/\s+/g, ' ').slice(0, 60) || element.tagName.toLowerCase();
  const names = new Map(pairs);

  for (const alert of document.querySelectorAll('[role="alert"]')) {
    findings.push({ rule: 'renders', element: `problem note "${describe(alert)}"` });
  }

  const targets = [...document.querySelectorAll(INTERACTIVE)];
  for (const target of targets) {
    const outer = target.parentElement?.closest(INTERACTIVE);
    if (outer) {
      findings.push({ rule: 'not-nested', element: `"${describe(target)}" inside "${describe(outer)}"` });
    }
  }

  // A control's visible part: its box cut by every box that clips it, up to `until`; none when nothing of it shows. A
  // control shows a part cut short, such as a reference at the end of a field's type, and is reached there.
  const drawing = document.querySelector<HTMLElement>('[role="group"][class*="_drawing_"]');
  const visiblePart = (element: Element, until: Element | null) => {
    if (!element.checkVisibility({ visibilityProperty: true })) {
      return undefined;
    }
    let { left, top, right, bottom } = element.getBoundingClientRect();
    for (let parent = element.parentElement; parent && parent !== until; parent = parent.parentElement) {
      const style = getComputedStyle(parent);
      if (/(auto|scroll|hidden|clip)/.test(style.overflow + style.overflowX + style.overflowY)) {
        const box = parent.getBoundingClientRect();
        left = Math.max(left, box.left);
        top = Math.max(top, box.top);
        right = Math.min(right, box.right);
        bottom = Math.min(bottom, box.bottom);
      }
    }
    return right - left >= 1 && bottom - top >= 1 ? { left, top, right, bottom } : undefined;
  };
  const middleOf = (part: { left: number; top: number; right: number; bottom: number }) => ({ x: (part.left + part.right) / 2, y: (part.top + part.bottom) / 2 });
  const reached = (target: Element, x: number, y: number) => {
    const hit = document.elementFromPoint(x, y);
    if (!(hit && (hit === target || target.contains(hit)))) {
      findings.push({ rule: 'reachable', element: `"${describe(target)}" under "${hit ? describe(hit) : 'nothing'}"` });
    }
  };

  // Controls off the canvas: each in sight is the element at the middle of its visible part.
  for (const target of targets.filter(item => !drawing?.contains(item))) {
    const part = visiblePart(target, null);
    const centre = part && middleOf(part);
    if (centre && centre.x >= 0 && centre.y >= 0 && centre.x < innerWidth && centre.y < innerHeight) {
      reached(target, centre.x, centre.y);
    }
  }

  // A view without a canvas opens on its nodes.
  if (view && !canvas) {
    const main = document.querySelector('main');
    const nodes = [...(main?.querySelectorAll('[data-node]') ?? [])].filter(element => visiblePart(element, null) !== undefined);
    if (nodes.length === 0) {
      findings.push({ rule: 'opens-on-content', element: 'no node in sight on the page' });
    }
  }

  if (canvas) {
    const viewport = drawing?.closest<HTMLElement>('[class*="_viewport_"]');
    const sheet = drawing?.closest<HTMLElement>('[class*="_sheet_"]');
    if (!drawing || !viewport || !sheet) {
      findings.push({ rule: 'renders', element: 'no canvas' });
      return findings;
    }
    const room = viewport.getBoundingClientRect();
    const boxes = [...drawing.querySelectorAll('[data-node]')];
    const shown = boxes.some(box => {
      const rect = box.getBoundingClientRect();
      return rect.width > 0 && rect.right > room.left && rect.left < room.right && rect.bottom > room.top && rect.top < room.bottom;
    });
    if (!shown) {
      findings.push({ rule: 'opens-on-content', element: `none of the canvas's ${boxes.length} boxes` });
    }

    // Controls on the canvas: the canvas is panned at full size to put the middle of each one's visible part at the
    // middle of its viewport. A control that shows nothing of itself cannot be reached.
    const opened = sheet.style.transform;
    const frame = sheet.getBoundingClientRect();
    const scale = frame.width / sheet.offsetWidth;
    const middle = { x: room.left + room.width / 2, y: room.top + room.height / 2 };
    const centres = targets.filter(item => drawing.contains(item) && item instanceof HTMLElement).map(target => {
      const part = visiblePart(target, viewport);
      const centre = part && middleOf(part);
      return { target, at: centre && { x: (centre.x - frame.left) / scale, y: (centre.y - frame.top) / scale } };
    });
    for (const { target, at } of centres) {
      if (!at) {
        findings.push({ rule: 'reachable', element: `"${describe(target)}" shows nothing of itself` });
        continue;
      }
      sheet.style.transform = `translate(${room.width / 2 - at.x}px, ${room.height / 2 - at.y}px)`;
      reached(target, middle.x, middle.y);
    }
    sheet.style.transform = opened;

    // Lines, in the drawing's own units: the stretches each runs, and the boxes drawn above the lines.
    const toDrawing = (rect: DOMRect) => {
      const origin = drawing.getBoundingClientRect();
      const factor = origin.width / drawing.offsetWidth;
      return { left: (rect.left - origin.left) / factor, top: (rect.top - origin.top) / factor, right: (rect.right - origin.left) / factor, bottom: (rect.bottom - origin.top) / factor };
    };
    const children = [...drawing.children];
    const linesAt = children.findIndex(child => child.tagName.toLowerCase() === 'svg');
    const covers = children.slice(linesAt + 1).filter(child => child.tagName.toLowerCase() !== 'svg' && !child.className.toString().includes('_marker_')).map(child => toDrawing(child.getBoundingClientRect()));
    const pointsOf = (path: Element) => {
      const tokens = (path.getAttribute('d') ?? '').trim().split(/\s+/);
      const points: { x: number; y: number }[] = [];
      let x = 0;
      let y = 0;
      for (let index = 0; index < tokens.length; index++) {
        const command = tokens[index];
        if (command === 'M' || command === 'L') {
          x = Number(tokens[++index]);
          y = Number(tokens[++index]);
        } else if (command === 'H') {
          x = Number(tokens[++index]);
        } else if (command === 'V') {
          y = Number(tokens[++index]);
        }
        points.push({ x, y });
      }
      return points;
    };
    const near = (point: { x: number; y: number }, points: { x: number; y: number }[], reach: number) => points.slice(1).some((b, index) => {
      const a = points[index] ?? b;
      const dx = b.x - a.x;
      const dy = b.y - a.y;
      const t = dx === 0 && dy === 0 ? 0 : Math.max(0, Math.min(1, ((point.x - a.x) * dx + (point.y - a.y) * dy) / (dx * dx + dy * dy)));
      return Math.hypot(point.x - (a.x + t * dx), point.y - (a.y + t * dy)) < reach;
    });
    const hits = [...drawing.querySelectorAll('svg path[data-hit]')].map(path => ({ label: path.getAttribute('aria-label') ?? '', points: pointsOf(path), width: parseFloat(getComputedStyle(path).strokeWidth) }));
    hits.forEach((line, index) => {
      const above = hits.slice(index + 1);
      const free = line.points.slice(1).some((b, at) => {
        const a = line.points[at] ?? b;
        const length = Math.hypot(b.x - a.x, b.y - a.y);
        for (let step = 0; step <= length; step += 2) {
          const point = { x: a.x + ((b.x - a.x) * step) / (length || 1), y: a.y + ((b.y - a.y) * step) / (length || 1) };
          const under = covers.some(box => point.x > box.left && point.x < box.right && point.y > box.top && point.y < box.bottom);
          if (!under && !above.some(other => near(point, other.points, other.width / 2))) {
            return true;
          }
        }
        return false;
      });
      if (!free) {
        findings.push({ rule: 'reachable', element: `line "${line.label}"` });
      }
    });

    // Two lines run along the same stretch where axis-aligned segments of both lie on one line for more than a hair.
    const segmentsOf = (path: Element) => {
      const points = pointsOf(path);
      return points.slice(1).flatMap((b, index) => {
        const a = points[index] ?? b;
        if (a.x === b.x) {
          return [{ across: false, at: a.x, from: Math.min(a.y, b.y), to: Math.max(a.y, b.y) }];
        }
        return a.y === b.y ? [{ across: true, at: a.y, from: Math.min(a.x, b.x), to: Math.max(a.x, b.x) }] : [];
      });
    };
    const drawn = [...drawing.querySelectorAll('svg path[aria-hidden="true"]')].filter(path => path.closest('defs') === null);
    const lines = drawn.map(path => ({ path, segments: segmentsOf(path) }));
    const labelOf = (path: Element) => path.parentElement?.getAttribute('data-line') ?? path.getAttribute('d')?.slice(0, 40) ?? '';
    lines.forEach((a, index) => {
      for (const b of lines.slice(index + 1)) {
        const shared = a.segments.reduce((sum, s) => sum + b.segments.reduce((part, t) => (s.across === t.across && Math.abs(s.at - t.at) < 1.5 ? part + Math.max(0, Math.min(s.to, t.to) - Math.max(s.from, t.from)) : part), 0), 0);
        if (shared > 2) {
          findings.push({ rule: 'lines-apart', element: `line "${labelOf(a.path)}" and line "${labelOf(b.path)}" share ${Math.round(shared)} px` });
        }
      }
    });

    // Labels and badges of lines keep off the boxes and off each other.
    const meet = (a: DOMRect, b: DOMRect) => Math.max(0, Math.min(a.right, b.right) - Math.max(a.left, b.left)) * Math.max(0, Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top));
    const markers = [...drawing.querySelectorAll('[class*="_marker_"]')].map(marker => {
      const parts = [...marker.children].map(child => child.getBoundingClientRect()).filter(rect => rect.width > 0);
      return { marker, box: new DOMRect(Math.min(...parts.map(rect => rect.left)), Math.min(...parts.map(rect => rect.top)), Math.max(...parts.map(rect => rect.right)) - Math.min(...parts.map(rect => rect.left)), Math.max(...parts.map(rect => rect.bottom)) - Math.min(...parts.map(rect => rect.top))) };
    }).filter(item => Number.isFinite(item.box.width));
    const solid = [...drawing.querySelectorAll('[data-node], [class*="_card_"], [class*="_fileBox_"], [class*="_fileHead_"], [class*="_chip_"]')]
      .filter(element => !element.closest('[class*="_marker_"]') && !element.matches('[class*="_boundary_"]'));
    markers.forEach((a, index) => {
      for (const box of solid) {
        if (meet(a.box, box.getBoundingClientRect()) > 4) {
          findings.push({ rule: 'marks-clear', element: `mark "${describe(a.marker)}" over "${describe(box)}"` });
        }
      }
      for (const b of markers.slice(index + 1)) {
        if (meet(a.box, b.box) > 4) {
          findings.push({ rule: 'marks-clear', element: `mark "${describe(a.marker)}" over mark "${describe(b.marker)}"` });
        }
      }
    });
  }

  // Texts that name a node: each inside a control, unless it names the node the page is about outside the drawing, or
  // names a field or a value of a row (the row's first cell); and no control holds the names of two nodes.
  const selfIds = new Set(self);
  const ignored = (parent: Element, value: string) => {
    const row = parent.parentElement;
    const fieldName = row !== null && row.firstElementChild === parent && (/_field_/.test(row.className.toString()) || parent.getAttribute('role') === 'cell');
    const own = selfIds.has(names.get(value) ?? '') && !(drawing?.contains(parent) ?? false);
    return fieldName || own || parent.closest('h1, [role="tooltip"], [aria-current="page"], [aria-hidden="true"]') !== null;
  };
  const byTarget = new Map<Element, Set<string>>();
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  for (let text = walker.nextNode(); text; text = walker.nextNode()) {
    const value = (text.textContent ?? '').trim();
    const parent = text.parentElement;
    const id = names.get(value);
    if (id === undefined || !parent || parent.getClientRects().length === 0) {
      continue;
    }
    const target = parent.closest(INTERACTIVE);
    if (target) {
      byTarget.set(target, new Set([...(byTarget.get(target) ?? []), id]));
    } else if (!ignored(parent, value)) {
      findings.push({ rule: 'linked', element: `"${value}" is no link` });
    }
  }
  for (const [target, ids] of byTarget) {
    if (ids.size > 1) {
      findings.push({ rule: 'linked', element: `"${describe(target)}" holds the names of ${[...ids].join(', ')}` });
    }
  }
  return findings;
}

/**
 * Opens a visit in a page of its own and checks it; undefined when the view does not apply to the visit's input. A load
 * the browser's network fails (a `net::` error of the navigation, or a request of the page cut off because the host's
 * network changed) is the sweep's own fault and is tried again, up to LOAD_ATTEMPTS times; any other failure counts
 * under renders.
 */
async function check(context: BrowserContext, base: string, tree: ApiNode[], visit: Visit): Promise<Finding[] | undefined> {
  const response = await fetch(`${base}/api${visit.path.replace(/^\/view\//, '/views/')}`);
  const answer = await response.json() as unknown;
  if (!response.ok) {
    const code = (answer as { code?: string }).code;
    return visit.view && code === 'serve.not-applicable' ? undefined : [{ rule: 'renders', element: `the interface refused the page's data: ${code ?? response.status}` }];
  }
  for (let attempt = 1; ; attempt++) {
    const page = await context.newPage();
    try {
      return await checkPage(page, base, namesOf([...tree, ...nodesIn(answer)]), visit, answer);
    } catch (error) {
      if (!(error instanceof Error && error.message.includes('net::')) || attempt === LOAD_ATTEMPTS) {
        return [{ rule: 'renders', element: `the page did not load: ${error instanceof Error ? error.message.split('\n')[0] : String(error)}` }];
      }
    } finally {
      await page.close();
    }
  }
}

/** Checks a page whose interface answer is `answer`; the problems the browser reports while it loads count under renders. */
async function checkPage(page: Page, base: string, names: [string, string][], visit: Visit, answer: unknown): Promise<Finding[]> {
  const findings: Finding[] = [];
  const onConsole = (message: { type: () => string; text: () => string }) => {
    if (message.type() === 'error') {
      findings.push({ rule: 'renders', element: `console error "${message.text().slice(0, 160)}"` });
    }
  };
  const onError = (error: Error) => findings.push({ rule: 'renders', element: `uncaught "${error.message.slice(0, 160)}"` });
  const onResponse = (response: { status: () => number; url: () => string }) => {
    if (response.status() >= 400) {
      findings.push({ rule: 'renders', element: `${response.url().replace(base, '')} answered ${response.status()}` });
    }
  };
  // Chromium cuts off the requests in flight when the host's network changes, as other work on the machine starts or
  // stops containers; the page then shows that cut, not the viewer.
  let networkChanged = false;
  const onRequestFailed = (request: { failure: () => { errorText: string } | null }) => {
    networkChanged ||= request.failure()?.errorText === NETWORK_CHANGED;
  };
  page.on('console', onConsole);
  page.on('pageerror', onError);
  page.on('response', onResponse);
  page.on('requestfailed', onRequestFailed);
  try {
    await page.goto(base + visit.path, { timeout: LOAD_TIMEOUT });
    // A view's page is drawn once its drawing (or, without a canvas, its first node) is there, or a problem note is.
    if (visit.view) {
      await page.waitForSelector(`${visit.canvas ? '[role="group"][class*="_drawing_"] svg' : 'main [data-node]'}, [role="alert"]`, { timeout: LOAD_TIMEOUT });
    }
    await page.waitForLoadState('networkidle', { timeout: LOAD_TIMEOUT });
    await page.waitForFunction(() => document.querySelector('[role="status"]') === null, undefined, { timeout: LOAD_TIMEOUT });
    // The labels of a drawing's lines are measured, then placed: two frames after the last answer.
    await page.evaluate(() => new Promise(done => requestAnimationFrame(() => requestAnimationFrame(done))));
    if (networkChanged) {
      throw new Error(`a request of the page failed with ${NETWORK_CHANGED}`);
    }
    const own = visit.view ? (answer as { input: { id: string } }).input.id : (answer as { node: { id: string } }).node.id;
    findings.push(...await page.evaluate(inspect, { view: visit.view, canvas: visit.canvas, names, self: [own] }));
  } finally {
    page.off('console', onConsole);
    page.off('pageerror', onError);
    page.off('response', onResponse);
    page.off('requestfailed', onRequestFailed);
  }
  return findings;
}

/** Builds this repository's snapshot and serves it with the built pages; the server's address and how to stop it. */
async function serve(directory: string): Promise<{ base: string; stop: () => void }> {
  const binary = join(directory, 'unicode');
  execFileSync('go', ['build', '-o', binary, './cmd/unicode'], { cwd: ROOT, stdio: 'inherit' });
  execFileSync(binary, ['build', '--out', directory], { cwd: ROOT, stdio: ['ignore', 'ignore', 'inherit'] });
  const server = spawn(binary, ['serve', '--snapshot', join(directory, 'graph.json'), '--web', PAGES, '--addr', '127.0.0.1:0'], { cwd: ROOT, stdio: ['ignore', 'pipe', 'pipe'] });
  let log = '';
  server.stderr.on('data', (chunk: Buffer) => {
    log = (log + chunk.toString()).slice(-4000);
  });
  const base = await new Promise<string>((done, fail) => {
    let out = '';
    server.stdout.on('data', (chunk: Buffer) => {
      out += chunk.toString();
      const line = out.split('\n')[0];
      if (out.includes('\n') && line) {
        done(line.trim().replace(/\/$/, ''));
      }
    });
    server.on('exit', code => fail(new Error(`unicode serve exited with ${code} before it gave its address:\n${log}`)));
  });
  return { base, stop: () => server.kill() };
}

async function main(): Promise<number> {
  const { values } = parseArgs({ options: { serve: { type: 'string' }, only: { type: 'string' } } });
  const started = Date.now();
  const directory = mkdtempSync(join(tmpdir(), 'unicode-view-check-'));
  if (values.serve === undefined && !existsSync(join(PAGES, 'index.html'))) {
    console.error(`view-check: ${PAGES} holds no built pages; build them first with pnpm --dir web build (make web-check does)`);
    return 2;
  }
  const server = values.serve === undefined
    ? await serve(directory).catch((error: unknown) => {
      rmSync(directory, { recursive: true, force: true });
      throw error;
    })
    : { base: values.serve.replace(/\/$/, ''), stop: () => undefined };
  const browser = await chromium.launch();
  try {
    const nodes = await (await fetch(`${server.base}/api/search?text=&limit=1000000`)).json() as ApiNode[];
    const tree = nodesIn(await (await fetch(`${server.base}/api/tree`)).json() as unknown);
    const only = values.only === undefined ? undefined : new RegExp(values.only);
    const contexts = await Promise.all(VIEWPORTS.map(viewport => browser.newContext({ viewport })));
    const queue = visits(nodes).filter(visit => only === undefined || only.test(visit.name))
      .flatMap(visit => contexts.map((context, index) => ({ visit, context, width: VIEWPORTS[index]?.width ?? 0 })));
    let total = 0;
    const failures: string[] = [];
    await Promise.all(Array.from({ length: WORKERS }, async () => {
      for (let item = queue.shift(); item !== undefined; item = queue.shift()) {
        const findings = await check(item.context, server.base, tree, item.visit);
        total += findings === undefined ? 0 : 1;
        for (const finding of findings ?? []) {
          failures.push(`${item.visit.name} at ${item.width} px (${item.visit.path}): ${finding.rule}: ${finding.element}`);
        }
      }
    }));
    const seconds = ((Date.now() - started) / 1000).toFixed(0);
    if (failures.length > 0) {
      console.error([...new Set(failures)].sort().join('\n'));
      console.error(`view-check: ${failures.length} failures on ${total} pages in ${seconds} s`);
      return 1;
    }
    console.log(`view-check: ${total} pages pass in ${seconds} s`);
    return 0;
  } finally {
    await browser.close();
    server.stop();
    rmSync(directory, { recursive: true, force: true });
  }
}

process.exitCode = await main();
