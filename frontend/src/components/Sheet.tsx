import { useEffect, useRef, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { AnimatePresence, motion, type PanInfo } from 'motion/react'
import { springSheet, project, haptic } from './motion'

type Side = 'right' | 'left' | 'bottom'

interface SheetProps {
  open: boolean
  onClose: () => void
  side?: Side
  /** Tailwind width/height constraint, e.g. "max-w-lg" (right/left) or "max-h-[85vh]" (bottom). */
  sizeClass?: string
  /** Disable drag-to-dismiss (e.g. while a call is live). */
  draggable?: boolean
  className?: string
  children: ReactNode
}

// A sheet enters and exits along the same path (§7). The user can grab it
// mid-flight and throw it — we project the release velocity to decide whether
// it commits to closing or springs back (§5, §6). The scrim dims to focus (§12).
export default function Sheet({
  open,
  onClose,
  side = 'right',
  sizeClass = 'max-w-lg',
  draggable = false,
  className = '',
  children,
}: SheetProps) {
  const panelRef = useRef<HTMLDivElement>(null)

  // Escape to dismiss + lock body scroll while open.
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    const prevOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = prevOverflow
    }
  }, [open, onClose])

  const isX = side === 'right' || side === 'left'
  const dragAxis = isX ? 'x' : 'y'
  // Closing direction: right → +x, left → −x, bottom → +y.
  const closeSign = side === 'left' ? -1 : 1

  const hidden = side === 'right' ? { x: '100%' } : side === 'left' ? { x: '-100%' } : { y: '100%' }
  const shown = isX ? { x: 0 } : { y: 0 }

  const position =
    side === 'right'
      ? 'inset-y-0 right-0 h-full'
      : side === 'left'
        ? 'inset-y-0 left-0 h-full'
        : 'inset-x-0 bottom-0 w-full'

  const rounded =
    side === 'right'
      ? 'rounded-l-[var(--radius-sheet)]'
      : side === 'left'
        ? 'rounded-r-[var(--radius-sheet)]'
        : 'rounded-t-[var(--radius-sheet)]'

  const dragProps = isX
    ? { dragConstraints: { left: 0, right: 0 } }
    : { dragConstraints: { top: 0, bottom: 0 } }

  // Track 1:1 toward the close direction; rubber-band resistance the other way.
  const dragElastic =
    side === 'right'
      ? { left: 0.08, right: 1 }
      : side === 'left'
        ? { left: 1, right: 0.08 }
        : { top: 0.08, bottom: 1 }

  function handleDragEnd(_: unknown, info: PanInfo) {
    const el = panelRef.current
    if (!el) return
    const extent = isX ? el.offsetWidth : el.offsetHeight
    const offset = isX ? info.offset.x : info.offset.y
    const velocity = isX ? info.velocity.x : info.velocity.y
    // Project where the throw lands, then decide by the projected endpoint.
    const projected = offset + project(velocity)
    const committedToClose = projected * closeSign > extent * 0.4
    if (committedToClose) {
      haptic(10)
      onClose()
    }
    // If not committed, Framer springs back to the constraint (0) automatically.
  }

  return createPortal(
    <AnimatePresence>
      {open && (
        <div className="fixed inset-0 z-50">
          {/* Scrim — dims the background to focus the task (§12). Tapping it dismisses. */}
          <motion.div
            className="absolute inset-0 bg-black/40 backdrop-blur-[2px]"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.25 }}
            onClick={onClose}
          />
          <motion.div
            ref={panelRef}
            className={`absolute ${position} ${sizeClass} w-full bg-white ${rounded} shadow-[var(--shadow-modal)] flex flex-col overflow-y-auto overscroll-contain ${className}`}
            initial={hidden}
            animate={shown}
            exit={hidden}
            transition={springSheet}
            drag={draggable ? dragAxis : false}
            dragElastic={dragElastic}
            dragMomentum={false}
            {...(draggable ? dragProps : {})}
            onDragEnd={draggable ? handleDragEnd : undefined}
          >
            {children}
          </motion.div>
        </div>
      )}
    </AnimatePresence>,
    document.body,
  )
}
