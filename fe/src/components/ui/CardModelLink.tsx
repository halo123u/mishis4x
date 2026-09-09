import { Link } from 'react-router-dom';
import { Card } from '../../types';
import ModelIcon from './ModelIcon';
import styles from './CardModelLink.module.css';

// Links to a card's character model (cards.character_model_char_code,
// set via PUT /api/models/cards/{cardID} - see be/handlers/character_
// models.go) once one's been linked; renders nothing otherwise. No
// inline picker for setting the link itself - with only a handful of
// models ever imported at once, that's a rare enough action to not need
// its own UI here yet.
//
// Only ever rendered when GlobalData.model_viewer_enabled is true (see
// SetDetail.tsx) - the backend enforces the same restriction on every
// /api/models/... route, this is just about not showing a link for a
// feature this account can't use anyway.
const CardModelLink = ({ card }: { card: Card }) => {
  if (!card.character_model_char_code) {
    return null;
  }

  return (
    <Link
      to={`/models/${card.character_model_char_code}`}
      className={styles.viewLink}
      aria-label={`View ${card.name}'s live model`}
    >
      <ModelIcon />
    </Link>
  );
};

export default CardModelLink;
