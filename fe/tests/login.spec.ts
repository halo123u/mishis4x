import { test, expect, Page } from "@playwright/test";
import { nanoid } from "nanoid";

test("login/logout", async ({ page }) => {
  await page.goto("http://localhost:8091/login");

  // submitForm already waits for the real post-login URL and asserts the
  // Home heading is visible - no need to repeat that check here.
  await submitForm(page, "test", "test");

  await page.getByRole("button", { name: "Log out" }).click();

  // Wait for the real URL before the text check - getByText matches
  // case-insensitively by default, so "Welcome to Mishis4x" here would
  // also match Home's still-rendered "Welcome to mishis4x" during the
  // async gap between clicking Log out and navigate('/login') actually
  // firing (logout awaits a fetch() first). See submitForm below for the
  // same race in the other direction.
  await page.waitForURL("http://localhost:8091/login");
  await expect(page.getByText("Welcome to Mishis4x")).toBeVisible();
});

test("test create account", async ({ page }) => {
  await page.goto("http://localhost:8091/login");

  await page.click('a[href="/sign-up"]');

  // Signup enforces an 8-char minimum password (see UserForm's
  // passwordMinLength) - login intentionally does not, so the seeded
  // "test"/"test" user above is unaffected.
  await submitForm(page, nanoid(), "validpass123");
});

const submitForm = async (page: Page, username: string, password: string) => {
  await page.fill('input[name="username"]', username);
  await page.fill('input[name="password"]', password);
  await page.click('button[type="submit"]');

  // Wait for the real post-login URL before trusting the text check - see
  // the logout assertion above for why getByText alone is a false-positive
  // waiting to happen (this was actually caught doing exactly that, in
  // collection.spec.ts).
  await page.waitForURL("http://localhost:8091/");
  await expect(page.getByText("Welcome to mishis4x")).toBeVisible();
};
