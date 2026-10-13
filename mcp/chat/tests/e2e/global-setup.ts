import { startFixtureServer } from '../helpers/fixture-server';
import { startHarnessServer } from './harness-server';

// Starts the two local servers every browser spec talks to, each on a port the OS picks (the operator's
// port rule: nothing here hardcodes a port):
//   - the AG-UI fixture server, standing in for `ipfs mcp` /agui (the live run is webui-agent-chat);
//   - the harness server, serving the stand-in Web UI page and the island's harness build.
// Their addresses reach the specs through CHAT_E2E in the environment, which workers inherit.

export const E2E_TOKEN = 'e2e-bearer-0123456789abcdef0123';

export interface E2eEnv {
  readonly harnessOrigin: string;
  readonly fixtureOrigin: string;
  readonly endpoint: string;
  readonly token: string;
}

export default async function globalSetup(): Promise<() => Promise<void>> {
  // Browser runs use random thread, run and message ids, so the fixture server matches by conversation.
  const fixtures = await startFixtureServer({ token: E2E_TOKEN, matching: 'conversation', keepAlive: true, controlEndpoint: true, holdWait: true });
  const harness = await startHarnessServer({ endpoint: fixtures.endpoint, token: E2E_TOKEN, connectOrigins: [fixtures.origin] });
  // The server's origin allowlist (ADR-0005): the page's origin over http, and Desktop's webui scheme.
  fixtures.allow(harness.origin);
  fixtures.allow('webui://localhost');

  const env: E2eEnv = { harnessOrigin: harness.origin, fixtureOrigin: fixtures.origin, endpoint: fixtures.endpoint, token: E2E_TOKEN };
  process.env['CHAT_E2E'] = JSON.stringify(env);
  // Specs read the request log by asking the fixture server over HTTP (GET /__test/requests).
  return async () => {
    await harness.close();
    await fixtures.close();
  };
}
