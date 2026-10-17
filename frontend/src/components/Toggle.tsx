import { motion } from 'motion/react'
import { springSnappy, haptic } from './motion'

interface ToggleProps {
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean
  label?: string
}

// iOS-style switch. The knob rides a spring between ends, so a rapid toggle
// stays continuous instead of snapping (§4). A light haptic marks the commit.
export default function Toggle({ checked, onChange, disabled, label }: ToggleProps) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => {
        haptic(6)
        onChange(!checked)
      }}
      className={`relative inline-flex h-6 w-10 flex-shrink-0 items-center rounded-full px-0.5 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-zinc-900/20 disabled:opacity-45 ${
        checked ? 'bg-emerald-500' : 'bg-zinc-300'
      }`}
    >
      <motion.span
        layout
        transition={springSnappy}
        className="h-5 w-5 rounded-full bg-white shadow-sm"
        style={{ marginLeft: checked ? 'auto' : 0 }}
      />
    </button>
  )
}
