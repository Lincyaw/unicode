import { at, codeNode, edge, site } from './fixture';
import { DESIGN_EDGES, DESIGN_NODES } from './shopDesign';
import type { NodeKind, RawEdge, RawNode } from './schemaTypes';

/*
 * The code layer of the shop as the Python backend extracts it (docs/design.md §4, §5.2), and what derivation adds
 * (docs/model.md §9): realizes of methods (D2), owns (D1), depends (D3), traces (D4), exercises (D5) and reaches (D7).
 * Names are written back in the design's spelling (on_paid stays the method's name; its fields read unitPrice).
 */

export const CODE_NODES: RawNode[] = [];
export const CODE_EDGES: RawEdge[] = [];

const ORDER_PY = 'src/shop/order/order.py';
const SERVICE_PY = 'src/shop/order/service.py';
const REPOSITORY_PY = 'src/shop/order/repository.py';
const TEST_ORDER_PY = 'src/shop/order/test_order.py';
const API_PY = 'src/shop/order/api.py';
const STOCK_PY = 'src/shop/inventory/stock.py';
const TEST_STOCK_PY = 'src/shop/inventory/test_stock.py';
const PAYMENTS_PY = 'src/shop/payment/payments.py';
const EVENTS_PY = 'src/shop/payment/events.py';
const GATEWAY_PY = 'src/shop/payment/gateway.py';
const LOG_PY = 'src/shop/logging/log.py';

/** The packages, each with its files and the design container whose paths own them. */
const PACKAGES: { id: string; owner: string; files: string[] }[] = [
  { id: 'py:shop.order', owner: 'order', files: [ORDER_PY, SERVICE_PY, REPOSITORY_PY, TEST_ORDER_PY, API_PY] },
  { id: 'py:shop.inventory', owner: 'inventory', files: [STOCK_PY, TEST_STOCK_PY] },
  { id: 'py:shop.payment', owner: 'payment', files: [PAYMENTS_PY, EVENTS_PY, GATEWAY_PY] },
  { id: 'py:shop.logging', owner: 'logging', files: [LOG_PY] },
];

export function fileId(path: string): string {
  return `file:${path}`;
}

/** The salient code elements, whose files and packages are salient in turn (D6). */
const SALIENT = new Set<string>();

interface ElementSpec {
  id: string;
  kind: NodeKind;
  form: string;
  title: string;
  file: string;
  needle: string;
  parent?: string;
  attrs?: Record<string, string[]>;
  /** The binding written on it, or derived for a method: realizes or checks, and the design identity */
  binding?: { verb: 'realizes' | 'checks'; target: string };
}

