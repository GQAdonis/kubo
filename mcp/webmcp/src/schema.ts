import type { JsonSchema } from './types.js';

/**
 * A small validator for the JSON Schema subset the contract's tool inputs use: type,
 * enum, pattern, min/max length and value, items, properties, required and
 * additionalProperties:false. Agent input crosses a trust boundary, so executors
 * validate it before they touch the RPC client. Unknown keywords are ignored; the
 * conformance test checks that the subset covers every generated schema.
 *
 * Input is parsed JSON from the host, so it can carry own properties named `__proto__`,
 * `constructor` or `prototype`. They are rejected at any depth, every property lookup uses
 * own-property checks, and the validated copy has no prototype to pollute.
 */
export interface ValidationResult {
  ok: boolean;
  errors: string[];
  /** The input with schema defaults applied. Present when ok. */
  value?: Record<string, unknown>;
}

const MAX_ERRORS = 5;
const FORBIDDEN_KEYS = new Set(['__proto__', 'constructor', 'prototype']);
const has = (o: object, k: string): boolean => Object.prototype.hasOwnProperty.call(o, k);

function typeOf(v: unknown): string {
  if (v === null) return 'null';
  if (Array.isArray(v)) return 'array';
  return typeof v;
}

function check(schema: JsonSchema, value: unknown, path: string, errors: string[]): unknown {
  if (errors.length >= MAX_ERRORS) return value;
  const type = schema.type as string | undefined;
  const actual = typeOf(value);
  if (type) {
    const okType =
      type === 'integer'
        ? typeof value === 'number' && Number.isInteger(value)
        : type === 'number'
          ? typeof value === 'number' && Number.isFinite(value)
          : actual === type;
    if (!okType) {
      errors.push(`${path}: expected ${type}, got ${actual}`);
      return value;
    }
  }
  if (Array.isArray(schema.enum) && !schema.enum.includes(value)) {
    errors.push(`${path}: must be one of ${schema.enum.map((e) => JSON.stringify(e)).join(', ')}`);
  }
  if (typeof value === 'string') {
    if (typeof schema.minLength === 'number' && value.length < schema.minLength) errors.push(`${path}: too short`);
    if (typeof schema.maxLength === 'number' && value.length > schema.maxLength) errors.push(`${path}: too long`);
    if (typeof schema.pattern === 'string' && !new RegExp(schema.pattern).test(value)) {
      errors.push(`${path}: does not match the required format`);
    }
  }
  if (typeof value === 'number') {
    if (typeof schema.minimum === 'number' && value < schema.minimum) errors.push(`${path}: below ${schema.minimum}`);
    if (typeof schema.maximum === 'number' && value > schema.maximum) errors.push(`${path}: above ${schema.maximum}`);
  }
  if (Array.isArray(value)) {
    if (typeof schema.minItems === 'number' && value.length < schema.minItems) errors.push(`${path}: too few items`);
    if (typeof schema.maxItems === 'number' && value.length > schema.maxItems) errors.push(`${path}: too many items`);
    if (schema.items && typeof schema.items === 'object') {
      return value.map((item, i) => check(schema.items as JsonSchema, item, `${path}[${i}]`, errors));
    }
    return value;
  }
  if (actual === 'object') {
    const obj = value as Record<string, unknown>;
    const props = (schema.properties ?? {}) as Record<string, JsonSchema>;
    const out: Record<string, unknown> = Object.create(null);
    for (const key of Object.keys(obj)) {
      if (FORBIDDEN_KEYS.has(key)) {
        errors.push(`${path}: property name ${key} is not allowed`);
      } else if (has(props, key)) {
        out[key] = check(props[key]!, obj[key], `${path}.${key}`, errors);
      } else if (schema.additionalProperties === false) {
        errors.push(`${path}: unknown property ${key}`);
      } else {
        out[key] = obj[key];
      }
    }
    for (const req of (schema.required ?? []) as string[]) {
      if (!has(obj, req)) errors.push(`${path}: missing required property ${req}`);
    }
    for (const [key, sub] of Object.entries(props)) {
      if (!has(obj, key) && has(sub, 'default')) out[key] = sub.default;
    }
    return out;
  }
  return value;
}

export function validateInput(schema: JsonSchema, input: unknown): ValidationResult {
  const errors: string[] = [];
  // A host may send no arguments for a tool with an empty input object.
  const value = input === undefined || input === null ? {} : input;
  const out = check(schema, value, 'input', errors);
  return errors.length === 0
    ? { ok: true, errors, value: out as Record<string, unknown> }
    : { ok: false, errors };
}
