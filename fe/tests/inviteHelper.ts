import { execFileSync } from "node:child_process";
import path from "node:path";

// `docker compose` resolves compose.yaml relative to its own cwd, but
// Playwright itself runs with cwd fe/ (see package.json's test script) -
// point at the repo-root compose.yaml explicitly rather than relying on
// whatever directory this happens to be invoked from.
const composeFile = path.resolve(__dirname, "..", "..", "compose.yaml");

// Signup is invite-only (see be/handlers/users.go's UserCreate) - e2e tests
// that need to create a fresh account go through the exact same path a real
// admin would: the `invite-create` CLI command, run inside the already-
// running `app` container from compose.yaml, exactly like a real deploy
// would (`docker compose exec app ./mishis4x invite-create`). No test-only
// backend route exists to mint one, and shouldn't - that would mean testing
// a code path that doesn't exist in production.
export function mintInviteToken(): string {
  const output = execFileSync(
    "docker",
    [
      "compose",
      "-f",
      composeFile,
      "exec",
      "-T",
      "app",
      "./mishis4x",
      "invite-create",
      "--env",
      "test",
    ],
    { encoding: "utf-8" },
  );

  // Matches be/cmd/invite.go's fmt.Printf("invite created: /sign-up?invite=%s\n", token).
  const match = output.match(/invite=(\S+)/);
  if (!match) {
    throw new Error(
      `could not parse invite token from invite-create output: ${output}`,
    );
  }
  return match[1];
}