const ELEMENTS: ElementSpec[] = [
  // order.py
  { id: 'py:shop.order.order.Order', kind: 'type', form: 'class', title: 'Order', file: ORDER_PY, needle: 'class Order:', attrs: { name: ['Order'], fields: ['id: string', 'lines: list<order.OrderLine>', 'status: order.OrderStatus'] }, binding: { verb: 'realizes', target: 'order.Order' } },
  { id: 'py:shop.order.order.Order.mark_paid', kind: 'op', form: 'method', title: 'Order.mark_paid', file: ORDER_PY, needle: 'def mark_paid', parent: 'py:shop.order.order.Order', attrs: { name: ['markPaid'], signature: ['()'], testKind: ['none'], stub: ['false'] } },
  { id: 'py:shop.order.order.Order.total', kind: 'op', form: 'method', title: 'Order.total', file: ORDER_PY, needle: 'def total', parent: 'py:shop.order.order.Order', attrs: { name: ['total'], signature: ['() -> int'], testKind: ['none'], stub: ['false'] }, binding: { verb: 'realizes', target: 'order.Order.total' } },
  { id: 'py:shop.order.order.EmptyCart', kind: 'type', form: 'class', title: 'EmptyCart', file: ORDER_PY, needle: 'class EmptyCart', attrs: { name: ['EmptyCart'] }, binding: { verb: 'realizes', target: 'order.EmptyCart' } },
  { id: 'py:shop.order.order._line_amount', kind: 'op', form: 'func', title: '_line_amount', file: ORDER_PY, needle: 'def _line_amount', attrs: { name: ['_lineAmount'], signature: ['(line: order.OrderLine) -> int'], testKind: ['none'], stub: ['false'] } },
  { id: 'py:shop.order.order.OrderLine', kind: 'type', form: 'class', title: 'OrderLine', file: ORDER_PY, needle: 'class OrderLine', attrs: { name: ['OrderLine'], fields: ['sku: string', 'quantity: int', 'unitPrice: int'] }, binding: { verb: 'realizes', target: 'order.OrderLine' } },
  { id: 'py:shop.order.order.OrderStatus', kind: 'type', form: 'class', title: 'OrderStatus', file: ORDER_PY, needle: 'class OrderStatus', attrs: { name: ['OrderStatus'], values: ['draft', 'paid', 'canceled', 'refunded'] }, binding: { verb: 'realizes', target: 'order.OrderStatus' } },
  { id: 'py:shop.order.order.Cart', kind: 'type', form: 'class', title: 'Cart', file: ORDER_PY, needle: 'class Cart', attrs: { name: ['Cart'], fields: ['lines: list<order.OrderLine>'] }, binding: { verb: 'realizes', target: 'order.Cart' } },
  // service.py
  { id: 'py:shop.order.service.OrderService', kind: 'type', form: 'class', title: 'OrderService', file: SERVICE_PY, needle: 'class OrderService', attrs: { name: ['OrderService'], fields: ['stock: inventory.Stock', 'payments: payment.Payments', 'orders: order.OrderRepository'] }, binding: { verb: 'realizes', target: 'order.OrderService' } },
  { id: 'py:shop.order.service.OrderService.submit', kind: 'op', form: 'method', title: 'OrderService.submit', file: SERVICE_PY, needle: 'def submit', parent: 'py:shop.order.service.OrderService', attrs: { name: ['submit'], signature: ['(cart: order.Cart) -> order.Order'], testKind: ['none'], stub: ['false'] }, binding: { verb: 'realizes', target: 'order.OrderService.submit' } },
  { id: 'py:shop.order.service.OrderService.on_paid', kind: 'op', form: 'method', title: 'OrderService.on_paid', file: SERVICE_PY, needle: 'def on_paid', parent: 'py:shop.order.service.OrderService', attrs: { name: ['onPaid'], signature: ['(event: payment.Paid)'], testKind: ['none'], stub: ['false'] }, binding: { verb: 'realizes', target: 'order.OrderService.onPaid' } },
  { id: 'py:shop.order.service._check_lines', kind: 'op', form: 'func', title: '_check_lines', file: SERVICE_PY, needle: 'def _check_lines', attrs: { name: ['_checkLines'], signature: ['(cart: order.Cart)'], testKind: ['none'], stub: ['false'] } },
  { id: 'py:shop.order.service._require_stock', kind: 'op', form: 'func', title: '_require_stock', file: SERVICE_PY, needle: 'def _require_stock', attrs: { name: ['_requireStock'], signature: ['(stock: inventory.Stock, items: list<inventory.StockItem>)'], testKind: ['none'], stub: ['false'] } },
  { id: 'py:shop.order.service._persist', kind: 'op', form: 'func', title: '_persist', file: SERVICE_PY, needle: 'def _persist', attrs: { name: ['_persist'], signature: ['(orders: order.OrderRepository, order: order.Order)'], testKind: ['none'], stub: ['false'] } },
  // repository.py
  { id: 'py:shop.order.repository._to_row', kind: 'op', form: 'func', title: '_to_row', file: REPOSITORY_PY, needle: 'def _to_row', attrs: { name: ['_toRow'], signature: ['(orders: order.OrderRepository, order: order.Order) -> map<string, string>'], testKind: ['none'], stub: ['false'] } },
  { id: 'py:shop.order.repository.OrderRepository', kind: 'type', form: 'class', title: 'OrderRepository', file: REPOSITORY_PY, needle: 'class OrderRepository', attrs: { name: ['OrderRepository'] }, binding: { verb: 'realizes', target: 'order.OrderRepository' } },
  { id: 'py:shop.order.repository.OrderRepository.save', kind: 'op', form: 'method', title: 'OrderRepository.save', file: REPOSITORY_PY, needle: 'def save', parent: 'py:shop.order.repository.OrderRepository', attrs: { name: ['save'], signature: ['(order: order.Order)'], testKind: ['none'], stub: ['true'] }, binding: { verb: 'realizes', target: 'order.OrderRepository.save' } },
  // test_order.py
  { id: 'py:shop.order.test_order.check_total_is_sum', kind: 'op', form: 'func', title: 'check_total_is_sum', file: TEST_ORDER_PY, needle: 'def check_total_is_sum', attrs: { name: ['total-is-sum'], signature: ['(order: order.Order)'], testKind: ['property'], stub: ['false'] }, binding: { verb: 'checks', target: 'order.total-is-sum' } },
  { id: 'py:shop.order.test_order.check_flow_submit_order', kind: 'op', form: 'func', title: 'check_flow_submit_order', file: TEST_ORDER_PY, needle: 'def check_flow_submit_order', attrs: { name: ['submit-order'], signature: ['(cart: order.Cart)'], testKind: ['property'], stub: ['false'] }, binding: { verb: 'checks', target: 'order.submit-order' } },
  // api.py
  { id: 'py:shop.order.api.post_order', kind: 'op', form: 'func', title: 'post_order', file: API_PY, needle: 'def post_order', attrs: { name: ['postOrder'], signature: ['(service: order.OrderService, body: map<string, list<map<string, string>>>) -> map<string, string>'], testKind: ['none'], stub: ['false'] } },
  // inventory
  { id: 'py:shop.inventory.stock.Stock', kind: 'type', form: 'class', title: 'Stock', file: STOCK_PY, needle: 'class Stock:', attrs: { name: ['Stock'], fields: ['available: map<string, int>'] }, binding: { verb: 'realizes', target: 'inventory.Stock' } },
  { id: 'py:shop.inventory.stock.Stock.covers', kind: 'op', form: 'method', title: 'Stock.covers', file: STOCK_PY, needle: 'def covers', parent: 'py:shop.inventory.stock.Stock', attrs: { name: ['covers'], signature: ['(items: list<inventory.StockItem>) -> bool'], testKind: ['none'], stub: ['false'] } },
  { id: 'py:shop.inventory.stock.Stock.reserve', kind: 'op', form: 'method', title: 'Stock.reserve', file: STOCK_PY, needle: 'def reserve', parent: 'py:shop.inventory.stock.Stock', attrs: { name: ['reserve'], signature: ['(items: list<inventory.StockItem>)'], testKind: ['none'], stub: ['false'] }, binding: { verb: 'realizes', target: 'inventory.Stock.reserve' } },
  { id: 'py:shop.inventory.stock.Stock.release', kind: 'op', form: 'method', title: 'Stock.release', file: STOCK_PY, needle: 'def release', parent: 'py:shop.inventory.stock.Stock', attrs: { name: ['release'], signature: ['(items: list<inventory.StockItem>, order: optional<order.Order>)'], testKind: ['none'], stub: ['false'] }, binding: { verb: 'realizes', target: 'inventory.Stock.release' } },
  { id: 'py:shop.inventory.stock.StockItem', kind: 'type', form: 'class', title: 'StockItem', file: STOCK_PY, needle: 'class StockItem', attrs: { name: ['StockItem'], fields: ['sku: string', 'quantity: int'] }, binding: { verb: 'realizes', target: 'inventory.StockItem' } },
  { id: 'py:shop.inventory.stock.OutOfStock', kind: 'type', form: 'class', title: 'OutOfStock', file: STOCK_PY, needle: 'class OutOfStock', attrs: { name: ['OutOfStock'] }, binding: { verb: 'realizes', target: 'inventory.OutOfStock' } },
  { id: 'py:shop.inventory.test_stock.check_available_never_negative', kind: 'op', form: 'func', title: 'check_available_never_negative', file: TEST_STOCK_PY, needle: 'def check_available_never_negative', attrs: { name: ['available-never-negative'], signature: ['(stock: inventory.Stock, steps: list<call<inventory.Stock>>)'], testKind: ['property'], stub: ['false'] }, binding: { verb: 'checks', target: 'inventory.available-never-negative' } },
  // payment
  { id: 'py:shop.payment.payments.Declined', kind: 'type', form: 'class', title: 'Declined', file: PAYMENTS_PY, needle: 'class Declined', attrs: { name: ['Declined'] }, binding: { verb: 'realizes', target: 'payment.Declined' } },
  { id: 'py:shop.payment.payments.Payments', kind: 'type', form: 'class', title: 'Payments', file: PAYMENTS_PY, needle: 'class Payments', attrs: { name: ['Payments'] }, binding: { verb: 'realizes', target: 'payment.Payments' } },
  { id: 'py:shop.payment.payments.Payments.charge', kind: 'op', form: 'method', title: 'Payments.charge', file: PAYMENTS_PY, needle: 'def charge', parent: 'py:shop.payment.payments.Payments', attrs: { name: ['charge'], signature: ['(orderId: string, amount: int)'], testKind: ['none'], stub: ['false'] }, binding: { verb: 'realizes', target: 'payment.Payments.charge' } },
  { id: 'py:shop.payment.events.Paid', kind: 'type', form: 'class', title: 'Paid', file: EVENTS_PY, needle: 'class Paid', attrs: { name: ['Paid'], fields: ['orderId: string', 'amount: int'] }, binding: { verb: 'realizes', target: 'payment.Paid' } },
  { id: 'py:shop.payment.gateway.Rejected', kind: 'type', form: 'class', title: 'Rejected', file: GATEWAY_PY, needle: 'class Rejected', attrs: { name: ['Rejected'] }, binding: { verb: 'realizes', target: 'payment-gateway.Rejected' } },
  { id: 'py:shop.payment.gateway.charge', kind: 'op', form: 'func', title: 'charge', file: GATEWAY_PY, needle: 'def charge', attrs: { name: ['charge'], signature: ['(amount: int) -> string'], testKind: ['none'], stub: ['false'] }, binding: { verb: 'realizes', target: 'payment-gateway.charge' } },
  // logging
  { id: 'py:shop.logging.log.LogRecord', kind: 'type', form: 'class', title: 'LogRecord', file: LOG_PY, needle: 'class LogRecord', attrs: { name: ['LogRecord'], fields: ['message: string'] }, binding: { verb: 'realizes', target: 'logging.LogRecord' } },
];

