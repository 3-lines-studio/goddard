import type { ReactNode } from "react";
import "./style.css";

export const metadata = {
  title: "Goddard",
};

export function Layout({ children }: { children: ReactNode }) {
  return <div className="bg-neutral-950 text-neutral-100">{children}</div>;
}
