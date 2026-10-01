import type { ReactNode } from "react";
import "./avatar.css";
import "./style.css";

export const metadata = {
  title: "Goddard",
};

const BOOT = `(() => {
  let saved = null;
  try {
    saved = localStorage.getItem("goddard-theme");
  } catch {}
  const light = matchMedia("(prefers-color-scheme: light)").matches;
  document.documentElement.dataset.theme = saved || (light ? "light" : "dark");
})();`;

export function Layout({ children }: { children: ReactNode }) {
  return (
    <div className="bg-bg text-text">
      <script dangerouslySetInnerHTML={{ __html: BOOT }} />
      {children}
    </div>
  );
}
