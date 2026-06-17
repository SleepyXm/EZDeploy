"""Tests for endpoint and decorator detection in the AST scanner."""
 
import pytest
from pathlib import Path
 
from test_scanner import scan_file, functions_to_payload
 
FIXTURE = Path(__file__).parent / "fixtures" / "routes.py"
 
 
@pytest.fixture(scope="module")
def functions():
    return scan_file(FIXTURE, module="fixtures.routes")
 
 
@pytest.fixture(scope="module")
def by_name(functions):
    return {f.name: f for f in functions}
 
 
class TestEndpointDetection:
    def test_finds_all_endpoints(self, by_name):
        """All decorated route functions are found."""
        expected = {
            "list_users", "user_detail", "charge_user", "delete_user",
            "get_item", "create_item",
        }
        assert expected.issubset(by_name.keys())
 
    def test_plain_function_not_endpoint(self, by_name):
        """validate_email has no route decorator — http_methods must be empty."""
        fn = by_name["validate_email"]
        assert fn.http_methods == []
        assert fn.route_path is None
 
    def test_flask_get_route(self, by_name):
        """@app.route('/users', methods=['GET']) → GET, path=/users."""
        fn = by_name["list_users"]
        assert fn.http_methods == ["GET"]
        assert fn.route_path == "/users"
 
    def test_flask_multi_method_route(self, by_name):
        """@app.route(..., methods=['GET','POST']) → both methods captured."""
        fn = by_name["user_detail"]
        assert set(fn.http_methods) == {"GET", "POST"}
        assert fn.route_path == "/users/<int:user_id>"
 
    def test_flask_post_shorthand(self, by_name):
        """@app.post('/path') shorthand → POST."""
        fn = by_name["charge_user"]
        assert fn.http_methods == ["POST"]
        assert fn.route_path == "/users/<int:user_id>/charge"
 
    def test_flask_delete_shorthand(self, by_name):
        """@app.delete('/path') shorthand → DELETE."""
        fn = by_name["delete_user"]
        assert fn.http_methods == ["DELETE"]
 
    def test_fastapi_router_get(self, by_name):
        """@router.get('/path') → GET."""
        fn = by_name["get_item"]
        assert fn.http_methods == ["GET"]
        assert fn.route_path == "/items/{item_id}"
 
    def test_fastapi_router_post(self, by_name):
        """@router.post('/path') → POST."""
        fn = by_name["create_item"]
        assert fn.http_methods == ["POST"]
        assert fn.route_path == "/items"
 
    def test_endpoint_params_still_extracted(self, by_name):
        """Endpoint params are extracted the same as plain function params."""
        fn = by_name["create_item"]
        param_names = [p.name for p in fn.params]
        assert "name" in param_names
        assert "price" in param_names
        assert "tags" in param_names
 
    def test_endpoint_param_types(self, by_name):
        fn = by_name["create_item"]
        by_param = {p.name: p for p in fn.params}
        assert by_param["price"].annotation == "float"
 
    def test_endpoint_optional_param_default(self, by_name):
        fn = by_name["delete_user"]
        by_param = {p.name: p for p in fn.params}
        assert by_param["reason"].default == "None"
 
    def test_decorator_names_recorded(self, by_name):
        """Raw decorator names are stored for downstream use."""
        fn = by_name["list_users"]
        assert any("route" in d for d in fn.decorators)
 
    def test_payload_includes_http_methods(self, functions):
        """Serialised payload includes http_methods for endpoints."""
        payload = functions_to_payload(functions)
        charge = next(d for d in payload if d["name"] == "charge_user")
        assert charge.get("http_methods") == ["POST"]
 
    def test_payload_omits_http_for_plain(self, functions):
        """Plain functions don't have http_methods in the payload."""
        payload = functions_to_payload(functions)
        validate = next(d for d in payload if d["name"] == "validate_email")
        assert "http_methods" not in validate
 