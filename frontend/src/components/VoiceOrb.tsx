import { motion } from 'motion/react'
import { Mic, MicOff } from 'lucide-react'

interface VoiceOrbProps {
  /** Agent is currently producing audio. */
  speaking: boolean
  /** Call is live. */
  active: boolean
  /** Mic is muted. */
  muted: boolean
}

// The orb is driven by the real audio state (AudioPlayer.onSpeakingChange).
// While the agent speaks it breathes and a waveform comes alive; muted and idle
// states are calm. Motion telegraphs what the system is doing (§8).
export default function VoiceOrb({ speaking, active, muted }: VoiceOrbProps) {
  const tone = speaking
    ? 'bg-blue-500/20 text-blue-300'
    : active
      ? muted
        ? 'bg-amber-500/20 text-amber-300'
        : 'bg-emerald-500/20 text-emerald-300'
      : 'bg-zinc-800 text-zinc-400'

  const ringColor = speaking
    ? 'ring-blue-500/30'
    : active && !muted
      ? 'ring-emerald-500/30'
      : 'ring-transparent'

  return (
    <div className="flex flex-col items-center gap-3">
      <div className="relative flex items-center justify-center">
        {/* Expanding halo while speaking */}
        {speaking && (
          <motion.span
            className="absolute inset-0 rounded-full bg-blue-500/20"
            initial={{ scale: 1, opacity: 0.5 }}
            animate={{ scale: 1.9, opacity: 0 }}
            transition={{ duration: 1.4, repeat: Infinity, ease: 'easeOut' }}
          />
        )}
        <motion.div
          className={`relative w-16 h-16 rounded-full flex items-center justify-center ring-4 ${ringColor} ${tone}`}
          animate={{ scale: speaking ? [1, 1.07, 1] : 1 }}
          transition={
            speaking
              ? { duration: 1.1, repeat: Infinity, ease: 'easeInOut' }
              : { type: 'spring', bounce: 0, duration: 0.4 }
          }
        >
          {muted ? <MicOff className="w-8 h-8" /> : <Mic className="w-8 h-8" />}
        </motion.div>
      </div>

      {/* Waveform — alive only while the agent speaks */}
      <div className="flex items-end gap-1 h-5">
        {[0, 1, 2, 3, 4].map((i) => (
          <motion.span
            key={i}
            className="w-1 rounded-full bg-blue-400/80"
            animate={
              speaking
                ? { height: ['30%', '100%', '45%', '85%', '30%'] }
                : { height: '20%' }
            }
            transition={
              speaking
                ? { duration: 0.9, repeat: Infinity, ease: 'easeInOut', delay: i * 0.08 }
                : { type: 'spring', bounce: 0, duration: 0.3 }
            }
            style={{ height: '20%' }}
          />
        ))}
      </div>
    </div>
  )
}
