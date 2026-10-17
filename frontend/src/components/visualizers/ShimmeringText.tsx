// frontend/src/components/visualizers/ShimmeringText.tsx
// Port do componente de texto com brilho gradiente fluido da ElevenLabs UI via Motion
import { useMemo } from 'react'
import { motion } from 'motion/react'

export interface ShimmeringTextProps {
  text: string
  duration?: number
  spread?: number
  color?: string
  shimmerColor?: string
  className?: string
}

export function ShimmeringText({
  text,
  duration = 2.2,
  spread = 2,
  color,
  shimmerColor,
  className = '',
}: ShimmeringTextProps) {
  const dynamicSpread = useMemo(() => {
    return Math.max(40, text.length * spread * 2.5)
  }, [text, spread])

  const baseColor = color || 'rgba(148, 163, 184, 0.85)' // Slate 400
  const activeShimmer = shimmerColor || '#ffffff'

  return (
    <motion.span
      className={`relative inline-block bg-clip-text text-transparent select-none font-medium tracking-tight ${className}`}
      style={
        {
          backgroundImage: `linear-gradient(90deg, transparent calc(50% - ${dynamicSpread}px), ${activeShimmer}, transparent calc(50% + ${dynamicSpread}px)), linear-gradient(${baseColor}, ${baseColor})`,
          backgroundSize: '250% 100%, 100% 100%',
          backgroundRepeat: 'no-repeat',
        } as React.CSSProperties
      }
      initial={{
        backgroundPosition: '150% center, 0% 0%',
        opacity: 0.8,
      }}
      animate={{
        backgroundPosition: ['150% center, 0% 0%', '-50% center, 0% 0%'],
        opacity: 1,
      }}
      transition={{
        backgroundPosition: {
          repeat: Infinity,
          duration,
          ease: 'linear',
        },
      }}
    >
      {text}
    </motion.span>
  )
}
