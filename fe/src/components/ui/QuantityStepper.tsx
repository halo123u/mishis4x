import { FC } from 'react';
import styles from './QuantityStepper.module.css';

type QuantityStepperProps = {
  value: number;
  onChange: (value: number) => void;
  // Not a visible label - this component has no room for one, so callers
  // pass what a checkbox's own label used to say (e.g. "Quantity of X")
  // and it's applied to every part needing one, distinguished by prefix.
  ariaLabel: string;
  min?: number;
  disabled?: boolean;
};

// A compact "how many" control: the number and both buttons read as one
// input-shaped unit, not three separate elements with gaps between them.
// Flanking −/+ buttons (not stacked ▲▼ arrows, an earlier design tuned for
// a cramped table cell) - both screens now share the wider binder-grid
// tile layout, so there's room for real tap targets instead of a pair of
// ~20x16px arrows that were hard to hit precisely on mobile.
const QuantityStepper: FC<QuantityStepperProps> = ({
  value,
  onChange,
  ariaLabel,
  min = 0,
  disabled = false,
}) => {
  const canDecrease = !disabled && value > min;

  return (
    <span className={styles.stepper}>
      <button
        type="button"
        className={styles.side}
        aria-label={`Decrease ${ariaLabel}`}
        onClick={() => onChange(Math.max(min, value - 1))}
        disabled={!canDecrease}
      >
        −
      </button>
      <input
        type="number"
        min={min}
        aria-label={ariaLabel}
        className={styles.value}
        value={value}
        onChange={(event) =>
          onChange(Math.max(min, Number(event.target.value)))
        }
        disabled={disabled}
      />
      <button
        type="button"
        className={styles.side}
        aria-label={`Increase ${ariaLabel}`}
        onClick={() => onChange(value + 1)}
        disabled={disabled}
      >
        +
      </button>
    </span>
  );
};

export default QuantityStepper;
