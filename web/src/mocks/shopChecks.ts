import { at, edge } from './fixture';
import type { Diagnostic, Evidence, RawEdge, Revision, Status, WorkingTree } from './schemaTypes';

/*
 * What checking the shop's snapshot gives (docs/model.md §10), what runs have shown about it (M6), and the history of
 * the repository the compare button reads.
 */

/** The statuses that are not verified; every other declared item and depends edge is verified. */
export const STATUS_EXCEPTIONS: Record<string, Status> = {
  'order.OrderStatus': 'contradicted',
  'order.OrderRepository': 'stub',
  'order.OrderRepository.save': 'stub',
  'order.declined-keeps-stock': 'missing',
  'order.submit-order#6': 'missing',
  'depends inventory order': 'contradicted',
};

const STOCK_IMPORT = at('src/shop/inventory/stock.py', 'from shop.order.order import');
const CHARGE = at('src/shop/payment/payments.py', 'def charge');
const REFUNDED = at('src/shop/order/order.py', 'REFUNDED');
const DECLINED_KEEPS_STOCK = at('design/order.uc', 'property declined-keeps-stock');

export const DIAGNOSTICS: Diagnostic[] = [
  {
    code: 'check.undeclared-dependency',
    subject: 'depends inventory order',
    ...STOCK_IMPORT,
    message: 'inventory depends on order: src/shop/inventory/stock.py imports shop.order and Stock.release calls Order.total, but inventory does not use order',
    fix: 'remove the import of shop.order and the call to Order.total from inventory, or declare `uses order` in component inventory',
  },
  {
    code: 'check.unreferenced-event',
    subject: 'order.submit-order#6',
    ...CHARGE,
    message: 'step order.submit-order#6 emits payment.Paid, but Payments.charge, which realizes payment.Payments.charge, does not reference Paid, which realizes payment.Paid',
    fix: 'construct and publish Paid in Payments.charge',
  },
  {
    code: 'check.extra-value',
    subject: 'order.OrderStatus',
    ...REFUNDED,
    message: 'the enum OrderStatus, which realizes order.OrderStatus, has the value refunded, which the design does not declare',
    fix: 'declare `value refunded` in type OrderStatus, or remove REFUNDED from the code',
  },
  {
    code: 'check.missing-predicate',
    subject: 'order.declined-keeps-stock',
    ...DECLINED_KEEPS_STOCK,
    message: 'no function checks the property order.declined-keeps-stock',
    fix: 'write its predicate with @uc.checks("order.declined-keeps-stock"), taking service and cart',
  },
].map(({ code, subject, path, line, column, message, fix }) => ({ code, subject, path, line, column, message, fix }));

/** Full commit hashes, as git writes them; the short form is their first seven digits. */
const COMMITS = {
  head: 'a654861c0f3e9b27d4a8e15f60b2c97d3e41a0b8',
  bind: '9c41e07a5d2b8f36e1c04a97b3d5e28f6a1c7b40',
  counterexample: 'e4f5a6b2c8d1e7f30a9b4c6d2e8f1a3b5c7d9e0f',
  record: 'a1b2c3d4e5f60718293a4b5c6d7e8f9012345678',
  design: '1e6a0c94b7d3f25e8a1c6b09d4e7f3a2c5b8d160',
};

export const HEAD: WorkingTree = {
  commit: COMMITS.head,
  short: COMMITS.head.slice(0, 7),
  branch: 'main',
  subject: 'Add the status of an order',
  time: '2026-11-26T16:40:12Z',
  dirty: false,
};

/** The latest commits of main, newest first. */
export const REVISIONS: Revision[] = [
  { commit: HEAD.commit, short: HEAD.short, branch: 'main', subject: HEAD.subject, time: HEAD.time },
  { commit: COMMITS.bind, short: COMMITS.bind.slice(0, 7), subject: 'Bind OrderService.submit to its design', time: '2026-11-26T11:05:40Z' },
  { commit: COMMITS.counterexample, short: COMMITS.counterexample.slice(0, 7), subject: 'Fix the total of a line without a price', time: '2026-11-25T17:22:03Z' },
  { commit: COMMITS.record, short: COMMITS.record.slice(0, 7), subject: 'Record the counterexample r-77e01b2a', time: '2026-11-25T09:48:51Z' },
  { commit: COMMITS.design, short: COMMITS.design.slice(0, 7), subject: 'Design the shop', time: '2026-11-24T15:30:00Z' },
];

/** When `unicode build` wrote the snapshot. */
export const BUILT_AT = '2026-11-26T16:41:30Z';

/** The snapshot file, relative to the repository root. */
export const SNAPSHOT_PATH = '.unicode/graph.json';

const TOTAL = 'py:shop.order.order.Order.total';
const LINE_AMOUNT = 'py:shop.order.order._line_amount';
const SUBMIT = 'py:shop.order.service.OrderService.submit';
const FLOW_CASE = 'order.submit-order.r-3f9a2c1d';

