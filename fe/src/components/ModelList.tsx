import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import Button from './ui/Button.tsx';
import styles from './ModelList.module.css';

// Lists every char_code the model-import CLI command has stored (see
// be/cmd/model_import.go) - gated server-side by modelOnlyMiddleware, so a
// 403 here means this account isn't handlers.Data.ModelViewerUserID, not a
// bug. Navigation only links here at all when GlobalData.model_viewer_enabled
// is true (see Navigation.tsx), but this page checks the real response too
// rather than trusting that gate alone.
const ModelList = () => {
  const [codes, setCodes] = useState<string[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [clearing, setClearing] = useState(false);
  const [clearMessage, setClearMessage] = useState<string | null>(null);

  useEffect(() => {
    fetch('/api/models')
      .then(async (res) => {
        if (res.status === 200) {
          setCodes(await res.json());
          return;
        }
        setError(
          res.status === 403
            ? 'Not available on this account.'
            : 'Something went wrong. Please try again.',
        );
      })
      .catch(() => setError('Could not reach the server. Please try again.'));
  }, []);

  // Resets the remote display (see ModelDisplay.tsx) back to its
  // "nothing selected" state - an empty char_code is a real, deliberate
  // value the backend accepts (see SetModelDisplay's own doc comment),
  // not something a controller could otherwise reach: Broadcast only
  // ever sends the character currently loaded in *this* browser, and
  // turning Broadcast off deliberately leaves the display showing
  // whatever it last had (same as unplugging a remote doesn't turn the
  // TV off) - there was previously no way at all to blank the display
  // short of restarting the server (which clears this in-memory state
  // as a side effect, not a real workflow).
  const clearDisplay = () => {
    setClearing(true);
    setClearMessage(null);
    fetch('/api/models/display', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ char_code: '', flip: '', pepper: false }),
    })
      .then((res) => {
        setClearMessage(
          res.ok ? 'Display cleared.' : 'Could not clear the display.',
        );
      })
      .catch(() => setClearMessage('Could not reach the server.'))
      .finally(() => setClearing(false));
  };

  return (
    <div className="stack">
      <h1>Character models</h1>
      {/* /models/display has no link to it anywhere else, and no way to
          type its URL directly on a device launched standalone from a
          Home Screen icon (see index.html's apple-mobile-web-app-capable -
          that mode removes Safari's address bar entirely). This page is
          already reachable through normal in-app navigation (the nav
          menu, gated the same GlobalData.model_viewer_enabled way this
          whole page is), so it's the one place worth putting a real link
          from - see ModelDisplay.tsx's own doc comment for what it is. */}
      <div className="row">
        <Link to="/models/display" className={styles.remoteDisplayLink}>
          Open remote display →
        </Link>
        <Button variant="secondary" onClick={clearDisplay} disabled={clearing}>
          {clearing ? 'Clearing…' : 'Clear display'}
        </Button>
        {clearMessage && <span className="muted">{clearMessage}</span>}
      </div>
      {error && <p className="muted">{error}</p>}
      {!error && codes === null && <p className="muted">Loading…</p>}
      {!error && codes !== null && codes.length === 0 && (
        <p className="muted">
          Nothing imported yet - see `model-import` in be/cmd.
        </p>
      )}
      {!error && codes !== null && codes.length > 0 && (
        <ul className={styles.grid}>
          {codes.map((code) => (
            <li key={code}>
              <Link to={`/models/${code}`} className={styles.card}>
                {code}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
};

export default ModelList;
