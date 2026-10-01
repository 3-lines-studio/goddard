import { useEffect, useRef, type CSSProperties } from "react";

const BODIES = ["round", "squircle", "square", "egg"];
const EYES = ["bar", "dot", "square", "slit"];

const bots = new Set<HTMLElement>();
let pointerX = 0;
let pointerY = 0;
let queued = 0;
let watching = false;

function hash(name: string, seed: number) {
  let value = (2166136261 ^ seed) >>> 0;
  for (const character of name) {
    value ^= character.codePointAt(0) ?? 0;
    value = Math.imul(value, 16777619) >>> 0;
  }
  value ^= value >>> 15;
  value = Math.imul(value, 2246822507) >>> 0;
  return (value ^ (value >>> 13)) >>> 0;
}

function hue(name: string) {
  let value = 0;
  for (const character of name) value = (value * 31 + (character.codePointAt(0) ?? 0)) % 360;
  return value;
}

function face(name: string) {
  return {
    body: BODIES[hash(name, 7) % BODIES.length],
    eyes: EYES[hash(name, 53) % EYES.length],
    blink: (2.8 + (hash(name, 11) % 340) / 100).toFixed(2) + "s",
    delay: "-" + ((hash(name, 13) % 500) / 100).toFixed(2) + "s",
    hop: "-" + ((hash(name, 17) % 110) / 100).toFixed(2) + "s",
    gaze: "-" + ((hash(name, 19) % 1100) / 100).toFixed(2) + "s",
  };
}

function follow() {
  queued = 0;
  for (const bot of bots) {
    if (!bot.isConnected) {
      bots.delete(bot);
      continue;
    }
    const box = bot.getBoundingClientRect();
    if (!box.width) continue;
    const dx = pointerX - (box.left + box.width / 2);
    const dy = pointerY - (box.top + box.height / 2);
    const distance = Math.hypot(dx, dy) || 1;
    const reach = Math.min(1, distance / 320);
    const shift = box.width * (bot.classList.contains("focused") ? 0.3 : 0.17) * reach;
    bot.style.setProperty("--dx", ((dx / distance) * shift).toFixed(2) + "px");
    bot.style.setProperty("--dy", ((dy / distance) * shift * 0.7).toFixed(2) + "px");
  }
}

function watch() {
  if (watching) return;
  watching = true;
  if (typeof window === "undefined" || matchMedia("(prefers-reduced-motion: reduce)").matches) return;
  window.addEventListener(
    "pointermove",
    (event) => {
      pointerX = event.clientX;
      pointerY = event.clientY;
      if (!queued) queued = requestAnimationFrame(follow);
    },
    { passive: true },
  );
}

export function Avatar({ name, state = "idle", size = 24 }: { name: string; state?: string; size?: number }) {
  const ref = useRef<HTMLSpanElement>(null);
  const bot = face(name);

  useEffect(() => {
    const element = ref.current;
    if (!element) return;
    bots.add(element);
    watch();
    return () => {
      bots.delete(element);
    };
  }, []);

  return (
    <span
      ref={ref}
      data-avatar={name}
      data-avatar-state={state}
      className={`bot body-${bot.body} eyes-${bot.eyes} ${state}`}
      style={
        {
          "--s": `${size}px`,
          "--h": hue(name),
          "--blink": bot.blink,
          "--delay": bot.delay,
          "--hop": bot.hop,
          "--gaze": bot.gaze,
        } as CSSProperties
      }
    >
      <span className="eye">
        <i />
      </span>
      <span className="eye">
        <i />
      </span>
    </span>
  );
}
