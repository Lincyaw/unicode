import { at, designNode, edge } from './fixture';
import type { NodeKind, RawEdge, RawNode } from './schemaTypes';

/*
 * The design layer of the shop: docs/language.md §7, as `unicode build` compiles it (docs/design.md §5.2), and a
 * utility component `logging`, which the architecture view hides. References in attributes are written from the root.
 */

const SYSTEM_FILE = 'design/system.uc';
const INVENTORY_FILE = 'design/inventory.uc';
const PAYMENT_FILE = 'design/payment.uc';
const ORDER_FILE = 'design/order.uc';
const LOGGING_FILE = 'design/logging.uc';

export const SYSTEM_ID = 'shop';
export const FLOW_ID = 'order.submit-order';

export const DESIGN_NODES: RawNode[] = [];
export const DESIGN_EDGES: RawEdge[] = [];

/** Declares a node inside its parent: the node, and the contains edge from the parent. */
function declare(parent: string | undefined, node: RawNode) {
  DESIGN_NODES.push(node);
  if (parent !== undefined) {
    DESIGN_EDGES.push(edge('contains', parent, node.id, 'declared', node.origin));
  }
}

function declared(kind: NodeKind, form: string, parent: string | undefined, id: string, summary: string, file: string, needle: string, attrs: Record<string, string[]> = {}) {
  declare(parent, designNode(kind, form, id, summary, at(file, needle), attrs));
}

// design/system.uc
declared('container', 'system', undefined, SYSTEM_ID, '网上商店：浏览商品、下单、支付', SYSTEM_FILE, 'system shop', {
  notations: ['ddd', 'c4'],
  roots: ['python: src'],
});
declared('actor', 'person', SYSTEM_ID, 'customer', '在商店下单的人', SYSTEM_FILE, 'actor customer');
declared('actor', 'external', SYSTEM_ID, 'payment-gateway', '外部支付网关', SYSTEM_FILE, 'actor payment-gateway');
declared('type', 'error', 'payment-gateway', 'payment-gateway.Rejected', '网关拒绝扣款', SYSTEM_FILE, 'type Rejected');
declared('op', 'op', 'payment-gateway', 'payment-gateway.charge', '扣款，返回网关的流水号', SYSTEM_FILE, 'op charge', {
  signature: ['(amount: int) -> string raises payment-gateway.Rejected'],
});

// design/inventory.uc
declared('container', 'component', SYSTEM_ID, 'inventory', '库存：各商品的可用数量与预留', INVENTORY_FILE, 'component inventory', {
  focus: ['support'],
  labels: ['bounded-context', 'container'],
  paths: ['src/shop/inventory/**'],
});
declared('type', 'record', 'inventory', 'inventory.StockItem', '一种商品及其数量', INVENTORY_FILE, 'type StockItem', {
  labels: ['value'],
  fields: ['sku: string', 'quantity: int'],
});
declared('type', 'error', 'inventory', 'inventory.OutOfStock', '某种商品的可用数量不足', INVENTORY_FILE, 'type OutOfStock');
declared('type', 'object', 'inventory', 'inventory.Stock', '各商品的可用数量', INVENTORY_FILE, 'type Stock object', {
  labels: ['aggregate'],
  fields: ['available: map<string, int>'],
});
declared('op', 'op', 'inventory.Stock', 'inventory.Stock.reserve', '预留数量；任何一项不足时都不预留', INVENTORY_FILE, 'op reserve', {
  signature: ['(items: list<inventory.StockItem>) raises inventory.OutOfStock'],
});
declared('op', 'op', 'inventory.Stock', 'inventory.Stock.release', '释放此前预留的数量', INVENTORY_FILE, 'op release', {
  signature: ['(items: list<inventory.StockItem>)'],
});
declared('property', 'invariant', 'inventory', 'inventory.available-never-negative', '任意一串预留和释放之后，每种商品的可用数量都不小于 0', INVENTORY_FILE, 'property available-never-negative', {
  symbols: ['stock: inventory.Stock', 'steps: list<call<inventory.Stock>>'],
  limits: [
    'stock.available size 0..5',
    'stock.available in 0..100',
    'steps size 0..30',
    'steps.reserve.items.quantity in 1..10',
    'steps.release.items.quantity in 1..10',
  ],
  ensure: ['all(sku in stock.available: stock.available[sku] >= 0)'],
});
declared('case', 'declared', 'inventory.available-never-negative', 'inventory.available-never-negative.reserve-then-release', '', INVENTORY_FILE, 'case reserve-then-release', {
  values: [
    'stock = {available: {"a": 5}}',
    'steps = [reserve(items: [{sku: "a", quantity: 3}]), release(items: [{sku: "a", quantity: 3}])]',
  ],
});

