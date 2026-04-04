import pytest
from pathlib import Path
 
from scanner.ast_scanner import scan_file
from scanner.serialiser import functions_to_payload
 
FIXTURE = Path(__file__).parent / "fixtures" / "billing.py"
 
 
@pytest.fixture(scope="module")
def functions():
    return scan_file(FIXTURE, module="fixtures.billing")
 
 
@pytest.fixture(scope="module")
def payload(functions):
    return functions_to_payload(functions)
 
 
class TestScanFile:
    def test_finds_expected_functions(self, functions):
        """Scanner finds all public callables in the fixture."""
        names = [f.name for f in functions]
        assert "charge" in names
        assert "refund" in names
        assert "calculate_tax" in names
        assert "fetch_invoice" in names
 
    def test_skips_private(self, functions):
        """Scanner does not include private/dunder methods (except __init__)."""
        names = [f.name for f in functions]
        assert not any(n.startswith("_") and n != "__init__" for n in names)
 
    def test_init_included(self, functions):
        """__init__ is included as it is the constructor."""
        names = [f.name for f in functions]
        assert "__init__" in names
 
    def test_params_extracted(self, functions):
        """charge() params are fully extracted with types and defaults."""
        charge = next(f for f in functions if f.name == "charge")
        param_names = [p.name for p in charge.params]
        assert "user_id" in param_names
        assert "amount" in param_names
        assert "currency" in param_names
        assert "idempotency_key" in param_names
 
    def test_param_types(self, functions):
        """Type annotations are preserved."""
        charge = next(f for f in functions if f.name == "charge")
        by_name = {p.name: p for p in charge.params}
        assert by_name["user_id"].annotation == "int"
        assert by_name["amount"].annotation == "Decimal"
 
    def test_param_defaults(self, functions):
        """Default values are captured."""
        charge = next(f for f in functions if f.name == "charge")
        by_name = {p.name: p for p in charge.params}
        assert by_name["currency"].default == "'GBP'"
 
    def test_return_annotation(self, functions):
        """Return annotations are captured."""
        charge = next(f for f in functions if f.name == "charge")
        assert charge.return_annotation == "bool"
 
    def test_docstring(self, functions):
        """Docstrings are extracted and stripped."""
        charge = next(f for f in functions if f.name == "charge")
        assert charge.docstring and "Charge" in charge.docstring
 
    def test_async_flag(self, functions):
        """Async functions are flagged."""
        fetch = next(f for f in functions if f.name == "fetch_invoice")
        assert fetch.is_async is True
 
    def test_method_class_name(self, functions):
        """Methods carry their class name."""
        charge = next(f for f in functions if f.name == "charge")
        assert charge.class_name == "PaymentService"
        assert charge.is_method is True
 
    def test_plain_function_no_class(self, functions):
        """Top-level functions have no class_name."""
        tax = next(f for f in functions if f.name == "calculate_tax")
        assert tax.class_name is None
        assert tax.is_method is False
 
 
class TestSerialiser:
    def test_payload_is_list_of_dicts(self, payload):
        assert isinstance(payload, list)
        assert all(isinstance(d, dict) for d in payload)
 
    def test_required_keys_present(self, payload):
        for d in payload:
            assert "name" in d
            assert "module" in d
            assert "params" in d
 
    def test_no_endpoint_keys_for_plain_functions(self, payload):
        tax = next(d for d in payload if d["name"] == "calculate_tax")
        assert "http_methods" not in tax
        assert "route_path" not in tax