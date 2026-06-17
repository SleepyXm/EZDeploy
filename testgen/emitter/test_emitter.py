"""
Tests for the pytest emitter.
 
These tests verify that the emitter produces syntactically valid,
correctly structured Python from a given ProjectTestBank — entirely
without calling the LLM.
"""
 
from __future__ import annotations
 
import ast
import textwrap
from pathlib import Path
 
import pytest
 
from generator.schema import (
    FunctionTestBank,
    MockSpec,
    ProjectTestBank,
    TestCase,
)
from pytest_emitter import (
    PytestEmitter,
    _render_value,
    _render_test_case,
    _render_test_class,
    _render_file,
)
 
 
# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
 
def _is_valid_python(source: str) -> bool:
    """Return True if *source* parses without error."""
    try:
        ast.parse(source)
        return True
    except SyntaxError:
        return False
 
 
def _make_bank(*functions: FunctionTestBank) -> ProjectTestBank:
    return ProjectTestBank(functions=list(functions))
 
 
def _simple_fn(
    name: str = "my_func",
    module: str = "myapp.utils",
    cases: list[TestCase] | None = None,
) -> FunctionTestBank:
    return FunctionTestBank(
        function=name,
        module=module,
        cases=cases or [_happy_case()],
    )
 
 
def _happy_case(**kwargs) -> TestCase:
    defaults = dict(
        id="happy_path",
        type="unit",
        description="Returns True for valid inputs.",
        args={"x": 1, "y": "hello"},
        expect_return=True,
    )
    return TestCase(**{**defaults, **kwargs})
 
 
# ---------------------------------------------------------------------------
# _render_value
# ---------------------------------------------------------------------------
 
class TestRenderValue:
    def test_none(self):
        assert _render_value(None) == "None"
 
    def test_true(self):
        assert _render_value(True) == "True"
 
    def test_false(self):
        assert _render_value(False) == "False"
 
    def test_int(self):
        assert _render_value(42) == "42"
 
    def test_float(self):
        assert _render_value(3.14) == "3.14"
 
    def test_string_simple(self):
        assert _render_value("hello") == "'hello'"
 
    def test_string_with_single_quote(self):
        rendered = _render_value("it's")
        # Must produce valid Python
        assert _is_valid_python(f"x = {rendered}")
 
    def test_list(self):
        assert _render_value([1, 2, 3]) == "[1, 2, 3]"
 
    def test_nested_list(self):
        result = _render_value([1, [2, 3]])
        assert _is_valid_python(f"x = {result}")
 
    def test_dict(self):
        result = _render_value({"a": 1})
        assert _is_valid_python(f"x = {result}")
 
    def test_empty_list(self):
        assert _render_value([]) == "[]"
 
    def test_empty_dict(self):
        assert _render_value({}) == "{}"
 
 
# ---------------------------------------------------------------------------
# _render_test_case
# ---------------------------------------------------------------------------
 
