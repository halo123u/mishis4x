import { execFileSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import path from "node:path";

// `docker compose` resolves compose.yaml relative to its own cwd, but
// Playwright itself runs with cwd fe/ (see package.json's test script) -
// point at the repo-root compose.yaml explicitly rather than relying on
// whatever directory this happens to be invoked from.
const composeFile = path.resolve(__dirname, "..", "..", "compose.yaml");

let emailCounter = 0;

// Signup is invite-only (see be/handlers/users.go's UserCreate), and a
// redeemable code only exists once the owner has approved a request (see
// be/cmd/invite.go's invite-approve) - which sends a real email via
// Resend. e2e tests shouldn't depend on a configured Resend account or
// trigger a real send, so this bypasses the request/approve flow
// entirely and inserts an already-'approved' row straight into the `db`
// service, the same way be/handlers' own test helpers bypass HTTP for
// setup (see testApprovedInvite in be/handlers/helpers_test.go) rather
// than going through invite-approve for real.
export function mintApprovedInviteCode(): string {
  const code = randomBytes(32).toString("base64url");
  emailCounter += 1;
  const email = `e2e-invite-${Date.now()}-${emailCounter}@example.com`;

  execFileSync(
    "docker",
    [
      "compose",
      "-f",
      composeFile,
      "exec",
      "-T",
      "db",
      "mysql",
      "-uroot",
      "-proot_password",
      "mishis4x",
      "-e",
      `INSERT INTO invites (code, status, email_address) VALUES ('${code}', 'approved', '${email}');`,
    ],
    { encoding: "utf-8" },
  );

  return code;
}
