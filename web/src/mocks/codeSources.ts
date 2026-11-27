/* The Python code of the shop: the files the code layer of the graph is extracted from. */

export const CODE_SOURCES: Record<string, string> = {
  "src/shop/order/order.py": `from __future__ import annotations
import enum
from dataclasses import dataclass
import uc


@uc.realizes("order.Order")
class Order:
    """An order: its lines and its status; the total follows from the lines."""

    def __init__(self, id: str, lines: list[OrderLine], status: OrderStatus) -> None:
        self.id = id
        self.lines = lines
        self.status = status

    def mark_paid(self) -> None:
        if self.status is not OrderStatus.DRAFT:
            raise ValueError(f"order {self.id} is {self.status.value}, not draft")
        self.status = OrderStatus.PAID

    # The sum of the lines' amounts, in cents.
    def total(self) -> int:
        return sum(_line_amount(line) for line in self.lines)


@uc.realizes("order.EmptyCart")
class EmptyCart(Exception):
    """The cart has no lines."""


def _line_amount(line: OrderLine) -> int:
    return line.quantity * line.unit_price


@uc.realizes("order.OrderLine")
@dataclass(frozen=True)
class OrderLine:
    sku: str
    quantity: int
    unit_price: int


@uc.realizes("order.OrderStatus")
class OrderStatus(enum.Enum):
    DRAFT = "draft"
    PAID = "paid"
    CANCELED = "canceled"
    REFUNDED = "refunded"


@uc.realizes("order.Cart")
@dataclass(frozen=True)
class Cart:
    lines: list[OrderLine]
`,
  "src/shop/order/service.py": `from __future__ import annotations

import uc
from shop.inventory.stock import Stock, StockItem
from shop.payment.events import Paid
from shop.payment.payments import Declined, Payments

from .order import Cart, EmptyCart, Order, OrderStatus
from .repository import OrderRepository, _to_row

@uc.realizes("order.OrderService")
class OrderService:
    """Places orders: reserves the stock, saves the order and charges for it.

    The service holds no orders of its own: the repository keeps them, and
    every change of an order's status goes through it. The stock and the
    payments are the other components' objects, which the service only calls.

    Submitting a cart follows the flow order.submit-order:

    1. the cart's lines are checked and the stock is asked to cover them;
    2. the items are reserved;
    3. the order is saved as a draft;
    4. the order's total is charged;
    5. a declined charge returns the reserved items to the stock.

    The order becomes paid when the payment component publishes Paid, which
    on_paid handles. Both are traced to the flow's steps by the viewer.

    Errors other than Declined leave the reservation to the inventory.
    """

    def __init__(self, stock: Stock, payments: Payments, orders: OrderRepository) -> None:
        self.stock = stock
        self.payments = payments
        self.orders = orders

    # Submitting a cart reserves its items first, so a declined charge can
    # give them back; the order is saved before the charge is attempted.
    def submit(self, cart: Cart) -> Order:
        if not cart.lines:
            raise EmptyCart()
        _check_lines(cart)
        items = [StockItem(line.sku, line.quantity) for line in cart.lines]
        _require_stock(self.stock, items)
        self.stock.reserve(items)
        order = Order(
            id=self.orders.next_id(),
            lines=list(cart.lines),
            status=OrderStatus.DRAFT,
        )
        if order.total() <= 0:
            self.stock.release(items)
            raise EmptyCart()
        _persist(self.orders, order)
        try:
            self.payments.charge(order.id, order.total())
            # A declined charge leaves the order a draft and
            # returns the reserved items to the stock.
            return order
        except Declined:
            self.stock.release(items)
            raise

    def on_paid(self, event: Paid) -> None:
        order = self.orders.find(event.order_id)
        if order is None:
            return
        order.status = OrderStatus.PAID
        self.orders.save(order)


# The helpers below are details: they realize nothing in the design.


def _check_lines(cart: Cart) -> None:
    """Every line asks for at least one item."""
    for line in cart.lines:
        if line.quantity <= 0:
            raise ValueError(f"line {line.sku} has no quantity")


def _require_stock(stock: Stock, items: list[StockItem]) -> None:
    if not stock.covers(items):
        raise ValueError("the stock does not cover the cart")


def _persist(orders: OrderRepository, order: Order) -> None:
    _to_row(orders, order)
`,
  "src/shop/order/repository.py": `from __future__ import annotations

from typing import Protocol

import uc

from .order import Order


# A row of the orders table.
Row = dict[str, object]


def _to_row(orders: OrderRepository, order: Order) -> Row:
    row: Row = {
        "id": order.id,
        "status": order.status.value,
        "lines": [(line.sku, line.quantity, line.unit_price) for line in order.lines],
    }
    orders.save(order)
    return row


@uc.realizes("order.OrderRepository")
class OrderRepository(Protocol):
    def save(self, order: Order) -> None:
        raise NotImplementedError("generated by unicode gen")

    def find(self, order_id: str) -> Order | None: ...

    def next_id(self) -> str: ...
`,
  "src/shop/order/test_order.py": `from __future__ import annotations

import uc
from shop.inventory.stock import Stock
from shop.payment.payments import Payments

from .order import Cart, Order
from .repository import OrderRepository
from .service import OrderService

@uc.checks("order.total-is-sum")
def check_total_is_sum(order: Order) -> None:
    expected = sum(line.quantity * line.unit_price for line in order.lines)
    assert order.total() == expected, "the total is not the sum of the lines"


class MemoryRepository:
    def __init__(self) -> None:
        self.orders: dict[str, Order] = {}

    def save(self, order: Order) -> None:
        self.orders[order.id] = order

    def find(self, order_id: str) -> Order | None:
        return self.orders.get(order_id)


# The flow's predicate drives submit and checks the calls against the steps.
@uc.checks("order.submit-order")
def check_flow_submit_order(cart: Cart) -> None:
    service = OrderService(Stock(), Payments(), MemoryRepository())
    order = service.submit(cart)
    assert order.lines == cart.lines, "the order does not keep the cart's lines"
`,
  "src/shop/order/api.py": `from __future__ import annotations

from .order import Cart, OrderLine
from .service import OrderService


def post_order(service: OrderService, body: dict[str, list[dict[str, object]]]) -> dict[str, str]:
    lines = [OrderLine(str(line["sku"]), int(line["quantity"]), int(line["unitPrice"])) for line in body["lines"]]
    order = service.submit(Cart(lines))
    return {"id": order.id}
`,
  "src/shop/inventory/stock.py": `from __future__ import annotations

from dataclasses import dataclass, field

import uc
from shop.order.order import Order


@uc.realizes("inventory.Stock")
@dataclass
class Stock:
    available: dict[str, int] = field(default_factory=dict)

    def covers(self, items: list[StockItem]) -> bool:
        return all(self.available.get(item.sku, 0) >= item.quantity for item in items)

    # Nothing is reserved unless every item is available.
    def reserve(self, items: list[StockItem]) -> None:
        if not self.covers(items):
            raise OutOfStock()
        for item in items:
            self.available[item.sku] -= item.quantity

    def release(self, items: list[StockItem], order: Order | None = None) -> None:
        for item in items:
            self.available[item.sku] = self.available.get(item.sku, 0) + item.quantity
        if order is not None:
            order.total()


@uc.realizes("inventory.StockItem")
@dataclass(frozen=True)
class StockItem:
    sku: str
    quantity: int


@uc.realizes("inventory.OutOfStock")
class OutOfStock(Exception):
    """Some item is not available in the quantity asked for."""
`,
  "src/shop/inventory/test_stock.py": `from __future__ import annotations

import uc

from .stock import Stock


@uc.checks("inventory.available-never-negative")
def check_available_never_negative(stock: Stock, steps: list[object]) -> None:
    for step in steps:
        step(stock)
        assert all(quantity >= 0 for quantity in stock.available.values()), "a quantity went below zero"
`,
  "src/shop/payment/payments.py": `from __future__ import annotations

import uc

from . import gateway


@uc.realizes("payment.Declined")
class Declined(Exception):
    """The charge was declined."""


@uc.realizes("payment.Payments")
class Payments:
    """Charges through the external gateway."""

    def __init__(self) -> None:
        self.references: dict[str, str] = {}

    def reference(self, order_id: str) -> str | None:
        return self.references.get(order_id)

    def refund(self, order_id: str) -> None:
        self.references.pop(order_id, None)

    def references_of(self, order_ids: list[str]) -> list[str]:
        return [self.references[order_id] for order_id in order_ids if order_id in self.references]

    # A rejection by the gateway is a declined charge; the reference is kept
    # so that the order can be traced to the gateway's records.
    def charge(self, order_id: str, amount: int) -> None:
        try:
            self.references[order_id] = gateway.charge(amount)
        except gateway.Rejected as rejected:
            raise Declined() from rejected
`,
  "src/shop/payment/events.py": `from __future__ import annotations

import uc

@uc.realizes("payment.Paid")
class Paid:
    def __init__(self, order_id: str, amount: int) -> None:
        self.order_id = order_id
        self.amount = amount
`,
  "src/shop/payment/gateway.py": `from __future__ import annotations

import uc


@uc.realizes("payment-gateway.Rejected")
class Rejected(Exception):
    """The gateway refused the charge."""


@uc.realizes("payment-gateway.charge")
def charge(amount: int) -> str:
    raise Rejected(f"no gateway configured for {amount}")
`,
  "src/shop/logging/log.py": `from __future__ import annotations

from dataclasses import dataclass

import uc


@uc.realizes("logging.LogRecord")
@dataclass(frozen=True)
class LogRecord:
    message: str
`,
};
