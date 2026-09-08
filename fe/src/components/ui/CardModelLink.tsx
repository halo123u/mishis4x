import { useState } from 'react';
import { Link } from 'react-router-dom';
import { Card } from '../../types';
import ModelIcon from './ModelIcon';
import styles from './CardModelLink.module.css';

// Links one catalog card to a character model (be/persist/character_
// models.go's character_model_char_code column) - several cards commonly
// share the same model across rarities (a character redrawn at R/SR/SSR
// is usually still the same underlying model), so this is a plain picker
// per card, not a bulk "apply to a whole rarity run" tool; linking each
// one is a few clicks either way given how few models actually get
// imported (see model-import's own doc comment on why it's a hand-picked
// allowlist, not a bulk import).
//
// Only ever rendered when GlobalData.model_viewer_enabled is true (see
// SetDetail.tsx) - the backend enforces the same restriction on the PUT
// itself, this is just about not showing a picker for a feature this
// account can't use anyway.
const CardModelLink = ({
  card,
  availableCodes,
  onLinked,
}: {
  card: Card;
  availableCodes: string[];
  onLinked: (cardId: string, charCode: string | null) => void;
}) => {
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(false);

  const handleChange = (event: React.ChangeEvent<HTMLSelectElement>) => {
    const charCode = event.target.value || null;
    setSaving(true);
    setError(false);

    fetch(`/api/models/cards/${card.id}`, {
      method: 'PUT',
      headers: { 'Content-type': 'application/json' },
      body: JSON.stringify({ char_code: charCode }),
    })
      .then((res) => {
        if (res.status === 200) {
          onLinked(card.id, charCode);
          return;
        }
        setError(true);
      })
      .catch(() => setError(true))
      .finally(() => setSaving(false));
  };

  return (
    <div className={styles.row}>
      {card.character_model_char_code && (
        <Link
          to={`/models/${card.character_model_char_code}`}
          className={styles.viewLink}
          aria-label={`View ${card.name}'s live model`}
        >
          <ModelIcon />
        </Link>
      )}
      <select
        aria-label={`Link ${card.name} to a character model`}
        className={styles.select}
        value={card.character_model_char_code ?? ''}
        onChange={handleChange}
        disabled={saving}
      >
        <option value="">No model</option>
        {availableCodes.map((code) => (
          <option key={code} value={code}>
            {code}
          </option>
        ))}
      </select>
      {error && <span className={styles.error}>Failed to save.</span>}
    </div>
  );
};

export default CardModelLink;