/** M6: what runs have shown, by identity. */
export const EVIDENCE: Record<string, Evidence> = {
  'order.total-is-sum': {
    outcome: 'passed',
    adequacy: { code: 0.92 },
    drivers: [
      { mode: 'replay', tool: 'pytest', cases: 4 },
      { mode: 'explore', tool: '@given' },
      { mode: 'fuzz', tool: 'atheris' },
    ],
  },
  'order.total-is-sum.empty': { outcome: 'passed' },
  'order.total-is-sum.two-lines': { outcome: 'passed' },
  'order.total-is-sum.r-3f9a2c1d': {
    outcome: 'passed',
    coverageGain: 3,
    history: [{ commit: COMMITS.head, at: '2026-11-26T16:02:00Z', mode: 'replay', outcome: 'passed' }],
  },
  'order.total-is-sum.r-77e01b2a': {
    outcome: 'passed',
    coverageGain: 1,
    history: [
      { commit: COMMITS.head, at: '2026-11-26T16:02:00Z', mode: 'replay', outcome: 'passed' },
      { commit: COMMITS.counterexample, at: '2026-11-25T17:30:12Z', mode: 'replay', outcome: 'passed' },
      { commit: COMMITS.record, at: '2026-11-25T09:41:27Z', mode: 'explore', outcome: 'failed' },
    ],
  },
  'order.submit-order': {
    outcome: 'passed',
    adequacy: { code: 0.81, branch: 0.9, error: 0.67 },
    drivers: [
      { mode: 'replay', tool: 'pytest', cases: 1 },
      { mode: 'explore', tool: '@given' },
    ],
  },
  [FLOW_CASE]: {
    outcome: 'passed',
    coverageGain: 12,
    history: [{ commit: COMMITS.head, at: '2026-11-26T16:02:00Z', mode: 'replay', outcome: 'passed' }],
  },
  'order.submit-order#4': { branches: [{ label: 'Declined', cases: 3 }] },
  'order.submit-order#7': { branches: [{ label: 'Declined', cases: 3 }] },
  [SUBMIT]: { branches: [{ label: 'Declined', cases: 3 }] },
};

/** The lines of a function that a case ran, as `covers` writes them. */
function lines(from: number, to: number): string[] {
  return Array.from({ length: to - from + 1 }, (_, index) => String(from + index));
}

function covers(caseId: string, target: string, ran: string[] = []): RawEdge {
  return edge('covers', caseId, target, 'produced', { path: SNAPSHOT_PATH, line: 1, column: 1 }, ran.length > 0 ? { lines: ran } : {});
}

/** M6: what each case ran, functions with their lines and the flow's steps. */
export const COVERS: RawEdge[] = [
  covers('order.total-is-sum.empty', TOTAL, lines(22, 23)),
  covers('order.total-is-sum.two-lines', TOTAL, lines(22, 23)),
  covers('order.total-is-sum.two-lines', LINE_AMOUNT, lines(31, 32)),
  covers('order.total-is-sum.r-3f9a2c1d', TOTAL, lines(22, 23)),
  covers('order.total-is-sum.r-3f9a2c1d', LINE_AMOUNT, lines(31, 32)),
  covers('order.total-is-sum.r-77e01b2a', TOTAL, lines(22, 23)),
  covers('order.total-is-sum.r-77e01b2a', LINE_AMOUNT, lines(31, 32)),
  ...[1, 2, 3, 4, 5, 7, 8].map(index => covers(FLOW_CASE, `order.submit-order#${index}`)),
  covers(FLOW_CASE, SUBMIT, lines(40, 62)),
  covers(FLOW_CASE, 'py:shop.order.service._check_lines', lines(77, 80)),
  covers(FLOW_CASE, 'py:shop.inventory.stock.Stock.reserve', lines(18, 22)),
  covers(FLOW_CASE, 'py:shop.inventory.stock.Stock.release', lines(24, 28)),
  covers(FLOW_CASE, 'py:shop.payment.payments.Payments.charge', lines(31, 35)),
];

/*
 * The base revisions the compare button can pick: every commit before the head. Each differs from the served snapshot
 * by the change the head made: order.Order gained its status, and the function realizing OrderService.submit changed
 * three lines of its body.
 */

/** The attribute a change added, by node: the base snapshot's node lacks these values. */
export const ADDED_ATTRS: Record<string, Record<string, string[]>> = {
  'order.Order': { fields: ['status: order.OrderStatus'] },
  'py:shop.order.order.Order': { fields: ['status: order.OrderStatus'] },
};

/** How many lines of a code element's source the change touched. */
export const LINES_CHANGED: Record<string, number> = {
  'py:shop.order.order.Order': 2,
  [SUBMIT]: 3,
};

/** The edges the change added. */
export const ADDED_EDGES: { kind: RawEdge['kind']; from: string; to: string }[] = [
  { kind: 'refers', from: 'order.Order', to: 'order.OrderStatus' },
  { kind: 'refers', from: 'py:shop.order.order.Order', to: 'py:shop.order.order.OrderStatus' },
];
