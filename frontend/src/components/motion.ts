// Motion presets + physics helpers, translated from Apple's "Designing Fluid
// Interfaces" (WWDC 2018). Springs are interruptible and velocity-aware by
// default; reach for these for anything the user can touch.
//
// House style: critically damped (bounce 0) everywhere by default. Reserve
// bounce for momentum-driven interactions (a flick, a drag release).

import type { Transition } from 'motion/react'

/** Default UI spring — critically damped, no overshoot. response ~0.4s. */
export const springDefault: Transition = {
  type: 'spring',
  bounce: 0,
  duration: 0.4,
}

/** Snappier default for small, frequent transitions. */
export const springSnappy: Transition = {
  type: 'spring',
  bounce: 0,
  duration: 0.3,
}

/** Drawer / sheet — a touch of bounce because the gesture carries momentum. */
export const springSheet: Transition = {
  type: 'spring',
  bounce: 0.12,
  duration: 0.35,
}

/** Momentum / flick — under-damped, slight overshoot. */
export const springMomentum: Transition = {
  type: 'spring',
  bounce: 0.2,
  duration: 0.4,
}

/**
 * Project the resting position of a flick, exactly like scroll deceleration.
 * Apple's exponential-decay form (NOT the textbook v^2/(2a)).
 * @param initialVelocity px/s at release
 * @param decelerationRate ~0.998 normal, 0.99 snappier
 */
export function project(initialVelocity: number, decelerationRate = 0.998): number {
  return ((initialVelocity / 1000) * decelerationRate) / (1 - decelerationRate)
}

/**
 * Progressive resistance past a boundary — real things slow before they stop.
 * @param overshoot how far past the bound the pointer has travelled
 * @param dimension the size of the draggable dimension
 */
export function rubberband(overshoot: number, dimension: number, constant = 0.55): number {
  return (overshoot * dimension * constant) / (dimension + constant * Math.abs(overshoot))
}

/** Fire a short haptic on commit/snap/error where supported (§13). */
export function haptic(pattern: number | number[] = 8): void {
  if (typeof navigator !== 'undefined' && 'vibrate' in navigator) {
    try {
      navigator.vibrate(pattern)
    } catch {
      /* ignore */
    }
  }
}
