import { test, expect } from "@playwright/test";

test.only("login/logout", async ({ page }) => {
  await page.goto("http://localhost:5173/");

  await page.fill('input[name="username"]', "test");
  await page.fill('input[name="password"]', "test");

  await page.click('button[type="submit"]');

  await expect(page.getByText("Welcome to my cool website")).toBeVisible();

  await page.getByRole("button", { name: "Logout" }).click();

  await expect(page.getByText("Welcome to Mishis4x")).toBeVisible();
});