class TestRenderTestCase:
    def test_happy_path_valid_python(self):
        fn = _simple_fn()
        case = _happy_case()
        rendered = _render_test_case(fn, case)
        # Wrap in a class to make it a complete parseable unit
        assert _is_valid_python(f"import pytest\nclass T:\n{rendered}")
 
    def test_method_name_uses_case_id(self):
        fn = _simple_fn()
        case = _happy_case(id="zero_input")
        rendered = _render_test_case(fn, case)
        assert "def test_zero_input(self):" in rendered
 
    def test_docstring_included(self):
        fn = _simple_fn()
        case = _happy_case(description="Verifies that X works.")
        rendered = _render_test_case(fn, case)
        assert "Verifies that X works." in rendered
 
    def test_assert_return_value(self):
        fn = _simple_fn()
        case = _happy_case(expect_return=42)
        rendered = _render_test_case(fn, case)
        assert "assert result == 42" in rendered
 
    def test_no_assert_when_return_is_none(self):
        fn = _simple_fn()
        case = _happy_case(expect_return=None)
        rendered = _render_test_case(fn, case)
        assert "assert result ==" not in rendered
 
    def test_expect_exception(self):
        fn = _simple_fn()
        case = _happy_case(expect_return=None, expect_exception="ValueError")
        rendered = _render_test_case(fn, case)
        assert "pytest.raises(ValueError)" in rendered
        assert "assert result ==" not in rendered
 
    def test_skip_decorator(self):
        fn = _simple_fn()
        case = _happy_case(skip_reason="not implemented yet")
        rendered = _render_test_case(fn, case)
        assert "@pytest.mark.skip" in rendered
        assert "not implemented yet" in rendered
 
    def test_type_marker(self):
        fn = _simple_fn()
        case = _happy_case(type="edge")
        rendered = _render_test_case(fn, case)
        assert "@pytest.mark.edge" in rendered
 
    def test_call_uses_class_init_for_methods(self):
        fn = FunctionTestBank(
            function="charge",
            module="myapp.billing",
            class_name="PaymentService",
            cases=[],
        )
        case = _happy_case(args={"amount": 10.0})
        rendered = _render_test_case(fn, case)
        assert "PaymentService().charge(" in rendered
 
    def test_call_plain_function(self):
        fn = _simple_fn(name="calculate_tax")
        case = _happy_case(args={"rate": 0.2})
        rendered = _render_test_case(fn, case)
        assert "calculate_tax(" in rendered
        assert "()." not in rendered
 
    def test_single_mock_valid_python(self):
        fn = _simple_fn()
        case = _happy_case(
            mocks=[MockSpec(target="myapp.db.get", return_value={"id": 1})],
        )
        rendered = _render_test_case(fn, case)
        assert _is_valid_python(f"import pytest\nfrom unittest.mock import patch\nclass T:\n{rendered}")
        assert "patch(" in rendered
 
    def test_mock_with_side_effect(self):
        fn = _simple_fn()
        case = _happy_case(
            expect_return=None,
            expect_exception="RuntimeError",
            mocks=[MockSpec(target="myapp.db.get", side_effect="RuntimeError")],
        )
        rendered = _render_test_case(fn, case)
        assert "side_effect=RuntimeError()" in rendered
 
    def test_multiple_mocks_valid_python(self):
        """Multiple mocks must produce valid nested with-blocks."""
        fn = _simple_fn()
        case = _happy_case(
            mocks=[
                MockSpec(target="myapp.db.get", return_value=1),
                MockSpec(target="myapp.cache.set", return_value=None),
            ],
        )
        rendered = _render_test_case(fn, case)
        assert _is_valid_python(
            f"import pytest\nfrom unittest.mock import patch\nclass T:\n{rendered}"
        )
 
    def test_side_effects_as_comments(self):
        fn = _simple_fn()
        case = _happy_case(expect_side_effects=["db.save called once"])
        rendered = _render_test_case(fn, case)
        assert "# assert: db.save called once" in rendered
 
 
# ---------------------------------------------------------------------------
# _render_test_class
# ---------------------------------------------------------------------------
 
class TestRenderTestClass:
    def test_class_name_format(self):
        fn = _simple_fn(name="calculate_tax")
        rendered = _render_test_class(fn)
        assert "class Test_calculate_tax:" in rendered
 
    def test_method_class_prefix(self):
        fn = FunctionTestBank(
            function="charge",
            module="myapp.billing",
            class_name="PaymentService",
            cases=[_happy_case()],
        )
        rendered = _render_test_class(fn)
        assert "class Test_PaymentService_charge:" in rendered
 
    def test_endpoint_docstring(self):
        fn = FunctionTestBank(
            function="list_users",
            module="myapp.views",
            route_path="/users",
            http_methods=["GET"],
            cases=[_happy_case()],
        )
        rendered = _render_test_class(fn)
        assert "/users" in rendered
        assert "GET" in rendered
 
    def test_all_cases_rendered(self):
        fn = _simple_fn(cases=[
            _happy_case(id="case_a"),
            _happy_case(id="case_b"),
            _happy_case(id="case_c"),
        ])
        rendered = _render_test_class(fn)
        assert "test_case_a" in rendered
        assert "test_case_b" in rendered
        assert "test_case_c" in rendered
 
    def test_valid_python(self):
        fn = _simple_fn(cases=[_happy_case(), _happy_case(id="edge_empty", args={})])
        rendered = _render_test_class(fn)
        assert _is_valid_python(f"import pytest\n{rendered}")
 
 
