from __future__ import annotations

import json
from collections import defaultdict
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any, Optional

from cassandra.query import BatchStatement, BatchType, PreparedStatement


def _ms(x: Optional[int]) -> int:
    return int(x or 0)


def _i(x: Optional[int]) -> int:
    return int(x or 0)


@dataclass
class ZoneSnapshot:
    available: int
    reserved: int
    last_event_ms: int


@dataclass
class ProductAgg:
    total_available: int
    total_reserved: int


@dataclass
class OrderRow:
    status: str
    last_event_ms: int
    lines_json: str


class PreparedCQL:
    def __init__(self, session: Any) -> None:
        self.session = session
        self.sel_processed: PreparedStatement = session.prepare(
            "SELECT event_id FROM processed_events WHERE event_id = ?"
        )
        self.sel_pz: PreparedStatement = session.prepare(
            "SELECT available, reserved, last_event_ms FROM inventory_by_product_zone WHERE product_id = ? AND zone_id = ?"
        )
        self.sel_p: PreparedStatement = session.prepare(
            "SELECT total_available, total_reserved FROM inventory_by_product WHERE product_id = ?"
        )
        self.sel_z: PreparedStatement = session.prepare(
            "SELECT available, reserved, last_event_ms FROM inventory_by_zone WHERE zone_id = ? AND product_id = ?"
        )
        self.sel_order: PreparedStatement = session.prepare(
            "SELECT status, last_event_ms, lines_json FROM orders WHERE order_id = ?"
        )

        self.upd_pz: PreparedStatement = session.prepare(
            """
            UPDATE inventory_by_product_zone
            SET available = ?, reserved = ?, last_event_ms = ?
            WHERE product_id = ? AND zone_id = ?
            """
        )
        self.upd_p: PreparedStatement = session.prepare(
            """
            UPDATE inventory_by_product
            SET total_available = ?, total_reserved = ?
            WHERE product_id = ?
            """
        )
        self.upd_z: PreparedStatement = session.prepare(
            """
            UPDATE inventory_by_zone
            SET available = ?, reserved = ?, last_event_ms = ?
            WHERE zone_id = ? AND product_id = ?
            """
        )
        self.ins_processed: PreparedStatement = session.prepare(
            "INSERT INTO processed_events (event_id, processed_at) VALUES (?, ?)"
        )
        self.ins_history: PreparedStatement = session.prepare(
            """
            INSERT INTO event_history (entity_type, entity_id, event_id, event_type, occurred_at_ms, payload_json)
            VALUES (?, ?, ?, ?, ?, ?)
            """
        )
        self.ins_order: PreparedStatement = session.prepare(
            """
            INSERT INTO orders (order_id, status, last_event_ms, lines_json)
            VALUES (?, ?, ?, ?)
            """
        )
        self.upd_order: PreparedStatement = session.prepare(
            """
            UPDATE orders SET status = ?, last_event_ms = ? WHERE order_id = ?
            """
        )


def _load_pz(session: Any, ps: PreparedCQL, product_id: str, zone_id: str) -> ZoneSnapshot:
    row = session.execute(ps.sel_pz, (product_id, zone_id)).one()
    if not row:
        return ZoneSnapshot(0, 0, 0)
    return ZoneSnapshot(_i(row.available), _i(row.reserved), _ms(row.last_event_ms))


def _load_p(session: Any, ps: PreparedCQL, product_id: str) -> ProductAgg:
    row = session.execute(ps.sel_p, (product_id,)).one()
    if not row:
        return ProductAgg(0, 0)
    return ProductAgg(_i(row.total_available), _i(row.total_reserved))


def _load_z(session: Any, ps: PreparedCQL, zone_id: str, product_id: str) -> ZoneSnapshot:
    row = session.execute(ps.sel_z, (zone_id, product_id)).one()
    if not row:
        return ZoneSnapshot(0, 0, 0)
    return ZoneSnapshot(_i(row.available), _i(row.reserved), _ms(row.last_event_ms))


def _load_order(session: Any, ps: PreparedCQL, order_id: str) -> Optional[OrderRow]:
    row = session.execute(ps.sel_order, (order_id,)).one()
    if not row:
        return None
    return OrderRow(str(row.status), _ms(row.last_event_ms), str(row.lines_json or "[]"))


def _stale(occurred_at: int, *lasts: int) -> bool:
    """True if event must be ignored (not strictly newer than all last_event_ms)."""
    return any(occurred_at <= last for last in lasts)


