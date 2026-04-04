"""
Fixture: a small billing module that covers the common patterns
the scanner needs to handle.
"""
 
from __future__ import annotations
 
from decimal import Decimal
from typing import Optional
 
 
class BillingError(Exception):
    pass
 
 
class PaymentService:
 
    def __init__(self, gateway_url: str, timeout: int = 30) -> None:
        self.gateway_url = gateway_url
        self.timeout = timeout
 
    def charge(
        self,
        user_id: int,
        amount: Decimal,
        currency: str = "GBP",
        idempotency_key: Optional[str] = None,
    ) -> bool:
        """Charge a user's saved payment method. Returns True on success."""
        if amount <= 0:
            raise BillingError("Amount must be positive")
        # ... real implementation ...
        return True
 
    def refund(self, transaction_id: str, reason: str = "customer_request") -> dict:
        """Refund a transaction. Returns the gateway response dict."""
        return {"status": "refunded", "transaction_id": transaction_id}
 
 
def calculate_tax(amount: Decimal, rate: float, region: str) -> Decimal:
    """Calculate tax for a given amount, rate (0-1), and region code."""
    if not 0 <= rate <= 1:
        raise ValueError(f"rate must be between 0 and 1, got {rate}")
    return (amount * Decimal(str(rate))).quantize(Decimal("0.01"))
 
 
async def fetch_invoice(invoice_id: str, include_items: bool = False) -> dict:
    """Async: fetch invoice data from the billing service."""

 