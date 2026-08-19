import { ButtonHTMLAttributes, FC } from 'react';
import styles from './Button.module.css';

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';

type ButtonPropsT = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant;
};

// The one button primitive for the app: pick a variant instead of styling a
// <button> ad hoc. `type="button"` is the default (matches the native HTML
// default) - pass type="submit" explicitly for form submit buttons.
const Button: FC<ButtonPropsT> = ({
  variant = 'primary',
  type = 'button',
  className,
  ...rest
}) => {
  const classes = [styles.btn, styles[variant], className]
    .filter(Boolean)
    .join(' ');

  return <button type={type} className={classes} {...rest} />;
};

export default Button;