def _audit(
    ps: PreparedCQL,
    batch: BatchStatement,
    entity_type: str,
    entity_id: str,
    event: dict[str, Any],
) -> None:
    batch.add(
        ps.ins_history,
        (
            entity_type,
            entity_id,
            event["event_id"],
            event["event_type"],
            int(event["occurred_at"]),
            json.dumps(event, ensure_ascii=False),
        ),
    )


def _finish_batch(
    ps: PreparedCQL,
    batch: BatchStatement,
    event: dict[str, Any],
    entity_type: str,
    entity_id: str,
) -> None:
    now = datetime.now(timezone.utc)
    batch.add(ps.ins_processed, (event["event_id"], now))
    _audit(ps, batch, entity_type, entity_id, event)


def _apply_zone_delta(
    ps: PreparedCQL,
    batch: BatchStatement,
    product_id: str,
    zone_id: str,
    occurred_at: int,
    d_avail: int,
    d_res: int,
    pz: ZoneSnapshot,
    p: ProductAgg,
    zi: ZoneSnapshot,
    *,
    update_product_agg: bool = True,
) -> None:
    new_pz = ZoneSnapshot(
        pz.available + d_avail,
        pz.reserved + d_res,
        occurred_at,
    )
    new_p = ProductAgg(
        p.total_available + (d_avail if update_product_agg else 0),
        p.total_reserved + (d_res if update_product_agg else 0),
    )
    new_zi = ZoneSnapshot(
        zi.available + d_avail,
        zi.reserved + d_res,
        occurred_at,
    )
    batch.add(
        ps.upd_pz,
        (new_pz.available, new_pz.reserved, new_pz.last_event_ms, product_id, zone_id),
    )
    if update_product_agg:
        batch.add(
            ps.upd_p,
            (new_p.total_available, new_p.total_reserved, product_id),
        )
    batch.add(
        ps.upd_z,
        (new_zi.available, new_zi.reserved, new_zi.last_event_ms, zone_id, product_id),
    )


def _require_non_null_str(event: dict[str, Any], field: str) -> str:
    v = event.get(field)
    if v is None:
        raise ValueError(f"Missing required field: {field}")
    s = str(v).strip()
    if not s:
        raise ValueError(f"Empty required field: {field}")
    return s


def _require_positive_int(event: dict[str, Any], field: str) -> int:
    v = event.get(field)
    if v is None:
        raise ValueError(f"Missing required field: {field}")
    q = int(v)
    if q <= 0:
        raise ValueError(f"Invalid {field}: {q} (must be positive)")
    return q


def _require_non_negative_int(event: dict[str, Any], field: str) -> int:
    v = event.get(field)
    if v is None:
        raise ValueError(f"Missing required field: {field}")
    q = int(v)
    if q < 0:
        raise ValueError(f"Invalid {field}: {q} (must be non-negative)")
    return q