for (const element of ELEMENTS) {
  const origin = at(element.file, element.needle);
  const salient = element.binding !== undefined;
  if (salient) {
    SALIENT.add(element.id);
    SALIENT.add(fileId(element.file));
  }
  CODE_NODES.push(codeNode(element.kind, element.form, element.id, element.title, origin, salient, element.attrs));
  CODE_EDGES.push(edge('contains', element.parent ?? fileId(element.file), element.id, 'extracted', origin));
  if (element.binding) {
    // A binding is written on a type or a free function; a method's realizes is derived from its type's (D2).
    const derived = element.form === 'method';
    const bindingOrigin = derived ? origin : at(element.file, `@uc.${element.binding.verb}("${element.binding.target}")`);
    CODE_EDGES.push(edge(element.binding.verb, element.id, element.binding.target, derived ? 'derived' : 'declared', bindingOrigin));
  }
}

for (const pkg of PACKAGES) {
  const first = pkg.files[0] ?? '';
  const salientPackage = pkg.files.some(path => SALIENT.has(fileId(path)));
  CODE_NODES.push(codeNode('container', 'package', pkg.id, pkg.id.slice('py:'.length), at(first, 'from __future__'), salientPackage, { language: ['python'], path: [pkg.id.slice('py:'.length)] }));
  for (const path of pkg.files) {
    const origin = { path, line: 1, column: 1 };
    CODE_NODES.push(codeNode('container', 'file', fileId(path), path, origin, SALIENT.has(fileId(path)), { language: ['python'], path: [path] }));
    CODE_EDGES.push(edge('contains', pkg.id, fileId(path), 'extracted', origin));
    CODE_EDGES.push(edge('owns', pkg.owner, fileId(path), 'derived', origin));
  }
}