// design/payment.uc
declared('container', 'component', SYSTEM_ID, 'payment', '支付：经外部网关扣款', PAYMENT_FILE, 'component payment', {
  focus: ['support'],
  labels: ['bounded-context', 'container'],
  paths: ['src/shop/payment/**'],
});
declared('type', 'error', 'payment', 'payment.Declined', '扣款被拒', PAYMENT_FILE, 'type Declined');
declared('type', 'event', 'payment', 'payment.Paid', '一笔订单已扣款', PAYMENT_FILE, 'type Paid', {
  fields: ['orderId: string', 'amount: int'],
});
declared('type', 'object', 'payment', 'payment.Payments', '扣款', PAYMENT_FILE, 'type Payments');
declared('op', 'op', 'payment.Payments', 'payment.Payments.charge', '经网关扣款；成功后发布 Paid', PAYMENT_FILE, 'op charge', {
  signature: ['(orderId: string, amount: int) raises payment.Declined'],
});

// design/order.uc
declared('container', 'component', SYSTEM_ID, 'order', '订单：从购物车生成订单，并管理订单的状态', ORDER_FILE, 'component order', {
  focus: ['core'],
  labels: ['bounded-context', 'container'],
  paths: ['src/shop/order/**'],
});
declared('type', 'record', 'order', 'order.OrderLine', '订单中的一行：商品、数量和单价', ORDER_FILE, 'type OrderLine', {
  labels: ['value'],
  fields: ['sku: string', 'quantity: int', 'unitPrice: int'],
});
declared('type', 'enum', 'order', 'order.OrderStatus', '订单的状态', ORDER_FILE, 'type OrderStatus', {
  values: ['draft', 'paid', 'canceled'],
});
declared('type', 'object', 'order', 'order.Order', '一笔订单：行项目和状态，总额由行项目算出', ORDER_FILE, 'type Order object', {
  labels: ['aggregate'],
  fields: ['id: string', 'lines: list<order.OrderLine>', 'status: order.OrderStatus'],
});
declared('op', 'op', 'order.Order', 'order.Order.total', '各行数量乘单价之和，单位为分', ORDER_FILE, 'op total', {
  signature: ['() -> int'],
  pure: ['true'],
});
declared('type', 'record', 'order', 'order.Cart', '顾客提交的购物车', ORDER_FILE, 'type Cart', {
  labels: ['value'],
  fields: ['lines: list<order.OrderLine>'],
});
declared('type', 'error', 'order', 'order.EmptyCart', '购物车为空', ORDER_FILE, 'type EmptyCart');
declared('type', 'interface', 'order', 'order.OrderRepository', '订单的持久化', ORDER_FILE, 'type OrderRepository', {
  labels: ['repository'],
});
declared('op', 'op', 'order.OrderRepository', 'order.OrderRepository.save', '', ORDER_FILE, 'op save', {
  signature: ['(order: order.Order)'],
});
declared('type', 'object', 'order', 'order.OrderService', '下单：预留库存、保存订单、扣款', ORDER_FILE, 'type OrderService', {
  labels: ['domain-service'],
  fields: ['stock: inventory.Stock', 'payments: payment.Payments', 'orders: order.OrderRepository'],
});
declared('op', 'op', 'order.OrderService', 'order.OrderService.submit', '从购物车生成订单', ORDER_FILE, 'op submit', {
  signature: ['(cart: order.Cart) -> order.Order raises order.EmptyCart, inventory.OutOfStock, payment.Declined'],
});
declared('op', 'op', 'order.OrderService', 'order.OrderService.onPaid', '扣款成功后把订单记为已支付', ORDER_FILE, 'op onPaid', {
  signature: ['(event: payment.Paid)'],
});
declared('flow', 'flow', 'order', FLOW_ID, '顾客提交购物车：预留库存、保存订单并扣款；扣款成功后订单记为已支付', ORDER_FILE, 'flow submit-order', {
  symbols: ['cart: order.Cart'],
  limits: ['cart.lines size 1..10', 'cart.lines.quantity in 1..5', 'cart.lines.unitPrice in 1..100000'],
  within: ['500ms'],
});
declared('property', 'invariant', 'order', 'order.total-is-sum', '订单总额等于各行数量乘单价之和', ORDER_FILE, 'property total-is-sum', {
  symbols: ['order: order.Order'],
  limits: ['order.lines size 0..20', 'order.lines.quantity in 1..99', 'order.lines.unitPrice in 0..1000000'],
  ensure: ['order.total() == sum(l in order.lines: l.quantity * l.unitPrice)'],
});
declared('case', 'declared', 'order.total-is-sum', 'order.total-is-sum.empty', '没有行项目时总额为 0', ORDER_FILE, 'case empty', {
  values: ['order = {id: "o1", lines: [], status: draft}'],
});
declared('case', 'declared', 'order.total-is-sum', 'order.total-is-sum.two-lines', '', ORDER_FILE, 'case two-lines', {
  values: ['order = {id: "o2", lines: [{sku: "a", quantity: 2, unitPrice: 300}, {sku: "b", quantity: 1, unitPrice: 50}], status: draft}'],
});
declared('property', 'postcondition', 'order', 'order.declined-keeps-stock', '扣款被拒时，提交前后各商品的可用数量相同', ORDER_FILE, 'property declined-keeps-stock', {
  symbols: ['service: order.OrderService', 'cart: order.Cart'],
  limits: ['cart.lines size 1..10'],
  call: ['service.submit(cart)'],
  ensure: ['raised(payment.Declined) implies service.stock.available == old(service.stock.available)'],
});

