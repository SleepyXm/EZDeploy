from __future__ import annotations
 
import ast
import textwrap
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any
 
 
# ---------------------------------------------------------------------------
# Data model
# ---------------------------------------------------------------------------
 
@dataclass
class ParamInfo:
    name: str
    annotation: str | None = None       # e.g. "int", "str | None", "list[str]"
    default: str | None = None          # repr of the default value, if any
    kind: str = "positional"            # positional | keyword_only | var_positional | var_keyword
 
 
@dataclass
class FunctionInfo:
    name: str
    module: str                         # dotted module path, e.g. "myapp.billing"
    source_file: str                    # absolute path
    lineno: int
    is_method: bool = False
    class_name: str | None = None
    decorators: list[str] = field(default_factory=list)
    params: list[ParamInfo] = field(default_factory=list)
    return_annotation: str | None = None
    docstring: str | None = None
    is_async: bool = False
    # Endpoint-specific extras (populated when a web framework decorator is found)
    http_methods: list[str] = field(default_factory=list)
    route_path: str | None = None
    framework: str | None = None        # "flask" | "fastapi" | "django"
 
 
# ---------------------------------------------------------------------------
# Annotation unparsing helpers
# ---------------------------------------------------------------------------
 
def _unparse_annotation(node: ast.expr | None) -> str | None:
    """Turn an annotation AST node back into a readable string."""
    if node is None:
        return None
    try:
        return ast.unparse(node)
    except Exception:
        return None
 
 
def _unparse_default(node: ast.expr | None) -> str | None:
    if node is None:
        return None
    try:
        return ast.unparse(node)
    except Exception:
        return repr(node)
 
 
# ---------------------------------------------------------------------------
# Decorator parsing
# ---------------------------------------------------------------------------
 
# Decorator name patterns that indicate a web route
_ROUTE_DECORATORS = {
    # Flask / Quart
    "route", "get", "post", "put", "patch", "delete",
    # FastAPI
    "app.get", "app.post", "app.put", "app.patch", "app.delete",
    "router.get", "router.post", "router.put", "router.patch", "router.delete",
    # Django (django-ninja / drf)
    "api.get", "api.post", "api.put", "api.patch", "api.delete",
}
 
_HTTP_VERB_DECORATORS = {"get", "post", "put", "patch", "delete"}
 
 
def _decorator_name(node: ast.expr) -> str:
    """Return a flat string name for a decorator expression."""
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Attribute):
        return f"{_decorator_name(node.value)}.{node.attr}"
    if isinstance(node, ast.Call):
        return _decorator_name(node.func)
    return ""
 
 
def _parse_decorators(
    decorator_list: list[ast.expr],
) -> tuple[list[str], list[str], str | None, str | None]:
    """
    Returns (decorator_names, http_methods, route_path, framework).
    """
    names: list[str] = []
    http_methods: list[str] = []
    route_path: str | None = None
    framework: str | None = None
 
    for dec in decorator_list:
        name = _decorator_name(dec)
        names.append(name)
 
        leaf = name.split(".")[-1].lower()

        # Match on the leaf name OR any suffix of the dotted name
        is_route = (
            leaf in _HTTP_VERB_DECORATORS
            or leaf == "route"
            or name.lower() in _ROUTE_DECORATORS
            or any(name.lower().endswith(f".{r}") for r in _ROUTE_DECORATORS)
        )

        if is_route:
            # Extract path from first positional arg
            if isinstance(dec, ast.Call) and dec.args:
                try:
                    route_path = ast.literal_eval(dec.args[0])
                except Exception:
                    route_path = ast.unparse(dec.args[0])

            # Determine HTTP methods
            if leaf in _HTTP_VERB_DECORATORS:
                http_methods = [leaf.upper()]
            elif isinstance(dec, ast.Call):
                # Flask @app.route("/path", methods=["GET", "POST"])
                for kw in dec.keywords:
                    if kw.arg == "methods":
                        try:
                            http_methods = [m.upper() for m in ast.literal_eval(kw.value)]
                        except Exception:
                            pass
                if not http_methods:
                    http_methods = ["GET"]
 
            # Guess framework from decorator prefix
            prefix = name.split(".")[0].lower()
            if prefix in ("app", "blueprint", "bp"):
                framework = "flask"
            elif prefix in ("router",):
                framework = "fastapi"
            elif prefix in ("api",):
                framework = "django"
 
    return names, http_methods, route_path, framework
 
 
# ---------------------------------------------------------------------------
# Core visitor
# ---------------------------------------------------------------------------
 