/** A call between two functions, with each place it is made; a needle given twice names its next occurrence. */
function calls(from: string, to: string, file: string, needles: string[]) {
  const sites = needles.map((needle, index) => site(file, needle, needles.slice(0, index).filter(earlier => earlier === needle).length + 1));
  CODE_EDGES.push(edge('calls', from, to, 'extracted', at(file, needles[0] ?? ''), { sites }));
}

const SUBMIT = 'py:shop.order.service.OrderService.submit';
const SAVE = 'py:shop.order.repository.OrderRepository.save';
const TOTAL = 'py:shop.order.order.Order.total';

calls(SUBMIT, 'py:shop.order.service._check_lines', SERVICE_PY, ['_check_lines(cart)']);
calls(SUBMIT, 'py:shop.order.service._require_stock', SERVICE_PY, ['_require_stock(self.stock']);
calls(SUBMIT, 'py:shop.inventory.stock.Stock.reserve', SERVICE_PY, ['self.stock.reserve(items)']);
calls(SUBMIT, TOTAL, SERVICE_PY, ['order.total() <= 0', 'order.total())']);
calls(SUBMIT, 'py:shop.inventory.stock.Stock.release', SERVICE_PY, ['self.stock.release(items)', 'self.stock.release(items)']);
calls(SUBMIT, 'py:shop.order.service._persist', SERVICE_PY, ['_persist(self.orders']);
calls(SUBMIT, 'py:shop.payment.payments.Payments.charge', SERVICE_PY, ['self.payments.charge(']);
calls('py:shop.order.service.OrderService.on_paid', SAVE, SERVICE_PY, ['self.orders.save(order)']);
calls('py:shop.order.service._require_stock', 'py:shop.inventory.stock.Stock.covers', SERVICE_PY, ['stock.covers(items)']);
calls('py:shop.order.service._persist', 'py:shop.order.repository._to_row', SERVICE_PY, ['_to_row(orders, order)']);
calls('py:shop.order.repository._to_row', SAVE, REPOSITORY_PY, ['orders.save(order)']);
calls(TOTAL, 'py:shop.order.order._line_amount', ORDER_PY, ['_line_amount(line) for']);
calls('py:shop.inventory.stock.Stock.reserve', 'py:shop.inventory.stock.Stock.covers', STOCK_PY, ['self.covers(items)']);
calls('py:shop.inventory.stock.Stock.release', TOTAL, STOCK_PY, ['order.total()']);
calls('py:shop.payment.payments.Payments.charge', 'py:shop.payment.gateway.charge', PAYMENTS_PY, ['gateway.charge(amount)']);
calls('py:shop.order.test_order.check_total_is_sum', TOTAL, TEST_ORDER_PY, ['order.total() ==']);
calls('py:shop.order.test_order.check_flow_submit_order', SUBMIT, TEST_ORDER_PY, ['service.submit(cart)']);
calls('py:shop.order.api.post_order', SUBMIT, API_PY, ['service.submit(']);

