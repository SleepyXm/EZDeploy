from __future__ import annotations
 
from dataclasses import dataclass, field
from typing import Any, Literal
 
 
TestType = Literal["unit", "integration", "edge", "regression"]
 
 
@dataclass
class TestCase:
    id: str                             # snake_case identifier, unique within the function
    type: TestType
    description: str                    # one sentence: what this case verifies
 
    # --- inputs ---
    args: dict[str, Any] = field(default_factory=dict)     # positional/kw args by name
    env: dict[str, Any] = field(default_factory=dict)      # env vars, if relevant
 
    # --- expectations (mutually exclusive groups) ---
    expect_return: Any = None           # expected return value (None means "not checked")
    expect_exception: str | None = None # exception class name, e.g. "ValueError"
    expect_side_effects: list[str] = field(default_factory=list)
    # e.g. ["db.save called once", "email.send called with user_id=42"]
 
    # --- mocks ---
    mocks: list[MockSpec] = field(default_factory=list)
 
    # --- metadata ---
    tags: list[str] = field(default_factory=list)          # free-form, e.g. ["auth", "slow"]
    skip_reason: str | None = None      # if set, the case is marked pytest.mark.skip
 
 
@dataclass
class MockSpec:
    target: str         # dotted path to patch, e.g. "myapp.db.session.execute"
    return_value: Any = None
    side_effect: str | None = None      # exception class name or "raise <exc>"
 
 
@dataclass
class FunctionTestBank:
    function: str                           # plain function name
    module: str                             # dotted module, e.g. "myapp.billing"
    cases: list[TestCase] = field(default_factory=list)
    class_name: str | None = None           # set for methods
    route_path: str | None = None           # set for endpoints
    http_methods: list[str] = field(default_factory=list)  # e.g. ["POST"]"""
 
 
@dataclass
class ProjectTestBank:
    """Top-level container written to tests.json."""
    schema_version: str = "1.0"
    generator: str = "testgen"
    functions: list[FunctionTestBank] = field(default_factory=list)
 
 
# ---------------------------------------------------------------------------
# JSON round-trip helpers
# ---------------------------------------------------------------------------
 
import json
from dataclasses import asdict
 
 
def _clean(obj: Any) -> Any:
    """Recursively strip None values so JSON stays tidy."""
    if isinstance(obj, dict):
        return {k: _clean(v) for k, v in obj.items() if v is not None and v != [] and v != {}}
    if isinstance(obj, list):
        return [_clean(i) for i in obj]
    return obj
 
 
def bank_to_dict(bank: ProjectTestBank) -> dict[str, Any]:
    return _clean(asdict(bank))
 
 
def bank_to_json(bank: ProjectTestBank, indent: int = 2) -> str:
    return json.dumps(bank_to_dict(bank), indent=indent)
 
 
def bank_from_dict(d: dict[str, Any]) -> ProjectTestBank:
    functions = []
    for fb in d.get("functions", []):
        cases = []
        for c in fb.get("cases", []):
            mocks = [MockSpec(**m) for m in c.pop("mocks", [])]
            cases.append(TestCase(**{**c, "mocks": mocks}))
        functions.append(FunctionTestBank(**{**fb, "cases": cases}))
    return ProjectTestBank(
        schema_version=d.get("schema_version", "1.0"),
        generator=d.get("generator", "testgen"),
        functions=functions,
    )
 
 
def bank_from_json(s: str) -> ProjectTestBank:
    return bank_from_dict(json.loads(s))
 