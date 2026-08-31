import { test, expect, Page } from "@playwright/test";
import { nanoid } from "nanoid";

// Relies on the fixture set/cards in be/db/seeds/005_sets_seed.sql and
// 006_cards_seed.sql - not real catalog data (that's #68/#70's job via the
// CSV import), just a committed fixture so this test has something real to
// click through in both local dev and CI. If that seed data ever changes,
// this test's assertions need to move with it.

test("card manager: widget -> dashboard -> set detail -> back", async ({
  page,
}) => {
  await login(page, "test", "test");

  // "Card Manager" appears as the widget's heading on Home - clicking it
  // is the actual entry point into the feature this test exercises.
  await page.getByRole("heading", { name: "Card Manager" }).click();

  await expect(page).toHaveURL(/\/collection$/);

  // The dashboard only lists onboarded (owned) sets, not the full catalog -
  // a fresh account starts empty here even though the catalog doesn't (see
  // CollectionDashboard's own comment). Onboard "Brown Dust 2" via the real
  // Add-a-set flow if it isn't already: CI always starts from a fresh seed,
  // but a repeat run against a persistent local `make run_db` volume won't,
  // and the fixture only has this one set to onboard either way.
  // isVisible() alone would race the dashboard's own async fetch (it
  // checks the DOM synchronously, before either outcome has necessarily
  // rendered yet) - wait for whichever of the two real end states actually
  // shows up instead of guessing at the state before data has loaded.
  const alreadyOwned = await Promise.race([
    page
      .getByText("Brown Dust 2")
      .waitFor({ state: "visible" })
      .then(() => true),
    page
      .getByRole("button", { name: "Add a set" })
      .waitFor({ state: "visible" })
      .then(() => false),
  ]);

  if (!alreadyOwned) {
    await page.getByRole("button", { name: "Add a set" }).click();
    await expect(page).toHaveURL(/\/collection\/add$/);
    await page.getByRole("button", { name: "Add" }).click();

    // Onboarding doesn't require checking any cards - "Skip for now"
    // onboards the set itself and lands straight on its detail page.
    await expect(page).toHaveURL(/\/collection\/[^/]+\/onboard$/);
    await page.getByRole("button", { name: "Skip for now" }).click();
    await expect(page).toHaveURL(/\/collection\/[^/]+$/);

    await page.getByText("Back to sets").click();
    await expect(page).toHaveURL(/\/collection$/);
  }

  await expect(page.getByText("Brown Dust 2")).toBeVisible();

  await page.getByText("Brown Dust 2").click();

  await expect(page).toHaveURL(/\/collection\/[^/]+$/);
  // exact: true - the real catalog also has "BRD/W139-001SSP", and a plain
  // substring match on "BRD/W139-001S" now hits both, which Playwright
  // (rightly) refuses to resolve to a single element.
  await expect(page.getByText("BRD/W139-001S", { exact: true })).toBeVisible();
  await expect(page.getByText("Poolside Fairy Refithea")).toBeVisible();

  await page.getByText("Back to sets").click();
  await expect(page).toHaveURL(/\/collection$/);
});

test("card manager: unknown set shows a not-found message, not a crash", async ({
  page,
}) => {
  await login(page, "test", "test");

  await page.goto("http://localhost:8091/collection/does-not-exist");
  await expect(page.getByText("This set could not be found.")).toBeVisible();
});

test("card manager: a real non-owner account can access it too", async ({
  page,
}) => {
  // The collection tracker isn't eBay-sourced data (catalog/images come
  // from TCG Republic, ownership/price-paid is the user's own), so it's
  // open to any authenticated user - not just the seeded "test" user (id
  // 1), which used to be the configured COLLECTION_OWNER_USER_ID for this
  // stack. See handlers.Data.CollectionOwnerUserID's doc comment: that
  // restriction is kept around for a future market-rate feature instead of
  // gating the whole tracker.
  await page.goto("http://localhost:8091/login");
  await page.click('a[href="/sign-up"]');
  await page.fill('input[name="username"]', nanoid());
  await page.fill('input[name="password"]', "validpass123");
  await page.click('button[type="submit"]');
  await page.waitForURL("http://localhost:8091/");

  await page.goto("http://localhost:8091/collection");
  // A brand new account starts with nothing onboarded - "Add a set" (not
  // an error) is the correct empty state, same as any account would see.
  await expect(
    page.getByText("Could not load sets. Please try again."),
  ).not.toBeVisible();
  await expect(page.getByRole("button", { name: "Add a set" })).toBeVisible();

  // Confirms GET /api/sets itself succeeds for this account too, not just
  // /api/owned-sets - the catalog picker is where that would show up.
  await page.getByRole("button", { name: "Add a set" }).click();
  await expect(page.getByText("Brown Dust 2")).toBeVisible();
});

const login = async (page: Page, username: string, password: string) => {
  await page.goto("http://localhost:8091/login");
  await page.fill('input[name="username"]', username);
  await page.fill('input[name="password"]', password);
  await page.click('button[type="submit"]');

  // Not asserting on "Welcome to mishis4x" text here (see login.spec.ts) -
  // Playwright's getByText matches case-insensitively by default, and the
  // Login page's own heading ("Welcome to Mishis4x") satisfies that same
  // text before the real post-login navigation ever happens. Waiting for
  // the URL itself is the only way to know we've actually reached Home,
  // not just that the login page (still) renders similar-looking text.
  await page.waitForURL("http://localhost:8091/");
};
