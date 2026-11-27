/* The design and corpus files of the shop (docs/language.md §7), as the repository holds them. */

export const DESIGN_SOURCES: Record<string, string> = {
  "design/system.uc": `system shop "网上商店：浏览商品、下单、支付"

notation ddd

notation c4

language python {
  root "src"
}

actor customer person "在商店下单的人"

actor payment-gateway external "外部支付网关" {
  type Rejected error "网关拒绝扣款"
  op charge(amount: int) -> string raises Rejected "扣款，返回网关的流水号"
}
`,
  "design/inventory.uc": `component inventory "库存：各商品的可用数量与预留" {
  label bounded-context
  label container
  focus support
  paths "src/shop/inventory/**"
}

type StockItem record "一种商品及其数量" {
  label value
  field sku: string
  field quantity: int
}

type OutOfStock error "某种商品的可用数量不足"

type Stock object "各商品的可用数量" {
  label aggregate
  field available: map<string, int>
  op reserve(items: list<StockItem>) raises OutOfStock "预留数量；任何一项不足时都不预留"
  op release(items: list<StockItem>) "释放此前预留的数量"
}

property available-never-negative invariant "任意一串预留和释放之后，每种商品的可用数量都不小于 0" {
  about Stock.reserve, Stock.release
  forall stock: Stock
  forall steps: list<call<Stock>>
  where stock.available size 0..5
  where stock.available in 0..100
  where steps size 0..30
  where steps.reserve.items.quantity in 1..10
  where steps.release.items.quantity in 1..10
  ensure all(sku in stock.available: stock.available[sku] >= 0)
  case reserve-then-release {
    stock = {available: {"a": 5}}
    steps = [reserve(items: [{sku: "a", quantity: 3}]), release(items: [{sku: "a", quantity: 3}])]
  }
}
`,
  "design/payment.uc": `component payment "支付：经外部网关扣款" {
  label bounded-context
  label container
  focus support
  paths "src/shop/payment/**"
  uses payment-gateway
}

type Declined error "扣款被拒"

type Paid event "一笔订单已扣款" {
  field orderId: string
  field amount: int
}

type Payments object "扣款" {
  op charge(orderId: string, amount: int) raises Declined "经网关扣款；成功后发布 Paid"
}
`,
  "design/order.uc": `component order "订单：从购物车生成订单，并管理订单的状态" {
  label bounded-context
  label container
  focus core
  paths "src/shop/order/**"
  uses inventory, payment
}

type OrderLine record "订单中的一行：商品、数量和单价" {
  label value
  field sku: string
  field quantity: int
  field unitPrice: int
}

type OrderStatus enum "订单的状态" {
  value draft
  value paid
  value canceled
}

type Order object "一笔订单：行项目和状态，总额由行项目算出" {
  label aggregate
  field id: string
  field lines: list<OrderLine>
  field status: OrderStatus
  op total() -> int pure "各行数量乘单价之和，单位为分"
}

type Cart record "顾客提交的购物车" {
  label value
  field lines: list<OrderLine>
}

type EmptyCart error "购物车为空"

type OrderRepository interface "订单的持久化" {
  label repository
  op save(order: Order)
}

type OrderService object "下单：预留库存、保存订单、扣款" {
  label domain-service
  field stock: inventory.Stock
  field payments: payment.Payments
  field orders: OrderRepository
  op submit(cart: Cart) -> Order raises EmptyCart, inventory.OutOfStock, payment.Declined "从购物车生成订单"
  op onPaid(event: payment.Paid) handles payment.Paid "扣款成功后把订单记为已支付"
}

flow submit-order "顾客提交购物车：预留库存、保存订单并扣款；扣款成功后订单记为已支付" {
  forall cart: Cart
  where cart.lines size 1..10
  where cart.lines.quantity in 1..5
  where cart.lines.unitPrice in 1..100000
  customer -> OrderService.submit(cart) {
    OrderService -> inventory.Stock.reserve(items)
    OrderService -> OrderRepository.save(order)
    OrderService -> payment.Payments.charge(orderId, amount) {
      payment.Payments -> payment-gateway.charge(amount)
      payment.Payments emits payment.Paid(orderId, amount)
    }
    on payment.Declined {
      OrderService -> inventory.Stock.release(items)
    }
  }
  async OrderService.onPaid {
    OrderService -> OrderRepository.save(order)
  }
  within "500ms"
}

property total-is-sum invariant "订单总额等于各行数量乘单价之和" {
  about Order.total
  forall order: Order
  where order.lines size 0..20
  where order.lines.quantity in 1..99
  where order.lines.unitPrice in 0..1000000
  ensure order.total() == sum(l in order.lines: l.quantity * l.unitPrice)
  case empty "没有行项目时总额为 0" {
    order = {id: "o1", lines: [], status: draft}
  }
  case two-lines {
    order = {
      id: "o2",
      lines: [{sku: "a", quantity: 2, unitPrice: 300}, {sku: "b", quantity: 1, unitPrice: 50}],
      status: draft,
    }
  }
}

property declined-keeps-stock postcondition "扣款被拒时，提交前后各商品的可用数量相同" {
  about OrderService.submit
  forall service: OrderService
  forall cart: Cart
  where cart.lines size 1..10
  call service.submit(cart)
  ensure raised(payment.Declined) implies service.stock.available == old(service.stock.available)
}
`,
  "corpus/order/total-is-sum.uc": `corpus order.total-is-sum

case r-3f9a2c1d coverage {
  order = {id: "", lines: [{sku: "", quantity: 99, unitPrice: 1000000}], status: canceled}
}

case r-77e01b2a counterexample {
  order = {
    id: "",
    lines: [
      {sku: "", quantity: 1, unitPrice: 0},
      {sku: "", quantity: 0, unitPrice: 7},
    ],
    status: draft,
  }
}
`,
  "corpus/order/submit-order.uc": `corpus order.submit-order

case r-3f9a2c1d coverage {
  cart = {lines: [{sku: "a", quantity: 1, unitPrice: 100}]}
}
`,
  "design/logging.uc": `component logging "日志：结构化日志的写出" {
  focus utility
  paths "src/shop/logging/**"
}

type LogRecord record "一条日志" {
  field message: string
}
`,
};
