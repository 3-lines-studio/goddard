import type { ReactNode } from "react";
import "./style.css";

export const metadata = {
  title: "Goddard",
};

export function Layout({ children }: { children: ReactNode }) {
  return (
    <>
      <header className="border-b border-neutral-800 p-4">
        <a href="/">Goddard</a>
      </header>
      <main className="p-4">{children}</main>
    </>
  );
}
