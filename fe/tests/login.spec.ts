import { test, expect, Page } from "@playwright/test";
import { nanoid } from "nanoid";

test("login/logout", async ({ page }) => {
  await page.goto("http://localhost:5173/");

  await submitForm(page, "test", "test");

  await expect(page.getByText("Welcome to my cool website")).toBeVisible();

  await page.getByRole("button", { name: "Logout" }).click();

  await expect(page.getByText("Welcome to Mishis4x")).toBeVisible();
});

test("test create account", async ({ page }) => {
  await page.goto("http://localhost:5173/");

  await page.click('a[href="/sign-up"]');

  await submitForm(page, nanoid(), "test");
});

const submitForm = async (page: Page, username: string, password: string) => {
  await page.fill('input[name="username"]', username);
  await page.fill('input[name="password"]', password);
  await page.click('button[type="submit"]');

  await expect(page.getByText("Welcome to my cool website")).toBeVisible();
};