# ---------------------------------------------------------------------------
# _render_file
# ---------------------------------------------------------------------------
 
class TestRenderFile:
    def test_header_comment(self):
        rendered = _render_file([_simple_fn()])
        assert "Auto-generated by testgen" in rendered
 
    def test_imports_included(self):
        rendered = _render_file([_simple_fn(module="myapp.utils", name="my_func")])
        assert "from myapp.utils import my_func" in rendered
 
    def test_init_not_imported(self):
        """__init__ cannot be imported from a module — must be excluded."""
        fn = FunctionTestBank(
            function="__init__",
            module="myapp.billing",
            class_name="PaymentService",
            cases=[_happy_case()],
        )
        rendered = _render_file([fn])
        assert "import __init__" not in rendered
 
    def test_multiple_modules_separate_imports(self):
        fns = [
            _simple_fn(name="foo", module="myapp.a"),
            _simple_fn(name="bar", module="myapp.b"),
        ]
        rendered = _render_file(fns)
        assert "from myapp.a import foo" in rendered
        assert "from myapp.b import bar" in rendered
 
    def test_same_module_single_import_line(self):
        fns = [
            _simple_fn(name="foo", module="myapp.utils"),
            _simple_fn(name="bar", module="myapp.utils"),
        ]
        rendered = _render_file(fns)
        import_lines = [l for l in rendered.splitlines() if l.startswith("from myapp.utils")]
        assert len(import_lines) == 1
        assert "foo" in import_lines[0]
        assert "bar" in import_lines[0]
 
    def test_output_is_valid_python(self):
        fns = [
            _simple_fn(name="foo", module="myapp.utils"),
            FunctionTestBank(
                function="charge",
                module="myapp.billing",
                class_name="PaymentService",
                cases=[
                    _happy_case(id="happy"),
                    _happy_case(
                        id="raises",
                        expect_return=None,
                        expect_exception="ValueError",
                        mocks=[MockSpec(target="myapp.billing.db.get", return_value=None)],
                    ),
                ],
            ),
        ]
        rendered = _render_file(fns)
        assert _is_valid_python(rendered), f"Invalid Python:\n{rendered}"
 
 
# ---------------------------------------------------------------------------
# PytestEmitter (file I/O)
# ---------------------------------------------------------------------------
 
class TestPytestEmitter:
    def test_creates_output_dir(self, tmp_path):
        bank = _make_bank(_simple_fn())
        emitter = PytestEmitter(output_dir=tmp_path / "out" / "nested")
        emitter.emit(bank)
        assert (tmp_path / "out" / "nested").is_dir()
 
    def test_one_file_per_module(self, tmp_path):
        bank = _make_bank(
            _simple_fn(name="foo", module="myapp.a"),
            _simple_fn(name="bar", module="myapp.b"),
        )
        emitter = PytestEmitter(output_dir=tmp_path)
        written = emitter.emit(bank)
        stems = {p.name for p in written}
        assert "test_myapp_a.py" in stems
        assert "test_myapp_b.py" in stems
 
    def test_one_file_mode(self, tmp_path):
        bank = _make_bank(
            _simple_fn(name="foo", module="myapp.a"),
            _simple_fn(name="bar", module="myapp.b"),
        )
        emitter = PytestEmitter(output_dir=tmp_path, one_file=True)
        written = emitter.emit(bank)
        assert len(written) == 1
        assert written[0].name == "test_all.py"
 
    def test_written_file_is_valid_python(self, tmp_path):
        bank = _make_bank(_simple_fn())
        emitter = PytestEmitter(output_dir=tmp_path)
        written = emitter.emit(bank)
        for p in written:
            source = p.read_text()
            assert _is_valid_python(source), f"Invalid Python in {p}:\n{source}"
 
    def test_returns_list_of_paths(self, tmp_path):
        bank = _make_bank(_simple_fn())
        emitter = PytestEmitter(output_dir=tmp_path)
        result = emitter.emit(bank)
        assert isinstance(result, list)
        assert all(isinstance(p, Path) for p in result)
 
    def test_empty_bank_writes_nothing(self, tmp_path):
        bank = ProjectTestBank()
        emitter = PytestEmitter(output_dir=tmp_path)
        written = emitter.emit(bank)
        assert written == []
 