// corpus/order/*.uc: the recorded cases
declared('case', 'recorded', 'order.total-is-sum', 'order.total-is-sum.r-3f9a2c1d', '', 'corpus/order/total-is-sum.uc', 'case r-3f9a2c1d', {
  values: ['order = {id: "", lines: [{sku: "", quantity: 99, unitPrice: 1000000}], status: canceled}'],
  reason: ['coverage'],
});
declared('case', 'recorded', 'order.total-is-sum', 'order.total-is-sum.r-77e01b2a', '', 'corpus/order/total-is-sum.uc', 'case r-77e01b2a', {
  values: ['order = {id: "", lines: [{sku: "", quantity: 1, unitPrice: 0}, {sku: "", quantity: 0, unitPrice: 7}], status: draft}'],
  reason: ['counterexample'],
});
declared('case', 'recorded', FLOW_ID, `${FLOW_ID}.r-3f9a2c1d`, '', 'corpus/order/submit-order.uc', 'case r-3f9a2c1d', {
  values: ['cart = {lines: [{sku: "a", quantity: 1, unitPrice: 100}]}'],
  reason: ['coverage'],
});

// design/logging.uc
declared('container', 'component', SYSTEM_ID, 'logging', '日志：结构化日志的写出', LOGGING_FILE, 'component logging', {
  focus: ['utility'],
  paths: ['src/shop/logging/**'],
});
declared('type', 'record', 'logging', 'logging.LogRecord', '一条日志', LOGGING_FILE, 'type LogRecord', {
  fields: ['message: string'],
});

/*
 * The steps of order.submit-order, numbered in the text's order. Each is a node under its flow or its enclosing step,
 * with its sender (from) and what it calls, emits or handles.
 */
interface StepSpec {
  index: number;
  form: string;
  parent: string;
  needle: string;
  occurrence?: number;
  sender?: string;
  calls?: string;
  emits?: string;
  handles?: string;
  args?: string[];
}

