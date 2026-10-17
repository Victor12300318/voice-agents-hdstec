// frontend/src/components/visualizers/Orb.tsx
// Port nativo do 3D WebGL Shader Orb da ElevenLabs UI para Three.js direto (React 19 friendly)
import { useEffect, useRef } from 'react'
import * as THREE from 'three'

export type AgentState =
  | null
  | 'idle'
  | 'connecting'
  | 'listening'
  | 'speaking'
  | 'talking'
  | 'thinking'
  | 'interrupted'

export interface OrbProps {
  colors?: [string, string]
  agentState?: AgentState
  inputVolume?: number // 0 a 1 (voz do Caller)
  outputVolume?: number // 0 a 1 (voz do Agent)
  seed?: number
  className?: string
}

export const STATE_COLORS: Record<string, [string, string]> = {
  idle: ['#64748b', '#94a3b8'],
  connecting: ['#38bdf8', '#818cf8'],
  listening: ['#2563eb', '#60a5fa'],
  thinking: ['#8b5cf6', '#d946ef'],
  speaking: ['#059669', '#10b981'],
  talking: ['#059669', '#10b981'],
  interrupted: ['#f43f5e', '#fb923c'],
}

const vertexShader = /* glsl */ `
uniform float uTime;
varying vec2 vUv;

void main() {
  vUv = uv;
  gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
}
`

const fragmentShader = /* glsl */ `
uniform float uTime;
uniform float uAnimation;
uniform float uInverted;
uniform float uOffsets[7];
uniform vec3 uColor1;
uniform vec3 uColor2;
uniform float uInputVolume;
uniform float uOutputVolume;
uniform float uOpacity;
uniform sampler2D uPerlinTexture;
varying vec2 vUv;

const float PI = 3.14159265358979323846;

bool drawOval(vec2 polarUv, vec2 polarCenter, float a, float b, bool reverseGradient, float softness, out vec4 color) {
    vec2 p = polarUv - polarCenter;
    float oval = (p.x * p.x) / (a * a) + (p.y * p.y) / (b * b);
    float edge = smoothstep(1.0, 1.0 - softness, oval);

    if (edge > 0.0) {
        float gradient = reverseGradient ? (1.0 - (p.x / a + 1.0) / 2.0) : ((p.x / a + 1.0) / 2.0);
        gradient = mix(0.5, gradient, 0.1);
        color = vec4(vec3(gradient), 0.85 * edge);
        return true;
    }
    return false;
}

vec3 colorRamp(float grayscale, vec3 color1, vec3 color2, vec3 color3, vec3 color4) {
    if (grayscale < 0.33) {
        return mix(color1, color2, grayscale * 3.0);
    } else if (grayscale < 0.66) {
        return mix(color2, color3, (grayscale - 0.33) * 3.0);
    } else {
        return mix(color3, color4, (grayscale - 0.66) * 3.0);
    }
}

vec2 hash2(vec2 p) {
    return fract(sin(vec2(dot(p, vec2(127.1, 311.7)), dot(p, vec2(269.5, 183.3)))) * 43758.5453);
}

float noise2D(vec2 p) {
    vec2 i = floor(p);
    vec2 f = fract(p);
    vec2 u = f * f * (3.0 - 2.0 * f);
    float n = mix(
        mix(dot(hash2(i + vec2(0.0, 0.0)), f - vec2(0.0, 0.0)),
            dot(hash2(i + vec2(1.0, 0.0)), f - vec2(1.0, 0.0)), u.x),
        mix(dot(hash2(i + vec2(0.0, 1.0)), f - vec2(0.0, 1.0)),
            dot(hash2(i + vec2(1.0, 1.0)), f - vec2(1.0, 1.0)), u.x),
        u.y
    );
    return 0.5 + 0.5 * n;
}

float sharpRing(vec3 decomposed, float time) {
    float ringStart = 1.0;
    float ringWidth = 0.3;
    float noiseScale = 5.0;

    float noise = mix(
        noise2D(vec2(decomposed.x, time) * noiseScale),
        noise2D(vec2(decomposed.y, time) * noiseScale),
        decomposed.z
    );

    noise = (noise - 0.5) * 2.5;
    return ringStart + noise * ringWidth * 1.5;
}

float smoothRing(vec3 decomposed, float time) {
    float ringStart = 0.9;
    float ringWidth = 0.2;
    float noiseScale = 6.0;

    float noise = mix(
        noise2D(vec2(decomposed.x, time) * noiseScale),
        noise2D(vec2(decomposed.y, time) * noiseScale),
        decomposed.z
    );

    noise = (noise - 0.5) * 5.0;
    return ringStart + noise * ringWidth;
}

float flow(vec3 decomposed, float time) {
    return mix(
        texture(uPerlinTexture, vec2(time, decomposed.x / 2.0)).r,
        texture(uPerlinTexture, vec2(time, decomposed.y / 2.0)).r,
        decomposed.z
    );
}

void main() {
    vec2 uv = vUv * 2.0 - 1.0;
    float radius = length(uv);
    float theta = atan(uv.y, uv.x);
    if (theta < 0.0) theta += 2.0 * PI;

    vec3 decomposed = vec3(
        theta / (2.0 * PI),
        mod(theta / (2.0 * PI) + 0.5, 1.0) + 1.0,
        abs(theta / PI - 1.0)
    );

    float noise = flow(decomposed, radius * 0.03 - uAnimation * 0.2) - 0.5;
    theta += noise * mix(0.08, 0.25, uOutputVolume);

    vec4 color = vec4(1.0, 1.0, 1.0, 1.0);

    float centers[7];
    for (int i = 0; i < 7; i++) {
        centers[i] = (float(i) * 0.5 * PI) + 0.5 * sin(uTime / 20.0 + uOffsets[i]);
    }

    float a, b;
    vec4 ovalColor;

    for (int i = 0; i < 7; i++) {
        float n = texture(uPerlinTexture, vec2(mod(centers[i] + uTime * 0.05, 1.0), 0.5)).r;
        a = 0.5 + n * 0.3;
        b = n * mix(3.5, 2.5, uInputVolume);
        bool reverseGradient = (i % 2 == 1);

        float distTheta = min(
            abs(theta - centers[i]),
            min(
                abs(theta + 2.0 * PI - centers[i]),
                abs(theta - 2.0 * PI - centers[i])
            )
        );
        float distRadius = radius;
        float softness = 0.6;

        if (drawOval(vec2(distTheta, distRadius), vec2(0.0, 0.0), a, b, reverseGradient, softness, ovalColor)) {
            color.rgb = mix(color.rgb, ovalColor.rgb, ovalColor.a);
            color.a = max(color.a, ovalColor.a);
        }
    }

    float ringRadius1 = sharpRing(decomposed, uTime * 0.1);
    float ringRadius2 = smoothRing(decomposed, uTime * 0.1);

    float inputRadius1 = radius + uInputVolume * 0.2;
    float inputRadius2 = radius + uInputVolume * 0.15;
    float opacity1 = mix(0.2, 0.6, uInputVolume);
    float opacity2 = mix(0.15, 0.45, uInputVolume);

    float ringAlpha1 = (inputRadius2 >= ringRadius1) ? opacity1 : 0.0;
    float ringAlpha2 = smoothstep(ringRadius2 - 0.05, ringRadius2 + 0.05, inputRadius1) * opacity2;
    float totalRingAlpha = max(ringAlpha1, ringAlpha2);

    vec3 ringColor = vec3(1.0);
    color.rgb = 1.0 - (1.0 - color.rgb) * (1.0 - ringColor * totalRingAlpha);

    vec3 color1 = vec3(0.0, 0.0, 0.0);
    vec3 color2 = uColor1;
    vec3 color3 = uColor2;
    vec3 color4 = vec3(1.0, 1.0, 1.0);

    float luminance = mix(color.r, 1.0 - color.r, uInverted);
    color.rgb = colorRamp(luminance, color1, color2, color3, color4);
    color.a *= uOpacity;

    gl_FragColor = color;
}
`

