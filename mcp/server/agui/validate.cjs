// Validates AG-UI events and RunAgentInput documents against the published
// @ag-ui/core zod schemas. Reads {"inputs": [...], "events": [...]} on stdin and
// prints one line per problem; exits 1 if there is any.
//
//   AGUI_CORE_DIR=/dir/with/node_modules node validate.cjs < doc.json
//
// The directory must hold node_modules/@ag-ui/core (1.0.2 was used) and zod
// (3.25.x). Neither is a dependency of Kubo: the Go test that calls this script
// skips when AGUI_CORE_DIR is not set (see SOURCES.md).
const { createRequire } = require('module');
const path = require('path');

const dir = process.env.AGUI_CORE_DIR;
if (!dir) {
  console.error('AGUI_CORE_DIR is not set');
  process.exit(2);
}
const req = createRequire(path.join(dir, 'noop.js'));
const { EventSchemas, RunAgentInputSchema, PROTOCOL_VERSION } = req('@ag-ui/core/schemas');

let input = '';
process.stdin.on('data', (c) => (input += c));
process.stdin.on('end', () => {
  const doc = JSON.parse(input);
  const problems = [];
  (doc.inputs || []).forEach((v, i) => {
    const r = RunAgentInputSchema.safeParse(v);
    if (!r.success) problems.push(`input ${i}: ${JSON.stringify(r.error.issues.slice(0, 3))}`);
  });
  (doc.events || []).forEach((v, i) => {
    const r = EventSchemas.safeParse(v);
    if (!r.success) problems.push(`event ${i} ${v && v.type}: ${JSON.stringify(r.error.issues.slice(0, 3))}`);
  });
  problems.forEach((p) => console.log(p));
  console.error(`protocol ${PROTOCOL_VERSION}: ${(doc.inputs || []).length} inputs, ${(doc.events || []).length} events, ${problems.length} problems`);
  process.exit(problems.length ? 1 : 0);
});
