// TestDashboard in test/e2e runs these against the kind cluster, after it creates an app called app in
// KUPRA_E2E_NAMESPACE, with a running preview called shared, and binds creator@example.com to kupra-creator
// and viewer@example.com to kupra-viewer there.
import { expect, test, type Page } from '@playwright/test';

const namespace = process.env.KUPRA_E2E_NAMESPACE ?? '';
const appPath = `/apps/${namespace}/app`;
const previewsPath = `/api/kube/apis/kupra.futo.tech/v1alpha1/namespaces/${namespace}/previews`;
// Previews start pulling the pause image and waiting for the operator, which can take a while.
const startTimeout = 3 * 60_000;

/** signIn opens the app's page, signs in with Dex's login form and waits to be back. */
const signIn = async (page: Page, email: string) => {
  await page.goto(appPath);
  await page.locator('#login').fill(email);
  await page.locator('#password').fill('password');
  await page.locator('#submit-login').click();
  await expect(page).toHaveURL(appPath);
  await expect(page.getByText(`oidc:${email}`)).toBeVisible();
};

test.beforeAll(() => {
  expect(namespace, 'KUPRA_E2E_NAMESPACE').not.toBe('');
});

test('creator creates, resets and deletes a preview', async ({ page }) => {
  await signIn(page, 'creator@example.com');

  await page.getByRole('button', { name: 'New preview' }).click();
  const form = page.getByRole('dialog');
  await form.getByPlaceholder('pr-1234').fill('pr-1');
  await form.getByRole('button', { name: 'Create' }).click();
  await expect(page).toHaveURL(`${appPath}/previews/pr-1`);
  await expect(page.getByText('Running', { exact: true })).toBeVisible({ timeout: startTimeout });
  await expect(page.getByRole('link', { name: 'Web' })).toHaveAttribute('href', 'https://pr-1.example.com/');

  await page.getByRole('button', { name: 'Actions' }).click();
  await page.getByRole('menuitem', { name: 'Reset' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Reset' }).click();
  // The page doesn't show whether a reset was handled, so this asks the API, through the dashboard, as the creator.
  await expect
    .poll(
      async () => {
        const response = await page.request.get(`${previewsPath}/pr-1`);
        const preview = (await response.json()) as {
          metadata: { annotations?: Record<string, string> };
          status?: { lastHandledResetAt?: string };
        };
        const requested = preview.metadata.annotations?.['kupra.futo.tech/reset-requested-at'];
        return requested !== undefined && requested === preview.status?.lastHandledResetAt;
      },
      { message: 'reset is handled', timeout: startTimeout },
    )
    .toBe(true);
  await expect(page.getByText('Running', { exact: true })).toBeVisible({ timeout: startTimeout });

  await page.getByRole('button', { name: 'Actions' }).click();
  await page.getByRole('menuitem', { name: 'Delete' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Delete' }).click();
  await expect(page).toHaveURL(appPath);
  await expect(page.getByRole('link', { name: 'pr-1', exact: true })).toHaveCount(0, { timeout: startTimeout });
  await expect(page.getByRole('link', { name: 'shared', exact: true })).toBeVisible();
});

test('viewer sees previews without actions', async ({ page }) => {
  // The page shows actions once the API server allows them, so their absence only counts once it has answered.
  const reviews: boolean[] = [];
  page.on('response', (response) => {
    if (response.url().endsWith('/selfsubjectaccessreviews') && response.ok()) {
      response
        .json()
        .then((review: { status?: { allowed?: boolean } }) => {
          reviews.push(review.status?.allowed ?? false);
        })
        // Responses to a page that has navigated away have no body.
        .catch(() => {});
    }
  });

  await signIn(page, 'viewer@example.com');

  await expect(page.getByRole('link', { name: 'app', exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: 'shared', exact: true })).toBeVisible();
  // One review for creating previews, and four for the actions on shared.
  await expect.poll(() => reviews.length, { message: 'access reviews' }).toBeGreaterThanOrEqual(5);
  expect(reviews).not.toContain(true);
  await expect(page.getByRole('button', { name: 'New preview' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Actions' })).toHaveCount(0);

  await page.getByRole('link', { name: 'shared', exact: true }).click();
  await expect(page).toHaveURL(`${appPath}/previews/shared`);
  await expect(page.getByText('Running', { exact: true })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Web' })).toHaveAttribute('href', 'https://shared.example.com/');
  await expect(page.getByRole('button', { name: 'Actions' })).toHaveCount(0);
});
