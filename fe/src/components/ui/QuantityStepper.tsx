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

// A compact "how many" control: the number and both arrows read as one
// input-shaped unit, not three separate elements with gaps between them -
// arrows stack vertically (▲ over ▼) rather than flanking the number
// left/right, which is what actually saves the horizontal space a
// side-by-side -/+ pair costs in a table cell.
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
      <span className={styles.arrows}>
        <button
          type="button"
          className={styles.arrow}
          aria-label={`Increase ${ariaLabel}`}
          onClick={() => onChange(value + 1)}
          disabled={disabled}
        >
          ▲
        </button>
        <button
          type="button"
          className={styles.arrow}
          aria-label={`Decrease ${ariaLabel}`}
          onClick={() => onChange(Math.max(min, value - 1))}
          disabled={!canDecrease}
        >
          ▼
        </button>
      </span>
    </span>
  );
};

export default QuantityStepper;