/** An import of a package by a file. */
function imports(file: string, pkg: string, needle: string) {
  CODE_EDGES.push(edge('imports', fileId(file), pkg, 'extracted', at(file, needle)));
}

imports(SERVICE_PY, 'py:shop.inventory', 'from shop.inventory.stock import');
imports(SERVICE_PY, 'py:shop.payment', 'from shop.payment.events import');
imports(TEST_ORDER_PY, 'py:shop.inventory', 'from shop.inventory.stock import');
imports(TEST_ORDER_PY, 'py:shop.payment', 'from shop.payment.payments import');
imports(STOCK_PY, 'py:shop.order', 'from shop.order.order import');

// D7: the salient functions submit reaches through details.
CODE_EDGES.push(edge('reaches', SUBMIT, SAVE, 'derived', at(SERVICE_PY, '_persist(self.orders'), {
  via: ['py:shop.order.service._persist', 'py:shop.order.repository._to_row'],
}));

// D4: each call step traces to the function realizing what it calls.
const TRACES: [number, string][] = [
  [1, SUBMIT],
  [2, 'py:shop.inventory.stock.Stock.reserve'],
  [3, SAVE],
  [4, 'py:shop.payment.payments.Payments.charge'],
  [5, 'py:shop.payment.gateway.charge'],
  [8, 'py:shop.inventory.stock.Stock.release'],
  [10, SAVE],
];
for (const [index, fn] of TRACES) {
  const step = `order.submit-order#${index}`;
  const stepNode = DESIGN_NODES.find(node => node.id === step);
  if (stepNode) {
    CODE_EDGES.push(edge('traces', step, fn, 'derived', stepNode.origin));
  }
}