class _FunctionVisitor(ast.NodeVisitor):
    def __init__(self, module: str, source_file: str) -> None:
        self.module = module
        self.source_file = source_file
        self.functions: list[FunctionInfo] = []
        self._class_stack: list[str] = []
 
    # ------------------------------------------------------------------
    def visit_ClassDef(self, node: ast.ClassDef) -> None:
        self._class_stack.append(node.name)
        self.generic_visit(node)
        self._class_stack.pop()
 
    # ------------------------------------------------------------------
    def _extract_params(
        self,
        args: ast.arguments,
    ) -> list[ParamInfo]:
        params: list[ParamInfo] = []
 
        # Build defaults mapping: last N positional args may have defaults
        n_args = len(args.args)
        n_defaults = len(args.defaults)
        defaults_offset = n_args - n_defaults
 
        for i, arg in enumerate(args.args):
            if arg.arg == "self" or arg.arg == "cls":
                continue
            default_node = args.defaults[i - defaults_offset] if i >= defaults_offset else None
            params.append(ParamInfo(
                name=arg.arg,
                annotation=_unparse_annotation(arg.annotation),
                default=_unparse_default(default_node),
                kind="positional",
            ))
 
        if args.vararg:
            params.append(ParamInfo(
                name=args.vararg.arg,
                annotation=_unparse_annotation(args.vararg.annotation),
                kind="var_positional",
            ))
 
        for i, arg in enumerate(args.kwonlyargs):
            default_node = args.kw_defaults[i]
            params.append(ParamInfo(
                name=arg.arg,
                annotation=_unparse_annotation(arg.annotation),
                default=_unparse_default(default_node),
                kind="keyword_only",
            ))
 
        if args.kwarg:
            params.append(ParamInfo(
                name=args.kwarg.arg,
                annotation=_unparse_annotation(args.kwarg.annotation),
                kind="var_keyword",
            ))
 
        return params
 
    # ------------------------------------------------------------------
    def _visit_funcdef(self, node: ast.FunctionDef | ast.AsyncFunctionDef) -> None:
        # Skip private/dunder methods unless they are __init__
        if node.name.startswith("_") and node.name != "__init__":
            self.generic_visit(node)
            return
 
        dec_names, http_methods, route_path, framework = _parse_decorators(node.decorator_list)
 
        docstring: str | None = None
        if (
            node.body
            and isinstance(node.body[0], ast.Expr)
            and isinstance(node.body[0].value, ast.Constant)
            and isinstance(node.body[0].value.value, str)
        ):
            docstring = textwrap.dedent(node.body[0].value.value).strip()
 
        info = FunctionInfo(
            name=node.name,
            module=self.module,
            source_file=self.source_file,
            lineno=node.lineno,
            is_method=bool(self._class_stack),
            class_name=self._class_stack[-1] if self._class_stack else None,
            decorators=dec_names,
            params=self._extract_params(node.args),
            return_annotation=_unparse_annotation(node.returns),
            docstring=docstring,
            is_async=isinstance(node, ast.AsyncFunctionDef),
            http_methods=http_methods,
            route_path=route_path,
            framework=framework,
        )
        self.functions.append(info)
        self.generic_visit(node)
 
    def visit_FunctionDef(self, node: ast.FunctionDef) -> None:
        self._visit_funcdef(node)
 
    def visit_AsyncFunctionDef(self, node: ast.AsyncFunctionDef) -> None:
        self._visit_funcdef(node)
 
 
# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------
 
def scan_file(path: Path, module: str | None = None) -> list[FunctionInfo]:
    """
    Parse *path* and return metadata for every public callable.
 
    *module* is the dotted module name (e.g. ``myapp.billing``).
    If omitted it is inferred from the path.
    """
    source = path.read_text(encoding="utf-8")
    try:
        tree = ast.parse(source, filename=str(path))
    except SyntaxError as exc:
        raise ValueError(f"Syntax error in {path}: {exc}") from exc
 
    if module is None:
        module = _path_to_module(path)
 
    visitor = _FunctionVisitor(module=module, source_file=str(path))
    visitor.visit(tree)
    return visitor.functions
 
 
def scan_project(root: Path) -> list[FunctionInfo]:
    """
    Recursively scan all ``*.py`` files under *root*, skipping test files,
    ``__pycache__``, and virtual-environment directories.
    """
    results: list[FunctionInfo] = []
    for path in sorted(root.rglob("*.py")):
        if _should_skip(path, root):
            continue
        try:
            results.extend(scan_file(path, module=_path_to_module(path, root)))
        except ValueError:
            # Syntax errors: skip gracefully
            pass
    return results
 
 
# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
 
_SKIP_DIRS = {"__pycache__", ".git", ".venv", "venv", "env", "node_modules", "dist", "build"}
 
 
def _should_skip(path: Path, root: Path) -> bool:
    rel = path.relative_to(root)
    parts = rel.parts
    if any(p in _SKIP_DIRS for p in parts):
        return True
    stem = path.stem
    # Skip test files and __init__ / __main__
    if stem.startswith("test_") or stem.endswith("_test"):
        return True
    if stem in ("__init__", "__main__", "conftest", "setup", "manage"):
        return True
    return False
 
 
def _path_to_module(path: Path, root: Path | None = None) -> str:
    """Convert ``src/myapp/billing.py`` → ``myapp.billing``."""
    try:
        if root:
            rel = path.relative_to(root)
        else:
            rel = path
        parts = list(rel.with_suffix("").parts)
        # Strip leading 'src' convention
        if parts and parts[0] == "src":
            parts = parts[1:]
        return ".".join(parts)
    except ValueError:
        return path.stem