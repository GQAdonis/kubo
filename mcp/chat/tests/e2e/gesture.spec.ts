import { expect, test } from './harness';
import { ask, userTextOf } from './helpers';

// Falsifier G11-3 (docs/mcp/0005): chat interrupts can only be resolved by a user gesture. Here the
// attacker is page script, in the same origin as the island, trying to answer an approval without a person.
// The positive cases use Playwright's real input (CDP Input.dispatch*), which the browser marks trusted.

async function reachApproval(page: import('@playwright/test').Page, fixtureName: string) {
  await ask(page, userTextOf(fixtureName));
  await expect(page.getByRole('group', { name: /Pin bafkrei|Run garbage collection/ })).toBeVisible();
}

test('script cannot approve: element.click(), dispatchEvent and Playwright dispatchEvent all fail', async ({ harness }) => {
  const page = await harness.open({ island: 'open' });
  await reachApproval(page, 'write-interrupt');
  await harness.resetFixtureRequests();

  const approve = page.getByRole('button', { name: 'Approve change' });
  await approve.evaluate((el) => (el as HTMLButtonElement).click());
  await approve.evaluate((el) => el.dispatchEvent(new MouseEvent('click', { bubbles: true, composed: true, cancelable: true })));
  await approve.evaluate((el) => el.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, composed: true })));
  await approve.dispatchEvent('click'); // Playwright's own synthetic dispatch: also untrusted

  await expect(page.getByText(/An answer needs a click or key press from you/)).toBeVisible();
  await expect(page.getByText('Awaiting a person')).toBeVisible();
  expect(await harness.fixtureRequests()).toEqual([]); // nothing was sent: the approval was never answered
});

test('script cannot deny either', async ({ harness }) => {
  const page = await harness.open({ island: 'open' });
  await reachApproval(page, 'write-interrupt');
  await harness.resetFixtureRequests();
  await page.getByRole('button', { name: 'Deny' }).evaluate((el) => (el as HTMLButtonElement).click());
  await expect(page.getByText(/An answer needs a click or key press from you/)).toBeVisible();
  expect(await harness.fixtureRequests()).toEqual([]);
});

test('script cannot reach into the card and replay a click it has seen', async ({ harness }) => {
  const page = await harness.open({ island: 'open' });
  await reachApproval(page, 'write-interrupt');
  await harness.resetFixtureRequests();
  // A listener that re-dispatches whatever click it sees, on the approve button, as a synthetic click.
  await page.evaluate(() => {
    const card = document.querySelector('ipfs-agent-chat')?.shadowRoot?.querySelector('.iu-approval');
    card?.addEventListener('click', (e) => {
      const approve = [...(card?.querySelectorAll('button') ?? [])].find((b) => b.classList.contains('iu-btn--primary'));
      if (e.isTrusted) approve?.dispatchEvent(new MouseEvent('click', { bubbles: true, composed: true }));
    });
  });
  // A real click on Deny's neighbour area (the card heading) triggers that listener's synthetic approve.
  await page.getByRole('heading', { name: /Pin bafkrei/ }).click();
  expect(await harness.fixtureRequests()).toEqual([]);
  await expect(page.getByText('Awaiting a person')).toBeVisible();
});

test('a real click on Approve resumes the run with exactly one resolved answer', async ({ harness }) => {
  const page = await harness.open({ island: 'open' });
  await reachApproval(page, 'write-interrupt');
  await harness.resetFixtureRequests();
  await page.getByRole('button', { name: 'Approve change' }).click();
  await expect(page.getByText('Pinned.')).toBeVisible();
  await expect(page.getByText('Approved', { exact: true })).toBeVisible();

  const sent = await harness.fixtureRequests();
  expect(sent).toHaveLength(1);
  expect(sent[0]?.headers['authorization']).toBe(`Bearer ${harness.env.token}`);
  expect(sent[0]?.body?.resume).toEqual([{ interruptId: 'int-cf_Zr4Yt1nB6kq0HdUe', status: 'resolved', payload: { approved: true } }]);
  expect(sent[0]?.matched).toBe('write-resume-approved');
});

test('the keyboard works: Enter on the focused Approve button answers', async ({ harness }) => {
  const page = await harness.open({ island: 'open' });
  await reachApproval(page, 'write-interrupt');
  await harness.resetFixtureRequests();
  await page.getByRole('button', { name: 'Approve change' }).focus();
  await page.keyboard.press('Enter');
  await expect(page.getByText('Pinned.')).toBeVisible();
  expect(await harness.fixtureRequests()).toHaveLength(1);
});

test('focus lands on Deny when an approval appears', async ({ harness }) => {
  const page = await harness.open({ island: 'open' });
  await reachApproval(page, 'write-interrupt');
  await expect(page.getByRole('button', { name: 'Deny' })).toBeFocused();
});

test('a denial is sent as resolved with approved: false', async ({ harness }) => {
  const page = await harness.open({ island: 'open' });
  await reachApproval(page, 'write-interrupt');
  await harness.resetFixtureRequests();
  await page.getByRole('button', { name: 'Deny' }).click();
  await expect(page.getByText('Denied', { exact: true })).toBeVisible();
  const sent = await harness.fixtureRequests();
  expect(sent[0]?.body?.resume).toEqual([{ interruptId: 'int-cf_Zr4Yt1nB6kq0HdUe', status: 'resolved', payload: { approved: false } }]);
});

test('a destructive call that needs the system prompt has no Approve control, and Deny is all there is', async ({ harness }) => {
  const page = await harness.open({ island: 'open' });
  await ask(page, userTextOf('destructive-native-prompt'));
  // The card and the header badge both say it; the session sent {wait: true} and is watching.
  await expect(page.getByText('Waiting for system prompt').first()).toBeVisible();
  await expect(page.getByRole('button', { name: /^Approve/ })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Deny' })).toBeVisible();
  // Even a script that fabricates an approve button and clicks it in the card sends nothing.
  await harness.resetFixtureRequests();
  await page.evaluate(() => {
    const actions = document.querySelector('ipfs-agent-chat')?.shadowRoot?.querySelector('.iu-approval__actions');
    const fake = document.createElement('button');
    fake.className = 'iu-btn iu-btn--primary';
    fake.textContent = 'Approve change';
    actions?.append(fake);
    fake.click();
  });
  expect(await harness.fixtureRequests()).toEqual([]);
});
