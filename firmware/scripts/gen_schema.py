#!/usr/bin/env python3
"""Generate cJSON validators directly from the device JSON Schema."""
import argparse
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def literal(value):
    return json.dumps(value, ensure_ascii=True)


def generate(schema):
    definitions = schema["$defs"]
    functions = []
    declarations = []

    def validator(name, spec):
        supported = {"$ref", "const", "enum", "type", "description", "additionalProperties",
                     "required", "properties", "items", "maxItems", "uniqueItems", "minLength",
                     "maxLength", "pattern", "minimum", "maximum"}
        unsupported = set(spec) - supported
        if unsupported:
            raise ValueError(f"Unsupported schema keywords in {name}: {sorted(unsupported)}")
        declarations.append(f"inline bool {name}(const cJSON* value);")
        lines = [f"inline bool {name}(const cJSON* value) {{"]
        if "$ref" in spec:
            lines.append(f"    return {spec['$ref'].split('/')[-1]}(value);")
        elif "const" in spec or "enum" in spec:
            values = spec.get("enum", [spec.get("const")])
            checks = []
            for item in values:
                if isinstance(item, str):
                    checks.append(f"StringEquals(value, {literal(item)})")
                elif isinstance(item, (int, float)):
                    checks.append(f"(cJSON_IsNumber(value) && value->valuedouble == {item})")
                else:
                    raise ValueError(f"Unsupported constant: {item}")
            lines.append("    return " + " || ".join(checks) + ";")
        elif spec["type"] == "object":
            lines.append("    if (!cJSON_IsObject(value) || !UniqueKeys(value)) return false;")
            for key in spec.get("required", []):
                lines.append(f"    if (!Item(value, {literal(key)})) return false;")
            properties = spec.get("properties", {})
            for key, child in properties.items():
                child_name = f"{name}_{key}"
                validator(child_name, child)
                lines.append(f"    if (auto child = Item(value, {literal(key)}); child && !{child_name}(child)) return false;")
            if spec.get("additionalProperties") is False:
                allowed = " && ".join(f"std::strcmp(child->string, {literal(key)}) != 0" for key in properties)
                lines.append("    for (auto child = value->child; child; child = child->next) {")
                lines.append(f"        if ({allowed or 'true'}) return false;")
                lines.append("    }")
            lines.append("    return true;")
        elif spec["type"] == "array":
            validator(name + "_item", spec["items"])
            lines.append("    if (!cJSON_IsArray(value)) return false;")
            if "maxItems" in spec:
                lines.append(f"    if (cJSON_GetArraySize(value) > {spec['maxItems']}) return false;")
            lines.append("    for (auto child = value->child; child; child = child->next) {")
            lines.append(f"        if (!{name}_item(child)) return false;")
            if spec.get("uniqueItems"):
                lines.append("        for (auto other = child->next; other; other = other->next) {")
                lines.append("            if (cJSON_Compare(child, other, true)) return false;")
                lines.append("        }")
            lines.append("    }")
            lines.append("    return true;")
        elif spec["type"] == "string":
            lines.append("    if (!cJSON_IsString(value)) return false;")
            if "minLength" in spec:
                lines.append(f"    if (Utf8Length(value->valuestring) < {spec['minLength']}) return false;")
            if "maxLength" in spec:
                lines.append(f"    if (Utf8Length(value->valuestring) > {spec['maxLength']}) return false;")
            if "pattern" in spec:
                patterns = {
                    "^[0-9a-f]{2}(:[0-9a-f]{2}){5}$": "Mac",
                    "^#[0-9a-fA-F]{6}$": "Color",
                    "^[a-z0-9_-]{1,32}$": "Skin",
                }
                lines.append(f"    return Match{patterns[spec['pattern']]}(value->valuestring);")
            else:
                lines.append("    return true;")
        elif spec["type"] == "number":
            lines.append("    if (!cJSON_IsNumber(value) || !std::isfinite(value->valuedouble)) return false;")
            for constraint, operator in [("minimum", "<"), ("maximum", ">")]:
                if constraint in spec:
                    lines.append(f"    if (value->valuedouble {operator} {spec[constraint]}) return false;")
            lines.append("    return true;")
        else:
            raise ValueError(f"Unsupported schema: {spec}")
        lines.append("}")
        functions.append("\n".join(lines))

    for name, spec in definitions.items():
        validator(name, spec)
    result = "// Generated from protocol/palpable.schema.json; do not edit.\n#pragma once\n\n"
    result += '#include "palpable_schema_support.h"\n\nnamespace palpable::schema {\n'
    result += "\n".join(declarations) + "\n\n" + "\n\n".join(functions)
    result += "\n\ninline bool Message(const cJSON* value) {\n    return "
    result += " ||\n           ".join(ref["$ref"].split("/")[-1] + "(value)" for ref in schema["oneOf"])
    return result + ";\n}\n}  // namespace palpable::schema\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(generate(json.loads((ROOT / "protocol/palpable.schema.json").read_text())))


if __name__ == "__main__":
    main()