def build_batch(
    session: Any,
    ps: PreparedCQL,
    event: dict[str, Any],
) -> tuple[str, Optional[BatchStatement]]:
    """
    Returns (status, batch) where status in:
      - duplicate
      - stale
      - ok
    and batch is None for duplicate/stale.
    """
    event_id = str(event["event_id"])
    if session.execute(ps.sel_processed, (event_id,)).one():
        return "duplicate", None

    et = str(event["event_type"])
    occurred_at = int(event["occurred_at"])

    if et == "PRODUCT_RECEIVED":
        product_id = _require_non_null_str(event, "product_id")
        zone_id = _require_non_null_str(event, "zone_id")
        q = _require_positive_int(event, "quantity")

        pz = _load_pz(session, ps, product_id, zone_id)
        p = _load_p(session, ps, product_id)
        zi = _load_z(session, ps, zone_id, product_id)
        if _stale(occurred_at, pz.last_event_ms, zi.last_event_ms):
            return "stale", None

        batch = BatchStatement(batch_type=BatchType.LOGGED)
        _apply_zone_delta(ps, batch, product_id, zone_id, occurred_at, q, 0, pz, p, zi)
        _finish_batch(ps, batch, event, "product", product_id)
        return "ok", batch

    if et == "PRODUCT_SHIPPED":
        product_id = _require_non_null_str(event, "product_id")
        zone_id = _require_non_null_str(event, "zone_id")
        q = _require_positive_int(event, "quantity")

        pz = _load_pz(session, ps, product_id, zone_id)
        p = _load_p(session, ps, product_id)
        zi = _load_z(session, ps, zone_id, product_id)
        if _stale(occurred_at, pz.last_event_ms, zi.last_event_ms):
            return "stale", None
        if pz.available < q:
            raise ValueError(f"Insufficient available stock: need {q}, have {pz.available}")

        batch = BatchStatement(batch_type=BatchType.LOGGED)
        _apply_zone_delta(ps, batch, product_id, zone_id, occurred_at, -q, 0, pz, p, zi)
        _finish_batch(ps, batch, event, "product", product_id)
        return "ok", batch

    if et == "PRODUCT_MOVED":
        product_id = _require_non_null_str(event, "product_id")
        from_z = _require_non_null_str(event, "from_zone_id")
        to_z = _require_non_null_str(event, "to_zone_id")
        q = _require_positive_int(event, "quantity")

        pz_from = _load_pz(session, ps, product_id, from_z)
        pz_to = _load_pz(session, ps, product_id, to_z)
        zi_from = _load_z(session, ps, from_z, product_id)
        zi_to = _load_z(session, ps, to_z, product_id)
        if _stale(occurred_at, pz_from.last_event_ms, pz_to.last_event_ms, zi_from.last_event_ms, zi_to.last_event_ms):
            return "stale", None
        if pz_from.available < q:
            raise ValueError(f"Insufficient available stock in source zone: need {q}, have {pz_from.available}")

        p = _load_p(session, ps, product_id)
        batch = BatchStatement(batch_type=BatchType.LOGGED)
        _apply_zone_delta(
            ps, batch, product_id, from_z, occurred_at, -q, 0, pz_from, p, zi_from, update_product_agg=False
        )
        _apply_zone_delta(
            ps, batch, product_id, to_z, occurred_at, q, 0, pz_to, p, zi_to, update_product_agg=False
        )
        _finish_batch(ps, batch, event, "product", product_id)
        return "ok", batch

    if et == "PRODUCT_RESERVED":
        product_id = _require_non_null_str(event, "product_id")
        zone_id = _require_non_null_str(event, "zone_id")
        q = _require_positive_int(event, "quantity")

        pz = _load_pz(session, ps, product_id, zone_id)
        p = _load_p(session, ps, product_id)
        zi = _load_z(session, ps, zone_id, product_id)
        if _stale(occurred_at, pz.last_event_ms, zi.last_event_ms):
            return "stale", None
        if pz.available < q:
            raise ValueError(f"Insufficient available stock to reserve: need {q}, have {pz.available}")

        batch = BatchStatement(batch_type=BatchType.LOGGED)
        _apply_zone_delta(ps, batch, product_id, zone_id, occurred_at, -q, q, pz, p, zi)
        _finish_batch(ps, batch, event, "product", product_id)
        return "ok", batch

    if et == "PRODUCT_RELEASED":
        product_id = _require_non_null_str(event, "product_id")
        zone_id = _require_non_null_str(event, "zone_id")
        q = _require_positive_int(event, "quantity")

        pz = _load_pz(session, ps, product_id, zone_id)
        p = _load_p(session, ps, product_id)
        zi = _load_z(session, ps, zone_id, product_id)
        if _stale(occurred_at, pz.last_event_ms, zi.last_event_ms):
            return "stale", None
        if pz.reserved < q:
            raise ValueError(f"Insufficient reserved stock to release: need {q}, have {pz.reserved}")

        batch = BatchStatement(batch_type=BatchType.LOGGED)
        _apply_zone_delta(ps, batch, product_id, zone_id, occurred_at, q, -q, pz, p, zi)
        _finish_batch(ps, batch, event, "product", product_id)
        return "ok", batch

    if et == "INVENTORY_COUNTED":
        product_id = _require_non_null_str(event, "product_id")
        zone_id = _require_non_null_str(event, "zone_id")
        counted = _require_non_negative_int(event, "counted_quantity")

        pz = _load_pz(session, ps, product_id, zone_id)
        p = _load_p(session, ps, product_id)
        zi = _load_z(session, ps, zone_id, product_id)
        if _stale(occurred_at, pz.last_event_ms, zi.last_event_ms):
            return "stale", None

        d_avail = counted - pz.available
        batch = BatchStatement(batch_type=BatchType.LOGGED)
        new_pz = ZoneSnapshot(counted, pz.reserved, occurred_at)
        new_p = ProductAgg(p.total_available + d_avail, p.total_reserved)
        new_zi = ZoneSnapshot(counted, zi.reserved, occurred_at)
        batch.add(
            ps.upd_pz,
            (new_pz.available, new_pz.reserved, new_pz.last_event_ms, product_id, zone_id),
        )
        batch.add(
            ps.upd_p,
            (new_p.total_available, new_p.total_reserved, product_id),
        )
        batch.add(
            ps.upd_z,
            (new_zi.available, new_zi.reserved, new_zi.last_event_ms, zone_id, product_id),
        )
        _finish_batch(ps, batch, event, "product", product_id)
        return "ok", batch

    if et == "ORDER_CREATED":
        order_id = _require_non_null_str(event, "order_id")
        lines = event.get("order_lines")
        if not lines:
            raise ValueError("order_lines is required for ORDER_CREATED")

        existing = _load_order(session, ps, order_id)
        if existing:
            raise ValueError(f"Order already exists: {order_id}")

        zone_checks: list[int] = []
        line_totals: dict[tuple[str, str], int] = defaultdict(int)
        for line in lines:
            pid = str(line["product_id"])
            zid = str(line["zone_id"])
            qty = int(line["quantity"])
            if qty <= 0:
                raise ValueError(f"Invalid order line quantity: {qty}")
            line_totals[(pid, zid)] += qty

        snapshots: list[tuple[str, str, ZoneSnapshot, ProductAgg, ZoneSnapshot, int]] = []
        for (pid, zid), qty in line_totals.items():
            pz = _load_pz(session, ps, pid, zid)
            p = _load_p(session, ps, pid)
            zi = _load_z(session, ps, zid, pid)
            zone_checks.extend([pz.last_event_ms, zi.last_event_ms])
            if pz.available < qty:
                raise ValueError(
                    f"Insufficient available stock for order line {pid}/{zid}: need {qty}, have {pz.available}"
                )
            snapshots.append((pid, zid, pz, p, zi, qty))

        if _stale(occurred_at, *zone_checks):
            return "stale", None

        lines_payload = [
            {"product_id": pid, "zone_id": zid, "quantity": qty} for (pid, zid), qty in sorted(line_totals.items())
        ]
        lines_json = json.dumps(lines_payload, ensure_ascii=False)

        batch = BatchStatement(batch_type=BatchType.LOGGED)
        batch.add(ps.ins_order, (order_id, "CREATED", occurred_at, lines_json))
        for pid, zid, pz, p, zi, qty in snapshots:
            _apply_zone_delta(ps, batch, pid, zid, occurred_at, -qty, qty, pz, p, zi)
        _finish_batch(ps, batch, event, "order", order_id)
        return "ok", batch

    if et == "ORDER_COMPLETED":
        order_id = _require_non_null_str(event, "order_id")
        row = _load_order(session, ps, order_id)
        if not row:
            raise ValueError(f"Order not found: {order_id}")
        if row.status != "CREATED":
            raise ValueError(f"Order must be CREATED to complete, got {row.status}")
        if _stale(occurred_at, row.last_event_ms):
            return "stale", None

        lines_payload = json.loads(row.lines_json)
        line_totals: dict[tuple[str, str], int] = defaultdict(int)
        for line in lines_payload:
            pid = str(line["product_id"])
            zid = str(line["zone_id"])
            qty = int(line["quantity"])
            line_totals[(pid, zid)] += qty

        snapshots: list[tuple[str, str, ZoneSnapshot, ProductAgg, ZoneSnapshot, int]] = []
        for (pid, zid), qty in line_totals.items():
            pz = _load_pz(session, ps, pid, zid)
            p = _load_p(session, ps, pid)
            zi = _load_z(session, ps, zid, pid)
            if _stale(occurred_at, pz.last_event_ms, zi.last_event_ms):
                return "stale", None
            if pz.reserved < qty:
                raise ValueError(f"Insufficient reserved to complete line {pid}/{zid}: need {qty}, have {pz.reserved}")
            snapshots.append((pid, zid, pz, p, zi, qty))

        batch = BatchStatement(batch_type=BatchType.LOGGED)
        batch.add(ps.upd_order, ("COMPLETED", occurred_at, order_id))
        for pid, zid, pz, p, zi, qty in snapshots:
            _apply_zone_delta(ps, batch, pid, zid, occurred_at, 0, -qty, pz, p, zi)
        _finish_batch(ps, batch, event, "order", order_id)
        return "ok", batch

    raise ValueError(f"Unsupported event_type: {et}")
