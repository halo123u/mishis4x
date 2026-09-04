import { useEffect, useState } from 'react';
import type { AdminInviteRequest } from '../types';
import { formatFreshness } from '../priceFreshness';
import Button from './ui/Button';
import styles from './AdminInvites.module.css';

// The admin-only invite request queue - GET /api/admin/invites lists
// everything still 'requested' (see be/handlers/admin.go), and
// approve/deny here are the web equivalent of the invite-approve/
// invite-deny CLI commands. Server-side, this whole page is backed by
// adminOnlyMiddleware (see handlers.Data.AdminUserID's doc comment) -
// the nav link that gets here is hidden for anyone else, but that's just
// UX; a non-admin hitting these routes directly still gets a real 403.
const AdminInvites = () => {
  const [requests, setRequests] = useState<AdminInviteRequest[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  // Keyed by request id - only one decision in flight per row at a time,
  // same "disable just this row's buttons, not the whole page" reasoning
  // as SetDetail's per-card refresh state.
  const [decidingID, setDecidingID] = useState<number | null>(null);
  const [actionError, setActionError] = useState<{
    id: number;
    message: string;
  } | null>(null);

  const loadRequests = () => {
    fetch('/api/admin/invites')
      .then(async (res) => {
        if (res.status !== 200) {
          // The nav link that gets here is already hidden for a non-admin
          // (see Navigation.tsx), so reaching this specifically means
          // someone navigated here directly - surface the server's real
          // reason rather than a generic network-sounding message.
          const body = await res.json().catch(() => null);
          setError(
            body?.error ?? 'Could not load invite requests. Please try again.',
          );
          return;
        }
        setRequests(await res.json());
      })
      .catch(() => {
        setError('Could not reach the server. Please try again.');
      });
  };

  useEffect(() => {
    loadRequests();
  }, []);

  const decide = (id: number, action: 'approve' | 'deny') => {
    setDecidingID(id);
    setActionError(null);

    fetch(`/api/admin/invites/${id}/${action}`, { method: 'POST' })
      .then(async (res) => {
        if (res.status !== 200) {
          const body = await res.json().catch(() => null);
          setActionError({
            id,
            message: body?.error ?? 'Something went wrong. Please try again.',
          });
          return;
        }
        // Re-fetch rather than just filtering the row out locally - a
        // second admin (or the CLI) could have acted on a different
        // request in the meantime, same "don't trust locally-cached
        // state past a mutation" reasoning as SetDetail's refresh flow.
        loadRequests();
      })
      .catch(() => {
        setActionError({
          id,
          message: 'Could not reach the server. Please try again.',
        });
      })
      .finally(() => {
        setDecidingID(null);
      });
  };

  return (
    <div className="stack">
      <h1>Invite requests</h1>

      {error && (
        <p className={styles.error} role="alert">
          {error}
        </p>
      )}

      {!error && requests === null && (
        <p className="muted">Loading invite requests…</p>
      )}

      {requests && requests.length === 0 && (
        <p className="muted">No pending invite requests.</p>
      )}

      {requests && requests.length > 0 && (
        <ul className={styles.list}>
          {requests.map((req) => {
            const freshness = formatFreshness(req.created_at);
            const deciding = decidingID === req.id;
            return (
              <li key={req.id} className={styles.row}>
                <div>
                  <span className={styles.email}>{req.email_address}</span>
                  {freshness && (
                    <span className={styles.meta}>
                      requested {freshness.amount} {freshness.suffix}
                    </span>
                  )}
                  {actionError?.id === req.id && (
                    <p className={styles.error} role="alert">
                      {actionError.message}
                    </p>
                  )}
                </div>
                <div className={styles.actions}>
                  <Button
                    onClick={() => decide(req.id, 'approve')}
                    disabled={deciding}
                  >
                    {deciding ? 'Working…' : 'Approve'}
                  </Button>
                  <Button
                    variant="danger"
                    onClick={() => decide(req.id, 'deny')}
                    disabled={deciding}
                  >
                    Deny
                  </Button>
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
};

export default AdminInvites;
