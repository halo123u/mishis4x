import { useNavigate } from 'react-router-dom';
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
  const navigate = useNavigate();

  if (!card.character_model_char_code) {
    return null;
  }

  // Not a plain <Link>: before leaving, this rewrites *this* history
  // entry (replace, not push - the click shouldn't itself add a stop to
  // the back stack) to carry `#card-{id}` in its URL. SetDetail's own
  // scroll-to-hash effect uses that to find its way back to this exact
  // card once the user backs out of the model - see that effect's own
  // comment for why the URL, not React state, is what has to carry this:
  // on a phone, ModelViewer's WebGL canvas is heavy enough that the
  // browser frequently discards the whole page from its back-cache
  // rather than restoring it, so backing out is often a genuine fresh
  // reload, not a client-side pop - anything held only in memory (scroll
  // position, component state, router state) is gone by then. A URL
  // hash is the one thing that survives either way, since it's part of
  // the history entry itself, not this page's own JS state.
  const handleClick = (event: React.MouseEvent) => {
    event.preventDefault();
    navigate(`${location.pathname}#card-${card.id}`, { replace: true });
    navigate(`/models/${card.character_model_char_code}`);
  };

  return (
    <a
      href={`/models/${card.character_model_char_code}`}
      onClick={handleClick}
      className={styles.viewLink}
      aria-label={`View ${card.name}'s live model`}
    >
      <ModelIcon />
    </a>
  );
};

export default CardModelLink;