function createProceduralNoiseTexture(): THREE.DataTexture {
  const size = 128
  const data = new Uint8Array(size * size * 4)
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      const idx = (y * size + x) * 4
      const v = Math.floor(
        ((Math.sin(x * 0.1) + Math.cos(y * 0.1) + Math.sin((x + y) * 0.05) + 3) / 6) * 255,
      )
      data[idx] = v
      data[idx + 1] = v
      data[idx + 2] = v
      data[idx + 3] = 255
    }
  }
  const tex = new THREE.DataTexture(data, size, size, THREE.RGBAFormat)
  tex.wrapS = THREE.RepeatWrapping
  tex.wrapT = THREE.RepeatWrapping
  tex.needsUpdate = true
  return tex
}

function clamp01(n: number) {
  if (!Number.isFinite(n)) return 0
  return Math.min(1, Math.max(0, n))
}

export function Orb({
  colors,
  agentState = 'idle',
  inputVolume = 0,
  outputVolume = 0,
  seed = 42,
  className = 'w-64 h-64',
}: OrbProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const canvasRef = useRef<HTMLCanvasElement>(null)

  const activeColors = colors || STATE_COLORS[agentState || 'idle'] || STATE_COLORS.idle
  const targetColor1Ref = useRef(new THREE.Color(activeColors[0]))
  const targetColor2Ref = useRef(new THREE.Color(activeColors[1]))
  const animSpeedRef = useRef(0.1)
  const curInRef = useRef(0)
  const curOutRef = useRef(0)

  useEffect(() => {
    targetColor1Ref.current.set(activeColors[0])
    targetColor2Ref.current.set(activeColors[1])
  }, [activeColors[0], activeColors[1]])

  useEffect(() => {
    const canvas = canvasRef.current
    const container = containerRef.current
    if (!canvas || !container) return

    const scene = new THREE.Scene()
    const camera = new THREE.OrthographicCamera(-3.6, 3.6, 3.6, -3.6, 0.1, 10)
    camera.position.z = 1

    const renderer = new THREE.WebGLRenderer({
      canvas,
      alpha: true,
      antialias: true,
      powerPreference: 'high-performance',
    })
    renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2))

    const isDark = document.documentElement.classList.contains('dark')
    const fallbackTexture = createProceduralNoiseTexture()

    const offsets = new Float32Array(7)
    let s = seed
    for (let i = 0; i < 7; i++) {
      s = (s * 16807) % 2147483647
      offsets[i] = (s / 2147483647) * Math.PI * 2
    }

    const uniforms = {
      uColor1: { value: new THREE.Color(activeColors[0]) },
      uColor2: { value: new THREE.Color(activeColors[1]) },
      uOffsets: { value: offsets },
      uPerlinTexture: { value: fallbackTexture as THREE.Texture },
      uTime: { value: 0 },
      uAnimation: { value: 0.1 },
      uInverted: { value: isDark ? 1 : 0 },
      uInputVolume: { value: 0 },
      uOutputVolume: { value: 0 },
      uOpacity: { value: 0 },
    }

    const geometry = new THREE.CircleGeometry(3.5, 64)
    const material = new THREE.ShaderMaterial({
      uniforms,
      vertexShader,
      fragmentShader,
      transparent: true,
    })

    const mesh = new THREE.Mesh(geometry, material)
    scene.add(mesh)

    // Tenta carregar a textura oficial da ElevenLabs; se offline, mantém procedural
    const loader = new THREE.TextureLoader()
    loader.load(
      'https://storage.googleapis.com/eleven-public-cdn/images/perlin-noise.png',
      (tex) => {
        tex.wrapS = THREE.RepeatWrapping
        tex.wrapT = THREE.RepeatWrapping
        uniforms.uPerlinTexture.value = tex
      },
      undefined,
      () => {
        // ignora se falhar e usa fallback
      },
    )

    const updateSize = () => {
      const rect = container.getBoundingClientRect()
      const width = rect.width || 256
      const height = rect.height || 256
      renderer.setSize(width, height)

      const aspect = width / height
      if (aspect >= 1) {
        camera.left = -3.6 * aspect
        camera.right = 3.6 * aspect
        camera.top = 3.6
        camera.bottom = -3.6
      } else {
        camera.left = -3.6
        camera.right = 3.6
        camera.top = 3.6 / aspect
        camera.bottom = -3.6 / aspect
      }
      camera.updateProjectionMatrix()
    }

    updateSize()
    const resizeObserver = new ResizeObserver(updateSize)
    resizeObserver.observe(container)

    let reqId = 0
    let lastTime = performance.now()

    const animate = (time: number) => {
      const delta = Math.min((time - lastTime) / 1000, 0.1)
      lastTime = time

      uniforms.uTime.value += delta * 0.5
      if (uniforms.uOpacity.value < 1) {
        uniforms.uOpacity.value = Math.min(1, uniforms.uOpacity.value + delta * 2.5)
      }

      // Interpolação suave dos volumes de entrada (Caller) e saída (Agent)
      const targetIn = clamp01(inputVolume)
      const targetOut = clamp01(outputVolume)

      curInRef.current += (targetIn - curInRef.current) * 0.2
      curOutRef.current += (targetOut - curOutRef.current) * 0.2

      const targetSpeed = 0.1 + (1 - Math.pow(curOutRef.current - 1, 2)) * 0.9
      animSpeedRef.current += (targetSpeed - animSpeedRef.current) * 0.12

      uniforms.uAnimation.value += delta * animSpeedRef.current
      uniforms.uInputVolume.value = curInRef.current
      uniforms.uOutputVolume.value = curOutRef.current

      uniforms.uColor1.value.lerp(targetColor1Ref.current, 0.08)
      uniforms.uColor2.value.lerp(targetColor2Ref.current, 0.08)

      renderer.render(scene, camera)
      reqId = requestAnimationFrame(animate)
    }

    reqId = requestAnimationFrame(animate)

    return () => {
      cancelAnimationFrame(reqId)
      resizeObserver.disconnect()
      geometry.dispose()
      material.dispose()
      fallbackTexture.dispose()
      renderer.dispose()
    }
  }, [seed])

  return (
    <div ref={containerRef} className={`relative flex items-center justify-center ${className}`}>
      <canvas ref={canvasRef} className="block w-full h-full" />
    </div>
  )
}
