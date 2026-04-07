"""
Fixture: simulates a small Flask + FastAPI-style app.
Tests that the scanner correctly detects route decorators,
HTTP methods, and route paths without importing Flask/FastAPI.
"""
 
from __future__ import annotations
 
from typing import Optional
 
 
# ---- Fake decorator stubs (no Flask/FastAPI install needed) ----
 
class _FakeApp:
    def route(self, path, methods=None):
        def dec(f): return f
        return dec
    def get(self, path):
        def dec(f): return f
        return dec
    def post(self, path):
        def dec(f): return f
        return dec
    def delete(self, path):
        def dec(f): return f
        return dec
 
app = _FakeApp()
 
class _FakeRouter:
    def get(self, path):
        def dec(f): return f
        return dec
    def post(self, path):
        def dec(f): return f
        return dec
 
router = _FakeRouter()
 
 
# ---- Flask-style endpoints ----
 
@app.route("/users", methods=["GET"])
def list_users():
    """Return all users."""
    return []
 
 
@app.route("/users/<int:user_id>", methods=["GET", "POST"])
def user_detail(user_id: int):
    """Get or update a single user."""
    return {}
 
 
@app.post("/users/<int:user_id>/charge")
def charge_user(user_id: int, amount: float, currency: str = "GBP"):
    """Charge a user's payment method."""
    return {"charged": True}
 
 
@app.delete("/users/<int:user_id>")
def delete_user(user_id: int, reason: Optional[str] = None):
    """Delete a user account."""
    return {"deleted": True}
 
 
# ---- FastAPI-style router endpoints ----
 
@router.get("/items/{item_id}")
def get_item(item_id: str, include_metadata: bool = False) -> dict:
    """Fetch a single item by ID."""
    return {}
 
 
@router.post("/items")
def create_item(name: str, price: float, tags: list[str] | None = None) -> dict:
    """Create a new item."""
    return {}
 
 
# ---- Plain function (should NOT be tagged as endpoint) ----
 
def validate_email(email: str) -> bool:
    """Validate email format. Returns True if valid."""
    return "@" in email