// D5: each predicate exercises what it reaches along calls.
for (const predicate of ['py:shop.order.test_order.check_total_is_sum', 'py:shop.order.test_order.check_flow_submit_order']) {
  const seen = new Set<string>();
  const pending = [predicate];
  while (pending.length > 0) {
    const current = pending.pop() ?? '';
    for (const call of CODE_EDGES.filter(each => each.kind === 'calls' && each.from === current)) {
      if (!seen.has(call.to)) {
        seen.add(call.to);
        pending.push(call.to);
      }
    }
  }
  const origin = CODE_NODES.find(node => node.id === predicate)?.origin ?? { path: TEST_ORDER_PY, line: 1, column: 1 };
  for (const fn of seen) {
    CODE_EDGES.push(edge('exercises', predicate, fn, 'derived', origin));
  }
}

// D3: the actual dependencies between design containers, with the imports and calls that make them.
const DEPENDS: { from: string; to: string; imports: string[]; calls: string[] }[] = [
  {
    from: 'order', to: 'inventory',
    imports: [`${fileId(SERVICE_PY)} py:shop.inventory`],
    calls: [`${SUBMIT} py:shop.inventory.stock.Stock.reserve`, `${SUBMIT} py:shop.inventory.stock.Stock.release`],
  },
  {
    from: 'order', to: 'payment',
    imports: [`${fileId(SERVICE_PY)} py:shop.payment`],
    calls: [`${SUBMIT} py:shop.payment.payments.Payments.charge`],
  },
  {
    from: 'payment', to: 'payment-gateway',
    imports: [],
    calls: ['py:shop.payment.payments.Payments.charge py:shop.payment.gateway.charge'],
  },
  {
    from: 'inventory', to: 'order',
    imports: [`${fileId(STOCK_PY)} py:shop.order`],
    calls: [`py:shop.inventory.stock.Stock.release ${TOTAL}`],
  },
];
for (const dependency of DEPENDS) {
  const origin = DESIGN_NODES.find(node => node.id === dependency.from)?.origin ?? { path: 'design/system.uc', line: 1, column: 1 };
  CODE_EDGES.push(edge('depends', dependency.from, dependency.to, 'derived', origin, {
    ...(dependency.imports.length > 0 ? { imports: dependency.imports } : {}),
    calls: dependency.calls,
  }));
}

// The code's own references between types, as the backend extracts them from fields and signatures: they mirror the
// design's references between the types and operations these elements realize.
const REALIZER = new Map(CODE_EDGES.filter(each => each.kind === 'realizes').map(each => [each.to, each.from]));
for (const refers of DESIGN_EDGES.filter(each => each.kind === 'refers')) {
  const from = REALIZER.get(refers.from);
  const to = REALIZER.get(refers.to);
  if (from && to) {
    const origin = CODE_NODES.find(node => node.id === from)?.origin ?? refers.origin;
    CODE_EDGES.push(edge('refers', from, to, 'extracted', origin));
  }
}
