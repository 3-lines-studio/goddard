import type { ReactNode } from "react";
import { TooltipProvider } from "@/components/ui/tooltip";
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
  const theme = saved || (light ? "light" : "dark");
  document.documentElement.classList.toggle("dark", theme === "dark");
})();`;

export function Layout({ children }: { children: ReactNode }) {
  return (
    <div className="bg-background text-foreground">
      <script dangerouslySetInnerHTML={{ __html: BOOT }} />
      <TooltipProvider>{children}</TooltipProvider>
    </div>
  );
}
