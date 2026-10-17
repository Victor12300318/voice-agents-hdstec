import { forwardRef, type ButtonHTMLAttributes } from 'react'
import { motion } from 'motion/react'
import { springSnappy } from './motion'

type Variant = 'primary' | 'secondary' | 'danger' | 'ghost'
type Size = 'sm' | 'md'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant
  size?: Size
}

const base =
  'inline-flex items-center justify-center gap-1.5 font-medium rounded-lg select-none transition-colors disabled:opacity-45 disabled:pointer-events-none focus:outline-none focus-visible:ring-2 focus-visible:ring-zinc-900/20'

const variants: Record<Variant, string> = {
  primary: 'bg-zinc-900 text-white hover:bg-zinc-800',
  secondary: 'bg-white text-zinc-800 border border-zinc-200 hover:bg-zinc-50',
  danger: 'bg-red-600 text-white hover:bg-red-700',
  ghost: 'text-zinc-600 hover:text-zinc-900 hover:bg-zinc-100',
}

const sizes: Record<Size, string> = {
  sm: 'text-xs px-3 py-1.5',
  md: 'text-sm px-4 py-2',
}

// Feedback lives on the press, instant and continuous (§1). The whileTap
// scale is a spring, so interrupting (releasing early) settles smoothly.
const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'primary', size = 'md', className = '', children, ...props },
  ref,
) {
  return (
    <motion.button
      ref={ref}
      whileTap={{ scale: 0.97 }}
      transition={springSnappy}
      className={`${base} ${variants[variant]} ${sizes[size]} ${className}`}
      {...(props as React.ComponentProps<typeof motion.button>)}
    >
      {children}
    </motion.button>
  )
})

export default Button
