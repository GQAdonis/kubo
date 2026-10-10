// Converts the subset of JSON Schema 2020-12 that mcp/contract uses into Zod 4
// source text. It fails loudly on any keyword it does not understand, so a
// contract change that the UI cannot represent stops the generator instead of
// silently weakening a schema.

const IGNORED = new Set([
  '$schema',
  '$id',
  'title',
  'description',
  'default',
  'examples',
  'contentEncoding',
  'contentMediaType',
]);

const KNOWN = new Set([
  ...IGNORED,
  'type',
  'enum',
  'const',
  'properties',
  'required',
  'additionalProperties',
  'items',
  'minimum',
  'maximum',
  'minLength',
  'maxLength',
  'pattern',
  'minItems',
  'maxItems',
  'uniqueItems',
  'format',
  '$ref',
  'oneOf',
  'allOf',
  '$defs',
]);

const q = (s) => JSON.stringify(s);

/**
 * @param {object} schema JSON Schema node
 * @param {(ref: string) => string} resolveRef maps a $ref string to a Zod identifier
 * @param {string} where location label for error messages
 * @returns {string} Zod expression
 */
export function toZod(schema, resolveRef, where = '#') {
  if (schema === true || (typeof schema === 'object' && Object.keys(schema).every((k) => IGNORED.has(k)))) {
    return 'z.unknown()';
  }
  for (const key of Object.keys(schema)) {
    if (!KNOWN.has(key)) throw new Error(`schema-to-zod: unsupported keyword "${key}" at ${where}`);
  }

  if (schema.$ref) {
    return resolveRef(schema.$ref);
  }
  if (schema.oneOf) {
    const parts = schema.oneOf.map((s, i) => toZod(s, resolveRef, `${where}/oneOf/${i}`));
    return `z.union([${parts.join(', ')}])`;
  }
  if (schema.const !== undefined) {
    return `z.literal(${q(schema.const)})`;
  }
  if (schema.enum) {
    if (!schema.enum.every((v) => typeof v === 'string')) {
      throw new Error(`schema-to-zod: non-string enum at ${where}`);
    }
    return `z.enum([${schema.enum.map(q).join(', ')}])`;
  }

  let expr;
  switch (schema.type) {
    case 'string':
      expr = stringExpr(schema, where);
      break;
    case 'integer':
      expr = `z.number().int()${numberChecks(schema)}`;
      break;
    case 'number':
      expr = `z.number()${numberChecks(schema)}`;
      break;
    case 'boolean':
      expr = 'z.boolean()';
      break;
    case 'array':
      expr = arrayExpr(schema, resolveRef, where);
      break;
    case 'object':
      expr = objectExpr(schema, resolveRef, where);
      break;
    case undefined:
      expr = 'z.unknown()';
      break;
    default:
      throw new Error(`schema-to-zod: unsupported type ${q(schema.type)} at ${where}`);
  }
  return expr;
}

function stringExpr(schema, where) {
  if (schema.format === 'date-time') {
    return 'z.iso.datetime({ offset: true })';
  }
  if (schema.format) throw new Error(`schema-to-zod: unsupported string format "${schema.format}" at ${where}`);
  let e = 'z.string()';
  if (schema.minLength !== undefined) e += `.min(${schema.minLength})`;
  if (schema.maxLength !== undefined) e += `.max(${schema.maxLength})`;
  if (schema.pattern !== undefined) e += `.regex(new RegExp(${q(schema.pattern)}))`;
  return e;
}

function numberChecks(schema) {
  let e = '';
  if (schema.minimum !== undefined) e += `.min(${schema.minimum})`;
  if (schema.maximum !== undefined) e += `.max(${schema.maximum})`;
  return e;
}

function arrayExpr(schema, resolveRef, where) {
  const item = schema.items ? toZod(schema.items, resolveRef, `${where}/items`) : 'z.unknown()';
  let e = `z.array(${item})`;
  if (schema.minItems !== undefined) e += `.min(${schema.minItems})`;
  if (schema.maxItems !== undefined) e += `.max(${schema.maxItems})`;
  if (schema.uniqueItems) e += `.refine((a) => new Set(a).size === a.length, { message: 'items must be unique' })`;
  return e;
}

function objectExpr(schema, resolveRef, where) {
  const props = schema.properties ?? {};
  const required = new Set(schema.required ?? []);
  for (const r of required) {
    if (!(r in props)) throw new Error(`schema-to-zod: required "${r}" has no property at ${where}`);
  }
  const keys = Object.keys(props);
  let e;
  if (keys.length === 0 && schema.additionalProperties !== false) {
    e = 'z.record(z.string(), z.unknown())';
  } else {
    const body = keys
      .map((k) => {
        const v = toZod(props[k], resolveRef, `${where}/properties/${k}`);
        return `${q(k)}: ${v}${required.has(k) ? '' : '.optional()'}`;
      })
      .join(', ');
    const ctor = schema.additionalProperties === false ? 'z.strictObject' : 'z.looseObject';
    e = `${ctor}({ ${body} })`;
  }
  if (schema.allOf) {
    e += allOfRefinements(schema.allOf, where);
  }
  return e;
}

// The only allOf shape the contract uses: "if <field> is one of <values> then
// <other field> must be absent".
function allOfRefinements(allOf, where) {
  let out = '';
  allOf.forEach((entry, i) => {
    const w = `${where}/allOf/${i}`;
    const cond = entry.if;
    const then = entry.then;
    const field = cond?.required?.[0];
    const values = cond?.properties?.[field]?.enum;
    const forbidden = then?.not?.required?.[0];
    const native = nativePromptRefinement(entry, w);
    if (native) {
      out += native;
      return;
    }
    if (!field || !values || !forbidden || Object.keys(entry).some((k) => !['if', 'then'].includes(k))) {
      throw new Error(`schema-to-zod: unsupported allOf shape at ${w}`);
    }
    out += `.refine((o) => !(${q(field)} in o && [${values.map(q).join(', ')}].includes(o[${q(field)}] as string)) || o[${q(forbidden)}] === undefined, { message: ${q(`${forbidden} must be absent when ${field} is ${values.join(' or ')}`)}, path: [${q(forbidden)}] })`;
  });
  return out;
}

// Second allOf shape: "if <flag> is const true then summary.class is const <c> and resolvers never
// contains <r>" (ConfirmationRequired.requires_native_prompt). Returns null for other shapes.
function nativePromptRefinement(entry, w) {
  const cond = entry.if;
  const then = entry.then;
  const flag = cond?.required?.[0];
  if (!flag || cond.properties?.[flag]?.const !== true || !then?.properties) return null;
  const cls = then.properties.summary?.properties?.class?.const;
  const banned = then.properties.resolvers?.not?.contains?.const;
  if (typeof cls !== 'string' || typeof banned !== 'string') {
    throw new Error(`schema-to-zod: unsupported allOf shape at ${w}`);
  }
  const msg = `${flag} requires summary.class ${cls} and no ${banned} resolver`;
  return `.refine((o) => o[${q(flag)}] !== true || (o.summary.class === ${q(cls)} && !o.resolvers.includes(${q(banned)})), { message: ${q(msg)}, path: [${q(flag)}] })`;
}

export function pascal(name) {
  return name
    .split(/[^A-Za-z0-9]+/)
    .filter(Boolean)
    .map((p) => p[0].toUpperCase() + p.slice(1))
    .join('');
}