const STEPS: StepSpec[] = [
  { index: 1, form: 'call', parent: FLOW_ID, needle: 'customer -> OrderService.submit(cart)', sender: 'customer', calls: 'order.OrderService.submit', args: ['cart'] },
  { index: 2, form: 'call', parent: '#1', needle: 'OrderService -> inventory.Stock.reserve(items)', sender: 'order.OrderService', calls: 'inventory.Stock.reserve', args: ['items'] },
  { index: 3, form: 'call', parent: '#1', needle: 'OrderService -> OrderRepository.save(order)', sender: 'order.OrderService', calls: 'order.OrderRepository.save', args: ['order'] },
  { index: 4, form: 'call', parent: '#1', needle: 'OrderService -> payment.Payments.charge(orderId, amount)', sender: 'order.OrderService', calls: 'payment.Payments.charge', args: ['orderId', 'amount'] },
  { index: 5, form: 'call', parent: '#4', needle: 'payment.Payments -> payment-gateway.charge(amount)', sender: 'payment.Payments', calls: 'payment-gateway.charge', args: ['amount'] },
  { index: 6, form: 'emit', parent: '#4', needle: 'payment.Payments emits payment.Paid(orderId, amount)', sender: 'payment.Payments', emits: 'payment.Paid', args: ['orderId', 'amount'] },
  { index: 7, form: 'on', parent: '#1', needle: 'on payment.Declined', handles: 'payment.Declined' },
  { index: 8, form: 'call', parent: '#7', needle: 'OrderService -> inventory.Stock.release(items)', sender: 'order.OrderService', calls: 'inventory.Stock.release', args: ['items'] },
  { index: 9, form: 'async', parent: FLOW_ID, needle: 'async OrderService.onPaid', calls: 'order.OrderService.onPaid' },
  { index: 10, form: 'call', parent: '#9', needle: 'OrderService -> OrderRepository.save(order)', occurrence: 2, sender: 'order.OrderService', calls: 'order.OrderRepository.save', args: ['order'] },
];

for (const step of STEPS) {
  const id = `${FLOW_ID}#${step.index}`;
  const parent = step.parent.startsWith('#') ? `${FLOW_ID}${step.parent}` : step.parent;
  const origin = at(ORDER_FILE, step.needle, step.occurrence);
  declare(parent, designNode('step', step.form, id, '', origin, { index: [String(step.index)], ...(step.args ? { args: step.args } : {}) }));
  if (step.sender) {
    DESIGN_EDGES.push(edge('from', id, step.sender, 'declared', origin));
  }
  if (step.calls) {
    DESIGN_EDGES.push(edge('calls', id, step.calls, 'declared', origin));
  }
  if (step.emits) {
    DESIGN_EDGES.push(edge('emits', id, step.emits, 'declared', origin));
  }
  if (step.handles) {
    DESIGN_EDGES.push(edge('handles', id, step.handles, 'declared', origin));
  }
}

/** A declared clause: an edge whose origin is the clause in its file. */
function clause(kind: RawEdge['kind'], from: string, to: string, file: string, needle: string) {
  DESIGN_EDGES.push(edge(kind, from, to, 'declared', at(file, needle)));
}

clause('uses', 'payment', 'payment-gateway', PAYMENT_FILE, 'uses payment-gateway');
clause('uses', 'order', 'inventory', ORDER_FILE, 'uses inventory, payment');
clause('uses', 'order', 'payment', ORDER_FILE, 'uses inventory, payment');
clause('subscribes', 'order.OrderService.onPaid', 'payment.Paid', ORDER_FILE, 'handles payment.Paid');
clause('about', 'inventory.available-never-negative', 'inventory.Stock.reserve', INVENTORY_FILE, 'about Stock.reserve, Stock.release');
clause('about', 'inventory.available-never-negative', 'inventory.Stock.release', INVENTORY_FILE, 'about Stock.reserve, Stock.release');
clause('about', 'order.total-is-sum', 'order.Order.total', ORDER_FILE, 'about Order.total');
clause('about', 'order.declined-keeps-stock', 'order.OrderService.submit', ORDER_FILE, 'about OrderService.submit');

/** The identities of the design's types, which the references in fields, signatures and symbols name. */
const TYPE_IDS = new Set(DESIGN_NODES.filter(node => node.kind === 'type').map(node => node.id));

/** The declared types a written type, signature or symbol names, in their order, each once. */
export function typesNamedIn(text: string): string[] {
  const named = text.match(/[a-z][\w-]*(?:\.[A-Za-z][\w-]*)+/g) ?? [];
  return [...new Set(named.filter(name => TYPE_IDS.has(name)))];
}

// refers: from fields to their types, from operations to the types of their signatures, from properties and flows to
// the types of their symbols.
for (const node of DESIGN_NODES) {
  const texts = [...(node.attrs.fields ?? []), ...(node.attrs.signature ?? []), ...(node.attrs.symbols ?? [])];
  for (const target of typesNamedIn(texts.join(' '))) {
    if (target !== node.id) {
      DESIGN_EDGES.push(edge('refers', node.id, target, 'declared', node.origin));
    }
  }
}
